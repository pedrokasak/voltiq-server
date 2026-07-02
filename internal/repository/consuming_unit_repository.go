package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/energybalance/server/internal/domain"
)

// ConsumingUnitRepository handles consuming unit data access
type ConsumingUnitRepository struct {
	db *Database
}

// NewConsumingUnitRepository creates a new ConsumingUnitRepository
func NewConsumingUnitRepository(db *Database) *ConsumingUnitRepository {
	return &ConsumingUnitRepository{db: db}
}

// Create inserts a new consuming unit into the database
func (r *ConsumingUnitRepository) Create(ctx context.Context, uc *domain.ConsumingUnit) error {
	query := `
		INSERT INTO consuming_units (id, tenant_id, transformer_id, uc_code, name, class, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		uc.ID,
		uc.TenantID,
		uc.TransformerID,
		uc.UCCode,
		uc.Name,
		uc.Class,
		uc.Active,
		uc.CreatedAt,
		uc.UpdatedAt,
	)

	return err
}

// GetByID retrieves a consuming unit by ID
func (r *ConsumingUnitRepository) GetByID(ctx context.Context, id domain.UUID) (*domain.ConsumingUnit, error) {
	query := `
		SELECT id, tenant_id, transformer_id, uc_code, name, class, active, created_at, updated_at, deleted_at
		FROM consuming_units
		WHERE id = $1 AND deleted_at IS NULL
	`

	uc := &domain.ConsumingUnit{}

	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&uc.ID,
		&uc.TenantID,
		&uc.TransformerID,
		&uc.UCCode,
		&uc.Name,
		&uc.Class,
		&uc.Active,
		&uc.CreatedAt,
		&uc.UpdatedAt,
		&uc.DeletedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return uc, nil
}

// GetByTransformer retrieves all consuming units for a transformer
func (r *ConsumingUnitRepository) GetByTransformer(ctx context.Context, transformerID domain.UUID) ([]*domain.ConsumingUnit, error) {
	query := `
		SELECT id, tenant_id, transformer_id, uc_code, name, class, active, created_at, updated_at, deleted_at
		FROM consuming_units
		WHERE transformer_id = $1 AND deleted_at IS NULL
		ORDER BY uc_code
	`

	rows, err := r.db.Pool.Query(ctx, query, transformerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ucs []*domain.ConsumingUnit
	for rows.Next() {
		uc := &domain.ConsumingUnit{}

		err := rows.Scan(
			&uc.ID,
			&uc.TenantID,
			&uc.TransformerID,
			&uc.UCCode,
			&uc.Name,
			&uc.Class,
			&uc.Active,
			&uc.CreatedAt,
			&uc.UpdatedAt,
			&uc.DeletedAt,
		)
		if err != nil {
			return nil, err
		}

		ucs = append(ucs, uc)
	}

	return ucs, nil
}

// GetByTenant retrieves all consuming units for a tenant
func (r *ConsumingUnitRepository) GetByTenant(ctx context.Context, tenantID domain.UUID) ([]*domain.ConsumingUnit, error) {
	query := `
		SELECT id, tenant_id, transformer_id, uc_code, name, class, active, created_at, updated_at, deleted_at
		FROM consuming_units
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY uc_code
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ucs []*domain.ConsumingUnit
	for rows.Next() {
		uc := &domain.ConsumingUnit{}

		err := rows.Scan(
			&uc.ID,
			&uc.TenantID,
			&uc.TransformerID,
			&uc.UCCode,
			&uc.Name,
			&uc.Class,
			&uc.Active,
			&uc.CreatedAt,
			&uc.UpdatedAt,
			&uc.DeletedAt,
		)
		if err != nil {
			return nil, err
		}

		ucs = append(ucs, uc)
	}

	return ucs, nil
}

// Update updates an existing consuming unit
func (r *ConsumingUnitRepository) Update(ctx context.Context, uc *domain.ConsumingUnit) error {
	query := `
		UPDATE consuming_units
		SET transformer_id = $1, uc_code = $2, name = $3, class = $4, active = $5, updated_at = $6
		WHERE id = $7
	`

	_, err := r.db.Pool.Exec(ctx, query,
		uc.TransformerID,
		uc.UCCode,
		uc.Name,
		uc.Class,
		uc.Active,
		time.Now(),
		uc.ID,
	)

	return err
}

// Delete soft deletes a consuming unit
func (r *ConsumingUnitRepository) Delete(ctx context.Context, id domain.UUID) error {
	query := `
		UPDATE consuming_units
		SET deleted_at = $1, updated_at = $2
		WHERE id = $3
	`

	_, err := r.db.Pool.Exec(ctx, query, time.Now(), time.Now(), id)
	return err
}
