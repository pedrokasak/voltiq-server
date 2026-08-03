package usecase

import (
	"context"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

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

// ListTenantsFilter holds filtering and pagination options for listing tenants
type ListTenantsFilter = domain.ListTenantsFilter

// TenantListResult holds the paginated result
type TenantListResult = domain.TenantListResult

// ListTenants lists tenants with filtering and pagination
func (uc *SuperAdminUseCase) ListTenants(ctx context.Context, filter ListTenantsFilter) (*TenantListResult, error) {
	// Default values
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	tenants, total, err := uc.tenantRepo.ListWithFilters(ctx, filter)
	if err != nil {
		return nil, err
	}

	totalPages := (total + filter.Limit - 1) / filter.Limit

	return &TenantListResult{
		Data:       tenants,
		Total:      total,
		Page:       filter.Page,
		Limit:      filter.Limit,
		TotalPages: totalPages,
	}, nil
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

// ActivateTenant activates a tenant (TRIAL -> ACTIVE or SUSPENDED -> ACTIVE)
func (uc *SuperAdminUseCase) ActivateTenant(ctx context.Context, id domain.UUID, plan domain.TenantPlan) (*domain.Tenant, error) {
	tenant, err := uc.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, ErrTenantNotFound
	}

	if tenant.Status == domain.TenantStatusActive {
		return nil, ErrTenantAlreadyActive
	}

	// Update plan if provided
	if plan != "" {
		tenant.Plan = plan
		tenant.MaxUsers = getMaxUsersForPlan(plan)
	}

	tenant.Status = domain.TenantStatusActive
	now := time.Now()
	tenant.ActivatedAt = &now
	tenant.UpdatedAt = now

	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}

	return tenant, nil
}

// UpdateTenantPlan updates the tenant's plan
func (uc *SuperAdminUseCase) UpdateTenantPlan(ctx context.Context, id domain.UUID, plan domain.TenantPlan) (*domain.Tenant, error) {
	tenant, err := uc.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, ErrTenantNotFound
	}

	// Check seat limit if downgrading
	newMaxUsers := getMaxUsersForPlan(plan)
	if tenant.SeatCount > newMaxUsers {
		return nil, ErrSeatLimitExceeded
	}

	tenant.Plan = plan
	tenant.MaxUsers = newMaxUsers
	tenant.UpdatedAt = time.Now()

	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}

	return tenant, nil
}

// UpdateTenantStatus updates the tenant's status manually
func (uc *SuperAdminUseCase) UpdateTenantStatus(ctx context.Context, id domain.UUID, status string, reason string) (*domain.Tenant, error) {
	tenant, err := uc.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, ErrTenantNotFound
	}

	tenant.Status = domain.TenantStatus(status)
	tenant.UpdatedAt = time.Now()

	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}

	// TODO: Log audit trail with reason, changed_by, etc.
	// This would go to an audit log table

	return tenant, nil
}

// getMaxUsersForPlan returns the max_users for a given plan
func getMaxUsersForPlan(plan domain.TenantPlan) int {
	switch plan {
	case domain.TenantPlanTrial:
		return 5
	case domain.TenantPlanStarter:
		return 10
	case domain.TenantPlanPro:
		return 50
	case domain.TenantPlanEnterprise:
		return 999999 // effectively unlimited
	default:
		return 5
	}
}