package repository

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
)

// SubstationRepository handles substation data access
type SubstationRepository struct {
	db *Database
}

// NewSubstationRepository creates a new SubstationRepository
func NewSubstationRepository(db *Database) *SubstationRepository {
	return &SubstationRepository{db: db}
}

// SubstationSummary is a substation with transformer totals.
type SubstationSummary struct {
	domain.Substation
	TransformerCount int     `json:"transformer_count"`
	TotalKVA         float64 `json:"total_kva"`
	CriticalCount    int     `json:"critical_count"`
	WarningCount     int     `json:"warning_count"`
}

// List returns the substations of a tenant with transformer totals.
func (r *SubstationRepository) List(ctx context.Context, tenantID domain.UUID) ([]*SubstationSummary, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT s.id, s.tenant_id, s.code, s.name, s.lat, s.lng, s.created_at, COALESCE(s.updated_at, s.created_at), s.deleted_at,
			COUNT(t.id), COALESCE(SUM(t.power_kva), 0)::float8,
			COUNT(*) FILTER (WHERE b.status = 'CRITICAL'),
			COUNT(*) FILTER (WHERE b.status = 'WARNING')
		FROM substations s
		LEFT JOIN transformers t ON t.substation_id = s.id AND t.deleted_at IS NULL
		LEFT JOIN LATERAL (
			SELECT status FROM transformer_balance WHERE transformer_id = t.id ORDER BY period_start DESC LIMIT 1
		) b ON true
		WHERE s.tenant_id = $1 AND s.deleted_at IS NULL
		GROUP BY s.id
		ORDER BY s.code`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SubstationSummary
	for rows.Next() {
		s := &SubstationSummary{}
		if err := rows.Scan(&s.ID, &s.TenantID, &s.Code, &s.Name, &s.Lat, &s.Lng, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
			&s.TransformerCount, &s.TotalKVA, &s.CriticalCount, &s.WarningCount); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetByID retrieves a substation of the request tenant.
func (r *SubstationRepository) GetByID(ctx context.Context, id domain.UUID) (*domain.Substation, error) {
	s := &domain.Substation{}
	q, args := scoped(ctx, `SELECT id, tenant_id, code, name, lat, lng, created_at, COALESCE(updated_at, created_at), deleted_at
		FROM substations WHERE id = $1 AND deleted_at IS NULL /*tenant*/`, id)
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(&s.ID, &s.TenantID, &s.Code, &s.Name, &s.Lat, &s.Lng, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Create inserts a substation.
func (r *SubstationRepository) Create(ctx context.Context, s *domain.Substation) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO substations (id, tenant_id, code, name, lat, lng, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`,
		s.ID, s.TenantID, s.Code, s.Name, s.Lat, s.Lng, time.Now())
	return err
}

// Update changes name, code and location of a substation.
func (r *SubstationRepository) Update(ctx context.Context, s *domain.Substation) error {
	q, args := scoped(ctx, `UPDATE substations SET code = $1, name = $2, lat = $3, lng = $4, updated_at = NOW()
		WHERE id = $5 AND deleted_at IS NULL /*tenant*/`, s.Code, s.Name, s.Lat, s.Lng, s.ID)
	tag, err := r.db.Pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete soft-deletes a substation and unlinks its transformers.
func (r *SubstationRepository) Delete(ctx context.Context, id domain.UUID) error {
	return r.db.InTx(ctx, func(tx pgx.Tx) error {
		q, args := scoped(ctx, `UPDATE substations SET deleted_at = NOW(), updated_at = NOW()
			WHERE id = $1 AND deleted_at IS NULL /*tenant*/`, id)
		tag, err := tx.Exec(ctx, q, args...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, `UPDATE transformers SET substation_id = NULL, updated_at = NOW() WHERE substation_id = $1`, id)
		return err
	})
}

// GetOrCreateByCode returns the substation with code, creating it (name =
// code) when the tenant does not have it yet.
func (r *SubstationRepository) GetOrCreateByCode(ctx context.Context, tx pgx.Tx, tenantID domain.UUID, code string) (domain.UUID, bool, error) {
	code = strings.TrimSpace(code)
	var id domain.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM substations WHERE tenant_id = $1 AND code = $2`, tenantID, code).Scan(&id)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE substations SET deleted_at = NULL WHERE id = $1 AND deleted_at IS NOT NULL`, id)
		return id, false, err
	}
	if err != pgx.ErrNoRows {
		return "", false, err
	}
	id = domain.UUID(uuid.New().String())
	_, err = tx.Exec(ctx, `INSERT INTO substations (id, tenant_id, code, name, created_at, updated_at)
		VALUES ($1, $2, $3, $3, NOW(), NOW())`, id, tenantID, code)
	return id, true, err
}
