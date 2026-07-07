package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrTenantExists = errors.New("tenant with this document already exists")
	ErrInvalidPlan  = errors.New("invalid tenant plan")
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

// SignupUseCase handles tenant and admin user creation (self-service onboarding)
type SignupUseCase struct {
	tenantRepo *repository.TenantRepository
	userRepo   *repository.UserRepository
}

// SignupInput contains data for self-service signup
type SignupInput struct {
	TenantName     string
	TenantDocument string
	Plan           domain.TenantPlan
	AdminName      string
	AdminEmail     string
	AdminPassword  string
}

// SignupOutput contains the created tenant and admin user
type SignupOutput struct {
	Tenant *domain.Tenant
	User   *domain.User
}

// NewSignupUseCase creates a new SignupUseCase
func NewSignupUseCase(
	tenantRepo *repository.TenantRepository,
	userRepo *repository.UserRepository,
) *SignupUseCase {
	return &SignupUseCase{
		tenantRepo: tenantRepo,
		userRepo:   userRepo,
	}
}

// Signup creates a new tenant and admin user
func (uc *SignupUseCase) Signup(ctx context.Context, input SignupInput) (*SignupOutput, error) {
	if input.Plan == "" {
		input.Plan = domain.TenantPlanTrial
	}

	if input.Plan != domain.TenantPlanTrial &&
		input.Plan != domain.TenantPlanStarter &&
		input.Plan != domain.TenantPlanPro &&
		input.Plan != domain.TenantPlanEnterprise {
		return nil, ErrInvalidPlan
	}

	existingTenant, _ := uc.tenantRepo.GetByDocument(ctx, input.TenantDocument)
	if existingTenant != nil {
		return nil, ErrTenantExists
	}

	trialUntil := time.Now().Add(30 * 24 * time.Hour)
	if input.Plan != domain.TenantPlanTrial {
		trialUntil = time.Time{}
	}

	tenant := &domain.Tenant{
		ID:         domain.UUID(uuid.New().String()),
		Name:       input.TenantName,
		Document:   input.TenantDocument,
		Plan:       input.Plan,
		TrialUntil: trialUntil,
		Active:     true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := uc.tenantRepo.Create(ctx, tenant); err != nil {
		return nil, errors.New("failed to create tenant")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	user := &domain.User{
		ID:           domain.UUID(uuid.New().String()),
		TenantID:     tenant.ID,
		Email:        input.AdminEmail,
		Name:         input.AdminName,
		PasswordHash: string(hashedPassword),
		Role:         domain.UserRoleAdmin,
		Active:       true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := uc.userRepo.Create(ctx, user); err != nil {
		return nil, errors.New("failed to create admin user")
	}

	return &SignupOutput{
		Tenant: tenant,
		User:   user,
	}, nil
}
