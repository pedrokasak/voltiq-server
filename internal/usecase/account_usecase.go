package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// AccountUseCase handles the profile of the authenticated user, tenant user
// management (RF07.2, RF07.3), onboarding (RF08.2), tenant settings and the
// audit trail (RF07.4).
type AccountUseCase struct {
	userRepo    *repository.UserRepository
	tenantRepo  *repository.TenantRepository
	inviteRepo  *repository.InviteRepository
	accountRepo *repository.AccountRepository
}

// NewAccountUseCase creates an AccountUseCase
func NewAccountUseCase(
	userRepo *repository.UserRepository,
	tenantRepo *repository.TenantRepository,
	inviteRepo *repository.InviteRepository,
	accountRepo *repository.AccountRepository,
) *AccountUseCase {
	return &AccountUseCase{userRepo: userRepo, tenantRepo: tenantRepo, inviteRepo: inviteRepo, accountRepo: accountRepo}
}

// TenantSummary is the part of the tenant the client needs on every page.
type TenantSummary struct {
	ID             domain.UUID         `json:"id"`
	Name           string              `json:"name"`
	Plan           domain.TenantPlan   `json:"plan"`
	Status         domain.TenantStatus `json:"status"`
	TrialExpiresAt *time.Time          `json:"trial_expires_at"`
	TrialDaysLeft  *int                `json:"trial_days_left"`
}

// Me is the profile of the authenticated user.
type Me struct {
	User        *domain.User      `json:"user"`
	Tenant      *TenantSummary    `json:"tenant"`
	Permissions []string          `json:"permissions"`
	Limits      domain.PlanLimits `json:"limits"`
	Usage       *repository.Usage `json:"usage"`
	Onboarding  *OnboardingStatus `json:"onboarding"`
}

// GetMe returns the profile, permissions, plan limits and onboarding state.
func (uc *AccountUseCase) GetMe(ctx context.Context, userID domain.UUID) (*Me, error) {
	user, err := uc.userRepo.GetByID(ctx, userID)
	if err != nil || user == nil {
		return nil, ErrUserNotFound
	}
	tenant, err := uc.tenantRepo.GetByID(ctx, user.TenantID)
	if err != nil || tenant == nil {
		return nil, ErrTenantNotFound
	}
	me := &Me{
		User:        user,
		Tenant:      summarizeTenant(tenant),
		Permissions: domain.PermissionsOf(user.Role),
		Limits:      domain.LimitsForPlan(tenant.Plan),
	}
	if usage, err := uc.accountRepo.GetUsage(ctx, tenant.ID); err == nil {
		me.Usage = usage
		me.Onboarding = uc.onboarding(ctx, tenant.ID, usage)
	}
	return me, nil
}

func summarizeTenant(t *domain.Tenant) *TenantSummary {
	s := &TenantSummary{ID: t.ID, Name: t.Name, Plan: t.Plan, Status: t.Status, TrialExpiresAt: t.TrialExpiresAt}
	if t.Status == domain.TenantStatusTrial && t.TrialExpiresAt != nil {
		days := int(time.Until(*t.TrialExpiresAt).Hours() / 24)
		if days < 0 {
			days = 0
		}
		s.TrialDaysLeft = &days
	}
	return s
}

// ---------------------------------------------------------------- users

// TenantUser is a user of the tenant as listed in the settings screen.
type TenantUser struct {
	*domain.User
	Permissions []string `json:"permissions"`
	IsSelf      bool     `json:"is_self"`
}

// UsersPage lists users and pending invites with the seat usage.
type UsersPage struct {
	Users    []*TenantUser    `json:"users"`
	Invites  []*domain.Invite `json:"invites"`
	MaxUsers int              `json:"max_users"`
	Seats    int              `json:"seats_used"`
}

// ListUsers returns the users and pending invites of the tenant.
func (uc *AccountUseCase) ListUsers(ctx context.Context, tenantID, actorID domain.UUID) (*UsersPage, error) {
	users, err := uc.userRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	page := &UsersPage{Users: []*TenantUser{}, Invites: []*domain.Invite{}}
	for _, u := range users {
		if u.Role == domain.UserRoleSuperAdmin {
			continue
		}
		page.Users = append(page.Users, &TenantUser{User: u, Permissions: domain.PermissionsOf(u.Role), IsSelf: u.ID == actorID})
	}
	if invites, err := uc.inviteRepo.ListPending(ctx, tenantID); err == nil {
		page.Invites = invites
	}
	if tenant, err := uc.tenantRepo.GetByID(ctx, tenantID); err == nil && tenant != nil {
		page.MaxUsers = domain.LimitsForPlan(tenant.Plan).MaxUsers
		if tenant.MaxUsers > 0 {
			page.MaxUsers = tenant.MaxUsers
		}
	}
	page.Seats = len(page.Users) + len(page.Invites)
	return page, nil
}

// UpdateUserInput changes role, name or active flag of a tenant user.
type UpdateUserInput struct {
	Role   *domain.UserRole
	Name   *string
	Active *bool
}

// UpdateUser aplica as regras de RBAC: ninguem altera o proprio papel ou se
// desativa, ninguem concede papel acima do seu, so OWNER mexe em OWNER e o
// tenant sempre mantem ao menos um OWNER ativo.
func (uc *AccountUseCase) UpdateUser(ctx context.Context, actorID, targetID domain.UUID, in UpdateUserInput) (*domain.User, error) {
	actor, err := uc.userRepo.GetByID(ctx, actorID)
	if err != nil || actor == nil {
		return nil, ErrUserNotFound
	}
	target, err := uc.userRepo.GetByID(ctx, targetID)
	if err != nil || target == nil || target.TenantID != actor.TenantID || target.Role == domain.UserRoleSuperAdmin {
		return nil, ErrNotFound
	}
	if target.Role == domain.UserRoleOwner && actor.Role != domain.UserRoleOwner && actor.Role != domain.UserRoleSuperAdmin {
		return nil, fmt.Errorf("%w: apenas o proprietário altera outro proprietário", ErrForbidden)
	}

	if in.Role != nil && *in.Role != target.Role {
		if actor.ID == target.ID {
			return nil, badRequest("você não pode alterar o próprio papel")
		}
		if !domain.CanAssignRole(actor.Role, *in.Role) {
			return nil, fmt.Errorf("%w: papel não permitido", ErrForbidden)
		}
		if target.Role == domain.UserRoleOwner {
			if err := uc.ensureAnotherOwner(ctx, target); err != nil {
				return nil, err
			}
		}
		target.Role = *in.Role
	}
	if in.Active != nil && *in.Active != target.Active {
		if actor.ID == target.ID && !*in.Active {
			return nil, badRequest("você não pode desativar o próprio usuário")
		}
		if !*in.Active && target.Role == domain.UserRoleOwner {
			if err := uc.ensureAnotherOwner(ctx, target); err != nil {
				return nil, err
			}
		}
		target.Active = *in.Active
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if len(name) < 2 {
			return nil, &domain.ValidationError{Fields: []domain.FieldError{{Field: "name", Message: "nome muito curto"}}}
		}
		target.Name = name
	}
	if err := uc.userRepo.Update(ctx, target); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return target, nil
}

func (uc *AccountUseCase) ensureAnotherOwner(ctx context.Context, target *domain.User) error {
	users, err := uc.userRepo.GetByTenant(ctx, target.TenantID)
	if err != nil {
		return err
	}
	for _, u := range users {
		if u.ID != target.ID && u.Role == domain.UserRoleOwner && u.Active {
			return nil
		}
	}
	return badRequest("o tenant precisa de ao menos um proprietário ativo; promova outro usuário antes")
}

// RemoveUser soft-deletes a tenant user.
func (uc *AccountUseCase) RemoveUser(ctx context.Context, actorID, targetID domain.UUID) error {
	if actorID == targetID {
		return badRequest("você não pode remover o próprio usuário")
	}
	actor, err := uc.userRepo.GetByID(ctx, actorID)
	if err != nil || actor == nil {
		return ErrUserNotFound
	}
	target, err := uc.userRepo.GetByID(ctx, targetID)
	if err != nil || target == nil || target.TenantID != actor.TenantID || target.Role == domain.UserRoleSuperAdmin {
		return ErrNotFound
	}
	if domain.RoleRank(target.Role) > domain.RoleRank(actor.Role) {
		return fmt.Errorf("%w: usuário com papel superior", ErrForbidden)
	}
	if target.Role == domain.UserRoleOwner {
		if err := uc.ensureAnotherOwner(ctx, target); err != nil {
			return err
		}
	}
	if err := uc.userRepo.Delete(ctx, targetID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// ---------------------------------------------------------------- onboarding

// OnboardingStep is one step of the onboarding wizard.
type OnboardingStep struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Done     bool   `json:"done"`
	Optional bool   `json:"optional"`
	Link     string `json:"link"`
}

// OnboardingStatus is the progress of the wizard (RF08.2).
type OnboardingStatus struct {
	Steps       []OnboardingStep `json:"steps"`
	Completed   bool             `json:"completed"`
	CompletedAt *time.Time       `json:"completed_at"`
	ProgressPct int              `json:"progress_pct"`
	NextStep    string           `json:"next_step"`
}

func (uc *AccountUseCase) onboarding(ctx context.Context, tenantID domain.UUID, u *repository.Usage) *OnboardingStatus {
	st := &OnboardingStatus{Steps: []OnboardingStep{
		{Key: "company", Title: "Criar a empresa", Done: true, Link: "/settings"},
		{Key: "transformer", Title: "Cadastrar o primeiro transformador", Done: u.Transformers > 0, Link: "/transformers"},
		{Key: "consuming_units", Title: "Vincular unidades consumidoras", Done: u.ConsumingUnits > 0, Link: "/imports"},
		{Key: "import", Title: "Importar leituras (CSV de exemplo)", Done: u.CompletedImport > 0, Link: "/imports"},
		{Key: "balance", Title: "Ver o primeiro balanço", Done: u.Balances > 0, Link: "/balance"},
		{Key: "alerts", Title: "Configurar alertas", Done: u.Recipients > 0, Optional: true, Link: "/alerts"},
		{Key: "team", Title: "Convidar a equipe", Done: u.Users > 1 || u.PendingInvites > 0, Optional: true, Link: "/settings/users"},
	}}
	required, done := 0, 0
	for _, s := range st.Steps {
		if s.Optional {
			continue
		}
		required++
		if s.Done {
			done++
		} else if st.NextStep == "" {
			st.NextStep = s.Key
		}
	}
	st.ProgressPct = done * 100 / required
	if settings, err := uc.accountRepo.GetSettings(ctx, tenantID); err == nil && settings.OnboardingCompletedAt != nil {
		st.CompletedAt = settings.OnboardingCompletedAt
		st.Completed = true
	}
	if done == required {
		st.Completed = true
	}
	return st
}

// GetOnboarding returns the onboarding progress of a tenant.
func (uc *AccountUseCase) GetOnboarding(ctx context.Context, tenantID domain.UUID) (*OnboardingStatus, error) {
	usage, err := uc.accountRepo.GetUsage(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return uc.onboarding(ctx, tenantID, usage), nil
}

// CompleteOnboarding marks the wizard as finished (or skipped).
func (uc *AccountUseCase) CompleteOnboarding(ctx context.Context, tenantID domain.UUID) (*OnboardingStatus, error) {
	now := time.Now()
	if err := uc.accountRepo.SetOnboardingCompleted(ctx, tenantID, &now); err != nil {
		return nil, err
	}
	return uc.GetOnboarding(ctx, tenantID)
}

// ---------------------------------------------------------------- settings

// SettingsInput changes tenant settings.
type SettingsInput struct {
	Name          *string
	TariffBRLMWh  *float64
	ClearTariff   bool
	Address       *string
	AddressNumber *string
	Province      *string
	PostalCode    *string
}

// GetSettings returns the tenant settings.
func (uc *AccountUseCase) GetSettings(ctx context.Context, tenantID domain.UUID) (*repository.TenantSettings, error) {
	return uc.accountRepo.GetSettings(ctx, tenantID)
}

// UpdateSettings validates and saves the tenant settings.
func (uc *AccountUseCase) UpdateSettings(ctx context.Context, tenantID domain.UUID, in SettingsInput) (*repository.TenantSettings, error) {
	s, err := uc.accountRepo.GetSettings(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	v := &domain.ValidationError{}
	if in.Name != nil {
		if n := strings.TrimSpace(*in.Name); len(n) < 2 {
			v.Fields = append(v.Fields, domain.FieldError{Field: "name", Message: "nome muito curto"})
		} else {
			s.Name = n
		}
	}
	if in.ClearTariff {
		s.TariffBRLMWh = nil
	} else if in.TariffBRLMWh != nil {
		if *in.TariffBRLMWh <= 0 || *in.TariffBRLMWh > 10000 {
			v.Fields = append(v.Fields, domain.FieldError{Field: "tariff_brl_mwh", Message: "tarifa deve estar entre 0 e 10.000 R$/MWh"})
		} else {
			s.TariffBRLMWh = in.TariffBRLMWh
		}
	}
	for _, f := range []struct {
		dst *string
		src *string
	}{{&s.Address, in.Address}, {&s.AddressNumber, in.AddressNumber}, {&s.Province, in.Province}, {&s.PostalCode, in.PostalCode}} {
		if f.src != nil {
			*f.dst = strings.TrimSpace(*f.src)
		}
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	if err := uc.accountRepo.UpdateSettings(ctx, s); err != nil {
		return nil, err
	}
	// a tarifa entra nos dashboards em cache
	repository.InvalidateSummaries(tenantID)
	return s, nil
}

// ---------------------------------------------------------------- audit

// ListAudit returns the audit trail of the tenant.
func (uc *AccountUseCase) ListAudit(ctx context.Context, tenantID domain.UUID, f repository.AuditFilter) ([]*repository.AuditEntry, int, error) {
	return uc.accountRepo.ListAudit(ctx, tenantID, f)
}
