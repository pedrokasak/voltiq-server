package repository

import (
	"context"
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
		INSERT INTO tenants (id, name, document, plan, trial_until, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		tenant.ID,
		tenant.Name,
		tenant.Document,
		tenant.Plan,
		tenant.TrialUntil,
		tenant.Active,
		tenant.CreatedAt,
		tenant.UpdatedAt,
	)

	return err
}

// GetByID retrieves a tenant by ID
func (r *TenantRepository) GetByID(ctx context.Context, id domain.UUID) (*domain.Tenant, error) {
	query := `
		SELECT id, name, document, plan, trial_until, active, created_at, updated_at
		FROM tenants
		WHERE id = $1
	`

	tenant := &domain.Tenant{}
	var trialUntil pgtype.Timestamptz

	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Document,
		&tenant.Plan,
		&trialUntil,
		&tenant.Active,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if trialUntil.Valid {
		tenant.TrialUntil = trialUntil.Time
	}

	return tenant, nil
}

// GetByDocument retrieves a tenant by document
func (r *TenantRepository) GetByDocument(ctx context.Context, document string) (*domain.Tenant, error) {
	query := `
		SELECT id, name, document, plan, trial_until, active, created_at, updated_at
		FROM tenants
		WHERE document = $1
	`

	tenant := &domain.Tenant{}
	var trialUntil pgtype.Timestamptz

	err := r.db.Pool.QueryRow(ctx, query, document).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Document,
		&tenant.Plan,
		&trialUntil,
		&tenant.Active,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if trialUntil.Valid {
		tenant.TrialUntil = trialUntil.Time
	}

	return tenant, nil
}

// Update updates an existing tenant
func (r *TenantRepository) Update(ctx context.Context, tenant *domain.Tenant) error {
	query := `
		UPDATE tenants
		SET name = $1, document = $2, plan = $3, trial_until = $4, active = $5, updated_at = $6
		WHERE id = $7
	`

	_, err := r.db.Pool.Exec(ctx, query,
		tenant.Name,
		tenant.Document,
		tenant.Plan,
		tenant.TrialUntil,
		tenant.Active,
		time.Now(),
		tenant.ID,
	)

	return err
}
