package repository

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/voltiq/server/internal/domain"
)

// TenantRepository handles tenant data access
type TenantRepository struct {
	db *Database
}

// NewTenantRepository creates a new TenantRepository
func NewTenantRepository(db *Database) *TenantRepository {
	return &TenantRepository{db: db}
}

// Create inserts a new tenant into the database
func (r *TenantRepository) Create(ctx context.Context, tenant *domain.Tenant) error {
	query := `
		INSERT INTO tenants (id, name, document, plan, status, trial_until, trial_expires_at, max_users, seat_count, features, active,
		                   payment_customer_id, payment_subscription_id, address, address_number, province, postal_code, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		tenant.ID,
		tenant.Name,
		tenant.Document,
		tenant.Plan,
		tenant.Status,
		tenant.TrialUntil,
		tenant.TrialExpiresAt,
		tenant.MaxUsers,
		tenant.SeatCount,
		tenant.Features,
		tenant.Active,
		tenant.PaymentCustomerID,
		tenant.PaymentSubscriptionID,
		tenant.Address,
		tenant.AddressNumber,
		tenant.Province,
		tenant.PostalCode,
		tenant.CreatedAt,
		tenant.UpdatedAt,
	)

	return err
}

// scanTenant scans a tenant from a row
func (r *TenantRepository) scanTenant(row interface {
	Scan(dest ...any) error
}, tenant *domain.Tenant, trialUntil, trialExpires, activatedAt, suspendedAt, cancelledAt *pgtype.Timestamptz, features *[]byte) error {
	err := row.Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Document,
		&tenant.Plan,
		&tenant.Status,
		trialUntil,
		trialExpires,
		&tenant.MaxUsers,
		&tenant.SeatCount,
		features,
		&tenant.Active,
		activatedAt,
		suspendedAt,
		cancelledAt,
		&tenant.PaymentCustomerID,
		&tenant.PaymentSubscriptionID,
		&tenant.Address,
		&tenant.AddressNumber,
		&tenant.Province,
		&tenant.PostalCode,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)
	if err != nil {
		return err
	}

	if trialUntil.Valid {
		tenant.TrialUntil = trialUntil.Time
	}
	if trialExpires.Valid {
		tenant.TrialExpiresAt = &trialExpires.Time
	}
	if activatedAt.Valid {
		tenant.ActivatedAt = &activatedAt.Time
	}
	if suspendedAt.Valid {
		tenant.SuspendedAt = &suspendedAt.Time
	}
	if cancelledAt.Valid {
		tenant.CancelledAt = &cancelledAt.Time
	}

	return nil
}

// baseSelectQuery returns the base SELECT query for tenants
func (r *TenantRepository) baseSelectQuery() string {
	return `
		SELECT id, name, document, plan, status, trial_until, trial_expires_at, max_users, seat_count,
		       features, active, activated_at, suspended_at, cancelled_at,
		       payment_customer_id, payment_subscription_id, address, address_number, province, postal_code,
		       created_at, updated_at
		FROM tenants
	`
}

// GetByID retrieves a tenant by ID
func (r *TenantRepository) GetByID(ctx context.Context, id domain.UUID) (*domain.Tenant, error) {
	query := r.baseSelectQuery() + ` WHERE id = $1`

	tenant := &domain.Tenant{}
	var trialUntil, trialExpires, activatedAt, suspendedAt, cancelledAt pgtype.Timestamptz
	var features []byte

	err := r.scanTenant(r.db.Pool.QueryRow(ctx, query, id), tenant, &trialUntil, &trialExpires, &activatedAt, &suspendedAt, &cancelledAt, &features)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return tenant, nil
}

// GetByDocument retrieves a tenant by document
func (r *TenantRepository) GetByDocument(ctx context.Context, document string) (*domain.Tenant, error) {
	query := r.baseSelectQuery() + ` WHERE document = $1`

	tenant := &domain.Tenant{}
	var trialUntil, trialExpires, activatedAt, suspendedAt, cancelledAt pgtype.Timestamptz
	var features []byte

	err := r.scanTenant(r.db.Pool.QueryRow(ctx, query, document), tenant, &trialUntil, &trialExpires, &activatedAt, &suspendedAt, &cancelledAt, &features)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return tenant, nil
}

// ListAll retrieves all tenants across the platform (cross-tenant, SUPER_ADMIN only)
func (r *TenantRepository) ListAll(ctx context.Context) ([]*domain.Tenant, error) {
	query := r.baseSelectQuery() + ` ORDER BY created_at DESC`

	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []*domain.Tenant
	for rows.Next() {
		tenant := &domain.Tenant{}
		var trialUntil, trialExpires, activatedAt, suspendedAt, cancelledAt pgtype.Timestamptz
		var features []byte

		err := r.scanTenant(rows, tenant, &trialUntil, &trialExpires, &activatedAt, &suspendedAt, &cancelledAt, &features)
		if err != nil {
			return nil, err
		}

		tenants = append(tenants, tenant)
	}

	return tenants, nil
}

// Update updates an existing tenant
func (r *TenantRepository) Update(ctx context.Context, tenant *domain.Tenant) error {
	query := `
		UPDATE tenants
		SET name = $1, document = $2, plan = $3, status = $4, trial_until = $5, trial_expires_at = $6,
		    max_users = $7, seat_count = $8, active = $9, features = $10, activated_at = $11,
		    suspended_at = $12, cancelled_at = $13, payment_customer_id = $14, payment_subscription_id = $15,
		    address = $16, address_number = $17, province = $18, postal_code = $19, updated_at = $20
		WHERE id = $21
	`

	_, err := r.db.Pool.Exec(ctx, query,
		tenant.Name,
		tenant.Document,
		tenant.Plan,
		tenant.Status,
		tenant.TrialUntil,
		tenant.TrialExpiresAt,
		tenant.MaxUsers,
		tenant.SeatCount,
		tenant.Active,
		tenant.Features,
		tenant.ActivatedAt,
		tenant.SuspendedAt,
		tenant.CancelledAt,
		tenant.PaymentCustomerID,
		tenant.PaymentSubscriptionID,
		tenant.Address,
		tenant.AddressNumber,
		tenant.Province,
		tenant.PostalCode,
		time.Now(),
		tenant.ID,
	)

	return err
}

// GetExpiredTrials retrieves tenants with TRIAL status and expired trial_expires_at
func (r *TenantRepository) GetExpiredTrials(ctx context.Context, now time.Time) ([]*domain.Tenant, error) {
	query := r.baseSelectQuery() + `
		WHERE status = $1 AND trial_expires_at IS NOT NULL AND trial_expires_at <= $2
	`

	rows, err := r.db.Pool.Query(ctx, query, domain.TenantStatusTrial, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []*domain.Tenant
	for rows.Next() {
		t := &domain.Tenant{}
		var trialUntil, trialExpires, activatedAt, suspendedAt, cancelledAt pgtype.Timestamptz
		var features []byte

		err := r.scanTenant(rows, t, &trialUntil, &trialExpires, &activatedAt, &suspendedAt, &cancelledAt, &features)
		if err != nil {
			return nil, err
		}

		tenants = append(tenants, t)
	}

	return tenants, nil
}

// ListWithFilters lists tenants with filtering and pagination
func (r *TenantRepository) ListWithFilters(ctx context.Context, filter domain.ListTenantsFilter) ([]*domain.Tenant, int, error) {
	// Build WHERE clause
	whereClause := "WHERE 1=1"
	args := []any{}
	argIndex := 1

	if filter.Status != "" {
		whereClause += " AND status = $" + strconv.Itoa(argIndex)
		args = append(args, filter.Status)
		argIndex++
	}

	if filter.Plan != "" {
		whereClause += " AND plan = $" + strconv.Itoa(argIndex)
		args = append(args, filter.Plan)
		argIndex++
	}

	if filter.Search != "" {
		whereClause += " AND (name ILIKE $" + strconv.Itoa(argIndex) + " OR document ILIKE $" + strconv.Itoa(argIndex) + ")"
		args = append(args, "%"+filter.Search+"%")
		argIndex++
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM tenants " + whereClause
	var total int
	err := r.db.Pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Build SELECT query with pagination
	offset := (filter.Page - 1) * filter.Limit
	query := r.baseSelectQuery() + `
		` + whereClause + `
		ORDER BY created_at DESC
		LIMIT $` + strconv.Itoa(argIndex) + ` OFFSET $` + strconv.Itoa(argIndex+1)

	args = append(args, filter.Limit, offset)

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tenants []*domain.Tenant
	for rows.Next() {
		tenant := &domain.Tenant{}
		var trialUntil, trialExpires, activatedAt, suspendedAt, cancelledAt pgtype.Timestamptz
		var features []byte

		err := r.scanTenant(rows, tenant, &trialUntil, &trialExpires, &activatedAt, &suspendedAt, &cancelledAt, &features)
		if err != nil {
			return nil, 0, err
		}

		tenants = append(tenants, tenant)
	}

	return tenants, total, nil
}

// GetByPaymentSubscriptionID retrieves a tenant by payment subscription ID
func (r *TenantRepository) GetByPaymentSubscriptionID(ctx context.Context, subscriptionID string) (*domain.Tenant, error) {
	query := r.baseSelectQuery() + ` WHERE payment_subscription_id = $1`

	tenant := &domain.Tenant{}
	var trialUntil, trialExpires, activatedAt, suspendedAt, cancelledAt pgtype.Timestamptz
	var features []byte

	err := r.scanTenant(r.db.Pool.QueryRow(ctx, query, subscriptionID), tenant, &trialUntil, &trialExpires, &activatedAt, &suspendedAt, &cancelledAt, &features)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return tenant, nil
}

// GetOwnerByTenant retrieves the owner user for a tenant
func (r *TenantRepository) GetOwnerByTenant(ctx context.Context, tenantID domain.UUID) (*domain.User, error) {
	// This should be in UserRepository, but keeping for billing use case
	return nil, nil
}