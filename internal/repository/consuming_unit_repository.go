package repository

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
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
		WHERE id = $1 AND deleted_at IS NULL /*tenant*/
	`

	uc := &domain.ConsumingUnit{}

	q, args := scoped(ctx, query, id)
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(
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
		WHERE transformer_id = $1 AND deleted_at IS NULL /*tenant*/
		ORDER BY uc_code
	`

	q, args := scoped(ctx, query, transformerID)
	rows, err := r.db.Pool.Query(ctx, q, args...)
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
		WHERE id = $7 /*tenant*/
	`

	q, args := scoped(ctx, query,
		uc.TransformerID,
		uc.UCCode,
		uc.Name,
		uc.Class,
		uc.Active,
		time.Now(),
		uc.ID,
	)
	_, err := r.db.Pool.Exec(ctx, q, args...)

	return err
}

// Delete soft deletes a consuming unit
func (r *ConsumingUnitRepository) Delete(ctx context.Context, id domain.UUID) error {
	query := `
		UPDATE consuming_units
		SET deleted_at = $1, updated_at = $2
		WHERE id = $3 AND deleted_at IS NULL /*tenant*/
	`

	now := time.Now()
	q, args := scoped(ctx, query, now, now, id)
	tag, err := r.db.Pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UCRef identifies a consuming unit and its transformer.
type UCRef struct {
	ID            domain.UUID
	TransformerID domain.UUID
	Code          string
}

// RefsByTenant returns every non-deleted UC of a tenant indexed by
// upper-cased code and by ID, for import resolution.
func (r *ConsumingUnitRepository) RefsByTenant(ctx context.Context, tenantID domain.UUID) (map[string]UCRef, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, transformer_id, uc_code FROM consuming_units WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]UCRef{}
	for rows.Next() {
		var ref UCRef
		if err := rows.Scan(&ref.ID, &ref.TransformerID, &ref.Code); err != nil {
			return nil, err
		}
		out[strings.ToUpper(ref.Code)] = ref
		out[strings.ToLower(string(ref.ID))] = ref
	}
	return out, rows.Err()
}

// CountByTenant returns the number of UCs of a tenant.
func (r *ConsumingUnitRepository) CountByTenant(ctx context.Context, tenantID domain.UUID) (int, error) {
	var n int
	err := r.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM consuming_units WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&n)
	return n, err
}

// UpsertByCode creates the UC or updates the existing one with the same
// code (restoring it if soft-deleted). Returns true when inserted.
func (r *ConsumingUnitRepository) UpsertByCode(ctx context.Context, tx pgx.Tx, u *domain.ConsumingUnit) (bool, error) {
	var inserted bool
	err := tx.QueryRow(ctx, `
		INSERT INTO consuming_units (id, tenant_id, transformer_id, uc_code, name, class, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, true, $7, $7)
		ON CONFLICT (tenant_id, uc_code) DO UPDATE SET
			transformer_id = EXCLUDED.transformer_id,
			name = COALESCE(EXCLUDED.name, consuming_units.name),
			class = COALESCE(EXCLUDED.class, consuming_units.class),
			active = true,
			deleted_at = NULL,
			updated_at = EXCLUDED.updated_at
		RETURNING id, (xmax = 0)`,
		u.ID, u.TenantID, u.TransformerID, u.UCCode, u.Name, u.Class, time.Now(),
	).Scan(&u.ID, &inserted)
	return inserted, err
}

// GetByCode retrieves a UC of a tenant by code.
func (r *ConsumingUnitRepository) GetByCode(ctx context.Context, tenantID domain.UUID, code string) (*domain.ConsumingUnit, error) {
	u := &domain.ConsumingUnit{}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, tenant_id, transformer_id, uc_code, COALESCE(name, ''), class, active, created_at, updated_at, deleted_at
		FROM consuming_units WHERE tenant_id = $1 AND uc_code = $2 AND deleted_at IS NULL`, tenantID, code,
	).Scan(&u.ID, &u.TenantID, &u.TransformerID, &u.UCCode, &u.Name, &u.Class, &u.Active, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}
