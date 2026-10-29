package usecase

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/email"
	"github.com/voltiq/server/internal/repository"
)

var (
	ErrInviteNotFound    = errors.New("invite not found")
	ErrInviteExpired     = errors.New("invite has expired")
	ErrInviteAlreadyUsed = errors.New("invite already accepted")
	ErrInviteCancelled   = errors.New("invite was cancelled")
	ErrEmailAlreadyInUse = errors.New("email already in use")
)

// inviteTTL is how long an invite stays valid.
const inviteTTL = 7 * 24 * time.Hour

// InviteUseCase handles user invitation business logic (RF07.3)
type InviteUseCase struct {
	inviteRepo    *repository.InviteRepository
	userRepo      *repository.UserRepository
	tenantRepo    *repository.TenantRepository
	emailProvider email.EmailProvider
	appURL        string
}

// CreateInviteInput contains data to create an invite
type CreateInviteInput struct {
	TenantID  domain.UUID
	Email     string
	Role      domain.UserRole
	InvitedBy domain.UUID
	ActorRole domain.UserRole
}

// CreateInviteOutput contains the created invite. Token is returned only
// here (and by Resend); the database keeps just its hash.
type CreateInviteOutput struct {
	Invite    *domain.Invite
	Token     string
	ExpiresAt time.Time
	InviteURL string
	EmailSent bool
}

// AcceptInviteInput contains data to accept an invite
type AcceptInviteInput struct {
	Token    string
	Password string
	Name     string
}

// NewInviteUseCase creates a new InviteUseCase. The e-mail provider is
// optional; without it the invite link is only returned to the admin.
func NewInviteUseCase(
	inviteRepo *repository.InviteRepository,
	userRepo *repository.UserRepository,
	tenantRepo *repository.TenantRepository,
) *InviteUseCase {
	return &InviteUseCase{inviteRepo: inviteRepo, userRepo: userRepo, tenantRepo: tenantRepo}
}

// SetEmail configures invite e-mails.
func (uc *InviteUseCase) SetEmail(p email.EmailProvider, appURL string) {
	uc.emailProvider = p
	uc.appURL = strings.TrimRight(appURL, "/")
}

func (uc *InviteUseCase) inviteURL(token string) string {
	base := uc.appURL
	if base == "" {
		base = "https://app.voltiq.com.br"
	}
	return base + "/invite/" + token
}

// CreateInvite creates a new user invitation
func (uc *InviteUseCase) CreateInvite(ctx context.Context, input CreateInviteInput) (*CreateInviteOutput, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(input.Email))
	if err != nil {
		return nil, &domain.ValidationError{Fields: []domain.FieldError{{Field: "email", Message: "e-mail inválido"}}}
	}
	input.Email = strings.ToLower(addr.Address)
	if input.Role == "" {
		input.Role = domain.UserRoleViewer
	}
	if !domain.ValidTenantRole(input.Role) {
		return nil, &domain.ValidationError{Fields: []domain.FieldError{{Field: "role", Message: "papel inválido"}}}
	}
	if input.ActorRole != "" && !domain.CanAssignRole(input.ActorRole, input.Role) {
		return nil, fmt.Errorf("%w: você não pode convidar com este papel", ErrForbidden)
	}

	tenant, err := uc.tenantRepo.GetByID(ctx, input.TenantID)
	if err != nil || tenant == nil {
		return nil, ErrTenantNotFound
	}
	if existing, _ := uc.userRepo.GetByEmail(ctx, input.Email); existing != nil {
		return nil, ErrEmailAlreadyInUse
	}

	maxUsers := tenant.MaxUsers
	if maxUsers <= 0 {
		maxUsers = domain.LimitsForPlan(tenant.Plan).MaxUsers
	}
	if maxUsers > 0 && tenant.SeatCount >= maxUsers {
		return nil, &PlanLimitError{Resource: "users", Plan: string(tenant.Plan), Limit: maxUsers, Current: tenant.SeatCount}
	}

	token := uuid.New().String() + uuid.New().String()[:8]
	expiresAt := time.Now().Add(inviteTTL)
	invite := &domain.Invite{
		ID:        domain.UUID(uuid.New().String()),
		TenantID:  input.TenantID,
		Email:     input.Email,
		Role:      input.Role,
		Token:     repository.HashToken(token),
		Status:    domain.InviteStatusPending,
		InvitedBy: input.InvitedBy,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	if err := uc.inviteRepo.Create(ctx, invite); err != nil {
		return nil, fmt.Errorf("create invite: %w", err)
	}
	invite.Token = ""

	out := &CreateInviteOutput{Invite: invite, Token: token, ExpiresAt: expiresAt, InviteURL: uc.inviteURL(token)}
	out.EmailSent = uc.sendInviteEmail(ctx, tenant.Name, input.Email, input.Role, out.InviteURL)
	return out, nil
}

func (uc *InviteUseCase) sendInviteEmail(ctx context.Context, tenantName, to string, role domain.UserRole, link string) bool {
	if uc.emailProvider == nil {
		return false
	}
	subject := fmt.Sprintf("Convite para acessar o Voltiq - %s", tenantName)
	body := fmt.Sprintf(`<p>Você foi convidado(a) para acessar o Voltiq da <strong>%s</strong> com o papel <strong>%s</strong>.</p>
<p><a href="%s" style="display:inline-block;background:#1a7a4a;color:#fff;padding:12px 24px;border-radius:8px;text-decoration:none">Aceitar convite</a></p>
<p style="color:#64748b;font-size:12px">O convite expira em 7 dias. Se você não esperava este e-mail, ignore-o.</p>`,
		html.EscapeString(tenantName), html.EscapeString(string(role)), html.EscapeString(link))
	err := uc.emailProvider.SendEmail(ctx, email.SendEmailInput{
		To: []string{to}, Subject: subject, HTMLBody: body,
		TextBody: fmt.Sprintf("Você foi convidado(a) para acessar o Voltiq da %s. Aceite em: %s", tenantName, link),
	})
	if err != nil {
		slog.Error("failed to send invite email", "error", err)
		return false
	}
	return true
}

// ValidateInvite validates an invite token
func (uc *InviteUseCase) ValidateInvite(ctx context.Context, token string) (*domain.Invite, error) {
	invite, err := uc.inviteRepo.GetByToken(ctx, token)
	if err != nil || invite == nil {
		return nil, ErrInviteNotFound
	}

	if invite.Status != domain.InviteStatusPending {
		if invite.Status == domain.InviteStatusAccepted {
			return nil, ErrInviteAlreadyUsed
		}
		return nil, ErrInviteCancelled
	}

	if time.Now().After(invite.ExpiresAt) {
		return nil, ErrInviteExpired
	}

	invite.Token = ""
	return invite, nil
}

// TenantName returns the name of the tenant of an invite (for the accept page).
func (uc *InviteUseCase) TenantName(ctx context.Context, tenantID domain.UUID) string {
	if t, err := uc.tenantRepo.GetByID(ctx, tenantID); err == nil && t != nil {
		return t.Name
	}
	return ""
}

// AcceptInvite accepts an invitation and creates the user
func (uc *InviteUseCase) AcceptInvite(ctx context.Context, input AcceptInviteInput) (*domain.User, error) {
	invite, err := uc.ValidateInvite(ctx, input.Token)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if len(name) < 2 {
		return nil, &domain.ValidationError{Fields: []domain.FieldError{{Field: "name", Message: "informe seu nome"}}}
	}
	if err := ValidatePassword(input.Password); err != nil {
		return nil, &domain.ValidationError{Fields: []domain.FieldError{{Field: "password", Message: err.Error()}}}
	}

	if existingUser, _ := uc.userRepo.GetByEmail(ctx, invite.Email); existingUser != nil {
		return nil, ErrEmailAlreadyInUse
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	now := time.Now()
	user := &domain.User{
		ID:           domain.UUID(uuid.New().String()),
		TenantID:     invite.TenantID,
		Email:        invite.Email,
		Name:         name,
		PasswordHash: string(hashedPassword),
		Role:         invite.Role,
		Active:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := uc.userRepo.Create(ctx, user); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailAlreadyInUse
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	invite.Status = domain.InviteStatusAccepted
	invite.AcceptedAt = &now
	if err := uc.inviteRepo.Update(ctx, invite); err != nil {
		return nil, fmt.Errorf("update invite: %w", err)
	}

	return user, nil
}

// CancelInvite cancels a pending invitation of the request tenant
func (uc *InviteUseCase) CancelInvite(ctx context.Context, inviteID domain.UUID) error {
	invite, err := uc.inviteRepo.GetByID(ctx, inviteID)
	if err != nil || invite == nil {
		return ErrInviteNotFound
	}
	if invite.Status != domain.InviteStatusPending {
		return ErrInviteAlreadyUsed
	}
	invite.Status = domain.InviteStatusCancelled
	return uc.inviteRepo.Update(ctx, invite)
}

// ResendInvite renews the token and expiry of a pending invite and sends it
// again.
func (uc *InviteUseCase) ResendInvite(ctx context.Context, inviteID domain.UUID) (*CreateInviteOutput, error) {
	invite, err := uc.inviteRepo.GetByID(ctx, inviteID)
	if err != nil || invite == nil {
		return nil, ErrInviteNotFound
	}
	if invite.Status != domain.InviteStatusPending {
		return nil, ErrInviteAlreadyUsed
	}
	token := uuid.New().String() + uuid.New().String()[:8]
	expiresAt := time.Now().Add(inviteTTL)
	if err := uc.inviteRepo.RenewToken(ctx, inviteID, repository.HashToken(token), expiresAt); err != nil {
		return nil, ErrInviteNotFound
	}
	invite.ExpiresAt = expiresAt
	out := &CreateInviteOutput{Invite: invite, Token: token, ExpiresAt: expiresAt, InviteURL: uc.inviteURL(token)}
	out.EmailSent = uc.sendInviteEmail(ctx, uc.TenantName(ctx, invite.TenantID), invite.Email, invite.Role, out.InviteURL)
	return out, nil
}

// ListPending lists the pending invites of a tenant.
func (uc *InviteUseCase) ListPending(ctx context.Context, tenantID domain.UUID) ([]*domain.Invite, error) {
	return uc.inviteRepo.ListPending(ctx, tenantID)
}
