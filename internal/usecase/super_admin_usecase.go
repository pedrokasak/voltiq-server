package usecase

import (
	"context"
	"errors"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// ErrTenantNotFound indicates the tenant does not exist
var ErrTenantNotFound = errors.New("tenant not found")

// SuperAdminUseCase handles cross-tenant administrative operations.
// Access is gated by the SUPER_ADMIN role at the router level.
type SuperAdminUseCase struct {
	tenantRepo *repository.TenantRepository
	userRepo   *repository.UserRepository
}

// NewSuperAdminUseCase creates a new SuperAdminUseCase
func NewSuperAdminUseCase(
	tenantRepo *repository.TenantRepository,
	userRepo *repository.UserRepository,
) *SuperAdminUseCase {
	return &SuperAdminUseCase{
		tenantRepo: tenantRepo,
		userRepo:   userRepo,
	}
}

// ListTenants lists every tenant on the platform (cross-tenant)
func (uc *SuperAdminUseCase) ListTenants(ctx context.Context) ([]*domain.Tenant, error) {
	tenants, err := uc.tenantRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return tenants, nil
}

// GetTenantByID retrieves a single tenant by ID across the platform
func (uc *SuperAdminUseCase) GetTenantByID(ctx context.Context, id domain.UUID) (*domain.Tenant, error) {
	t, err := uc.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTenantNotFound
	}
	return t, nil
}

// ListTenantUsers lists all users belonging to a tenant
func (uc *SuperAdminUseCase) ListTenantUsers(ctx context.Context, tenantID domain.UUID) ([]*domain.User, error) {
	// Validate the tenant exists first so we return 404 instead of an empty list
	t, err := uc.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTenantNotFound
	}

	users, err := uc.userRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return users, nil
}
