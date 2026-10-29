package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// ParseTenantPlan parses a string to TenantPlan
func ParseTenantPlan(s string) domain.TenantPlan {
	switch domain.TenantPlan(s) {
	case domain.TenantPlanStarter, domain.TenantPlanPro, domain.TenantPlanEnterprise:
		return domain.TenantPlan(s)
	default:
		return domain.TenantPlanTrial
	}
}

// TrialDays returns the trial length (RF08.1: 14 dias), configurable with
// TRIAL_DAYS.
func TrialDays() int {
	if v, err := strconv.Atoi(os.Getenv("TRIAL_DAYS")); err == nil && v > 0 && v <= 90 {
		return v
	}
	return 14
}

// MinPasswordLength follows SDD 05-SEGURANCA (minimo 8 caracteres).
const MinPasswordLength = 8

// SignupUseCase handles tenant and owner creation (self-service onboarding)
type SignupUseCase struct {
	db         *repository.Database
	tenantRepo *repository.TenantRepository
	userRepo   *repository.UserRepository
}

// SignupInput contains data for self-service signup
type SignupInput struct {
	TenantName     string
	TenantDocument string
	// Plan is the plan the customer intends to buy; self-service signups
	// always start in trial and are activated through billing.
	Plan          domain.TenantPlan
	AdminName     string
	AdminEmail    string
	AdminPassword string
}

// SignupOutput contains the created tenant and owner user
type SignupOutput struct {
	Tenant *domain.Tenant
	User   *domain.User
}

// NewSignupUseCase creates a new SignupUseCase
func NewSignupUseCase(db *repository.Database, tenantRepo *repository.TenantRepository, userRepo *repository.UserRepository) *SignupUseCase {
	return &SignupUseCase{db: db, tenantRepo: tenantRepo, userRepo: userRepo}
}

// ValidatePassword applies the password policy.
func ValidatePassword(p string) error {
	if len(p) < MinPasswordLength {
		return fmt.Errorf("a senha deve ter pelo menos %d caracteres", MinPasswordLength)
	}
	var letter, digit bool
	for _, c := range p {
		switch {
		case unicode.IsLetter(c):
			letter = true
		case unicode.IsDigit(c):
			digit = true
		}
	}
	if !letter || !digit {
		return errors.New("a senha deve conter letras e números")
	}
	return nil
}

func validateSignup(in *SignupInput) error {
	v := &domain.ValidationError{}
	in.TenantName = strings.TrimSpace(in.TenantName)
	in.AdminName = strings.TrimSpace(in.AdminName)
	in.TenantDocument = strings.TrimSpace(in.TenantDocument)
	if len(in.TenantName) < 2 {
		v.Fields = append(v.Fields, domain.FieldError{Field: "tenant_name", Message: "informe o nome da empresa"})
	}
	if len(in.AdminName) < 2 {
		v.Fields = append(v.Fields, domain.FieldError{Field: "admin_name", Message: "informe seu nome"})
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(in.AdminEmail))
	if err != nil {
		v.Fields = append(v.Fields, domain.FieldError{Field: "admin_email", Message: "e-mail inválido"})
	} else {
		in.AdminEmail = strings.ToLower(addr.Address)
	}
	if err := ValidatePassword(in.AdminPassword); err != nil {
		v.Fields = append(v.Fields, domain.FieldError{Field: "admin_password", Message: err.Error()})
	}
	if in.TenantDocument != "" {
		digits := 0
		for _, c := range in.TenantDocument {
			if unicode.IsDigit(c) {
				digits++
			}
		}
		if digits != 14 && digits != 11 {
			v.Fields = append(v.Fields, domain.FieldError{Field: "tenant_document", Message: "CNPJ (14 dígitos) ou CPF (11 dígitos) inválido"})
		}
	}
	return v.OrNil()
}

// Signup cria o tenant em trial (RF08.1) e o usuario OWNER numa unica
// transacao.
func (uc *SignupUseCase) Signup(ctx context.Context, input SignupInput) (*SignupOutput, error) {
	if err := validateSignup(&input); err != nil {
		return nil, err
	}

	if input.TenantDocument != "" {
		if existing, _ := uc.tenantRepo.GetByDocument(ctx, input.TenantDocument); existing != nil {
			return nil, ErrTenantExists
		}
	}
	if existing, _ := uc.userRepo.GetByEmail(ctx, input.AdminEmail); existing != nil {
		return nil, ErrEmailAlreadyInUse
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.AdminPassword), 12)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	now := time.Now()
	trialEnd := now.AddDate(0, 0, TrialDays())
	features := map[string]any{}
	if input.Plan != "" && input.Plan != domain.TenantPlanTrial {
		features["intended_plan"] = string(input.Plan)
	}
	tenant := &domain.Tenant{
		ID:             domain.UUID(uuid.New().String()),
		Name:           input.TenantName,
		Document:       input.TenantDocument,
		Plan:           domain.TenantPlanTrial,
		Status:         domain.TenantStatusTrial,
		TrialUntil:     trialEnd,
		TrialExpiresAt: &trialEnd,
		MaxUsers:       domain.LimitsForPlan(domain.TenantPlanTrial).MaxUsers,
		SeatCount:      1,
		Features:       features,
		Active:         true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	user := &domain.User{
		ID:           domain.UUID(uuid.New().String()),
		TenantID:     tenant.ID,
		Email:        input.AdminEmail,
		Name:         input.AdminName,
		PasswordHash: string(hashedPassword),
		Role:         domain.UserRoleOwner,
		Active:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err = uc.db.InTx(ctx, func(tx pgx.Tx) error {
		if err := uc.tenantRepo.CreateTx(ctx, tx, tenant); err != nil {
			return fmt.Errorf("create tenant: %w", err)
		}
		if err := uc.userRepo.CreateTx(ctx, tx, user); err != nil {
			return fmt.Errorf("create owner: %w", err)
		}
		return nil
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailAlreadyInUse
		}
		slog.Error("signup failed", "error", err)
		return nil, errors.New("failed to create account")
	}

	slog.Info("tenant created", "tenant_id", tenant.ID, "trial_days", TrialDays())
	return &SignupOutput{Tenant: tenant, User: user}, nil
}
