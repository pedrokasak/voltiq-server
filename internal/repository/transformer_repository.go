package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
)

// TransformerRepository handles transformer data access
type TransformerRepository struct {
	db *Database
}

// NewTransformerRepository creates a new TransformerRepository
func NewTransformerRepository(db *Database) *TransformerRepository {
	return &TransformerRepository{db: db}
}

// Create inserts a new transformer into the database
func (r *TransformerRepository) Create(ctx context.Context, transformer *domain.Transformer) error {
	query := `
		INSERT INTO transformers (
			id, tenant_id, substation_id, code, power_kva, primary_voltage_kv, 
			secondary_voltage_v, lat, lng, core_loss_kw, winding_loss_kw, 
			loss_limit_pct, active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		transformer.ID,
		transformer.TenantID,
		transformer.SubstationID,
		transformer.Code,
		transformer.PowerKVA,
		transformer.PrimaryVoltageKV,
		transformer.SecondaryVoltageV,
		transformer.Lat,
		transformer.Lng,
		transformer.CoreLossKW,
		transformer.WindingLossKW,
		transformer.LossLimitPct,
		transformer.Active,
		transformer.CreatedAt,
		transformer.UpdatedAt,
	)

	return err
}

// GetByID retrieves a transformer by ID
func (r *TransformerRepository) GetByID(ctx context.Context, id domain.UUID) (*domain.Transformer, error) {
	query := `
		SELECT id, tenant_id, substation_id, code, power_kva, primary_voltage_kv,
			secondary_voltage_v, lat, lng, core_loss_kw, winding_loss_kw,
			loss_limit_pct, active, created_at, updated_at, deleted_at
		FROM transformers
		WHERE id = $1 AND deleted_at IS NULL
	`

	transformer := &domain.Transformer{}

	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&transformer.ID,
		&transformer.TenantID,
		&transformer.SubstationID,
		&transformer.Code,
		&transformer.PowerKVA,
		&transformer.PrimaryVoltageKV,
		&transformer.SecondaryVoltageV,
		&transformer.Lat,
		&transformer.Lng,
		&transformer.CoreLossKW,
		&transformer.WindingLossKW,
		&transformer.LossLimitPct,
		&transformer.Active,
		&transformer.CreatedAt,
		&transformer.UpdatedAt,
		&transformer.DeletedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return transformer, nil
}

// GetByTenant retrieves all transformers for a tenant
func (r *TransformerRepository) GetByTenant(ctx context.Context, tenantID domain.UUID) ([]*domain.Transformer, error) {
	query := `
		SELECT id, tenant_id, substation_id, code, power_kva, primary_voltage_kv,
			secondary_voltage_v, lat, lng, core_loss_kw, winding_loss_kw,
			loss_limit_pct, active, created_at, updated_at, deleted_at
		FROM transformers
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY code
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transformers []*domain.Transformer
	for rows.Next() {
		transformer := &domain.Transformer{}

		err := rows.Scan(
			&transformer.ID,
			&transformer.TenantID,
			&transformer.SubstationID,
			&transformer.Code,
			&transformer.PowerKVA,
			&transformer.PrimaryVoltageKV,
			&transformer.SecondaryVoltageV,
			&transformer.Lat,
			&transformer.Lng,
			&transformer.CoreLossKW,
			&transformer.WindingLossKW,
			&transformer.LossLimitPct,
			&transformer.Active,
			&transformer.CreatedAt,
			&transformer.UpdatedAt,
			&transformer.DeletedAt,
		)
		if err != nil {
			return nil, err
		}

		transformers = append(transformers, transformer)
	}

	return transformers, nil
}

// Update updates an existing transformer
func (r *TransformerRepository) Update(ctx context.Context, transformer *domain.Transformer) error {
	query := `
		UPDATE transformers
		SET substation_id = $1, code = $2, power_kva = $3, primary_voltage_kv = $4,
			secondary_voltage_v = $5, lat = $6, lng = $7, core_loss_kw = $8,
			winding_loss_kw = $9, loss_limit_pct = $10, active = $11, updated_at = $12
		WHERE id = $13
	`

	_, err := r.db.Pool.Exec(ctx, query,
		transformer.SubstationID,
		transformer.Code,
		transformer.PowerKVA,
		transformer.PrimaryVoltageKV,
		transformer.SecondaryVoltageV,
		transformer.Lat,
		transformer.Lng,
		transformer.CoreLossKW,
		transformer.WindingLossKW,
		transformer.LossLimitPct,
		transformer.Active,
		time.Now(),
		transformer.ID,
	)

	return err
}

// Delete soft deletes a transformer
func (r *TransformerRepository) Delete(ctx context.Context, id domain.UUID) error {
	query := `
		UPDATE transformers
		SET deleted_at = $1, updated_at = $2
		WHERE id = $3
	`

	_, err := r.db.Pool.Exec(ctx, query, time.Now(), time.Now(), id)
	return err
}
