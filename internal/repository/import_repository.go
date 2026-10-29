package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
)

// ImportRepository handles import history data access
type ImportRepository struct {
	db *Database
}

// NewImportRepository creates a new ImportRepository
func NewImportRepository(db *Database) *ImportRepository {
	return &ImportRepository{db: db}
}

const importColumns = `id, tenant_id, user_id, file_name, total_rows, rows_ok, rows_error, status,
	errors_json, created_at, completed_at, COALESCE(reading_type, ''), column_mapping,
	duplicates_skipped, (file_content IS NOT NULL), updated_at`

func scanImport(row pgx.Row) (*domain.Import, error) {
	imp := &domain.Import{}
	var errorsJSON, mappingJSON []byte
	err := row.Scan(
		&imp.ID, &imp.TenantID, &imp.UserID, &imp.FileName, &imp.TotalRows, &imp.RowsOK, &imp.RowsError,
		&imp.Status, &errorsJSON, &imp.CreatedAt, &imp.CompletedAt, &imp.ReadingType, &mappingJSON,
		&imp.DuplicatesSkipped, &imp.HasFile, &imp.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(errorsJSON) > 0 {
		if err := json.Unmarshal(errorsJSON, &imp.ErrorsJSON); err != nil {
			return nil, err
		}
	}
	if len(mappingJSON) > 0 {
		if err := json.Unmarshal(mappingJSON, &imp.ColumnMapping); err != nil {
			return nil, err
		}
	}
	return imp, nil
}

func marshalNullable(v any, isNil bool) ([]byte, error) {
	if isNil {
		return nil, nil
	}
	return json.Marshal(v)
}

// Create inserts a new import record. content is the original file, kept so
// the import can be retried; pass nil to not store it.
func (r *ImportRepository) Create(ctx context.Context, imp *domain.Import, content []byte) error {
	errorsJSON, err := marshalNullable(imp.ErrorsJSON, imp.ErrorsJSON == nil)
	if err != nil {
		return err
	}
	mappingJSON, err := marshalNullable(imp.ColumnMapping, len(imp.ColumnMapping) == 0)
	if err != nil {
		return err
	}

	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO imports (id, tenant_id, user_id, file_name, total_rows, rows_ok, rows_error, status,
			errors_json, created_at, reading_type, column_mapping, file_content, duplicates_skipped)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), $12, $13, $14)`,
		imp.ID, imp.TenantID, imp.UserID, imp.FileName, imp.TotalRows, imp.RowsOK, imp.RowsError, imp.Status,
		errorsJSON, imp.CreatedAt, imp.ReadingType, mappingJSON, content, imp.DuplicatesSkipped,
	)
	return err
}

// GetByID retrieves an import record by ID
func (r *ImportRepository) GetByID(ctx context.Context, id domain.UUID) (*domain.Import, error) {
	q, args := scoped(ctx, `SELECT `+importColumns+` FROM imports WHERE id = $1 AND deleted_at IS NULL /*tenant*/`, id)
	return scanImport(r.db.Pool.QueryRow(ctx, q, args...))
}

// GetFileContent returns the stored file of an import, if any.
func (r *ImportRepository) GetFileContent(ctx context.Context, id domain.UUID) ([]byte, error) {
	q, args := scoped(ctx, `SELECT file_content FROM imports WHERE id = $1 AND deleted_at IS NULL /*tenant*/`, id)
	var content []byte
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(&content)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return content, err
}

// ImportFilter filters the import history.
type ImportFilter struct {
	Status      string
	ReadingType string
	Limit       int
	Offset      int
}

// GetByTenant retrieves the imports of a tenant, newest first.
func (r *ImportRepository) GetByTenant(ctx context.Context, tenantID domain.UUID) ([]*domain.Import, error) {
	items, _, err := r.List(ctx, tenantID, ImportFilter{Limit: 200})
	return items, err
}

// List retrieves a filtered page of imports and the total.
func (r *ImportRepository) List(ctx context.Context, tenantID domain.UUID, f ImportFilter) ([]*domain.Import, int, error) {
	where := `tenant_id = $1 AND deleted_at IS NULL AND ($2 = '' OR status = $2) AND ($3 = '' OR reading_type = $3)`
	args := []any{tenantID, f.Status, f.ReadingType}

	var total int
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM imports WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.db.Pool.Query(ctx,
		`SELECT `+importColumns+` FROM imports WHERE `+where+` ORDER BY created_at DESC LIMIT $4 OFFSET $5`,
		append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*domain.Import
	for rows.Next() {
		imp, err := scanImport(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, imp)
	}
	return out, total, rows.Err()
}

// Update updates the result fields of an import record
func (r *ImportRepository) Update(ctx context.Context, imp *domain.Import) error {
	errorsJSON, err := marshalNullable(imp.ErrorsJSON, imp.ErrorsJSON == nil)
	if err != nil {
		return err
	}
	q, args := scoped(ctx, `
		UPDATE imports
		SET total_rows = $1, rows_ok = $2, rows_error = $3, status = $4, errors_json = $5,
			completed_at = $6, duplicates_skipped = $7, updated_at = $8
		WHERE id = $9 /*tenant*/`,
		imp.TotalRows, imp.RowsOK, imp.RowsError, imp.Status, errorsJSON,
		imp.CompletedAt, imp.DuplicatesSkipped, time.Now(), imp.ID,
	)
	_, err = r.db.Pool.Exec(ctx, q, args...)
	return err
}

// SoftDelete marks an import as deleted and drops its stored file.
func (r *ImportRepository) SoftDelete(ctx context.Context, tx pgx.Tx, id domain.UUID) error {
	q, args := scoped(ctx, `UPDATE imports SET deleted_at = NOW(), updated_at = NOW(), file_content = NULL
		WHERE id = $1 AND deleted_at IS NULL /*tenant*/`, id)
	tag, err := tx.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ImportSummary aggregates the import history of a tenant.
type ImportSummary struct {
	TotalImports    int        `json:"total_imports"`
	Completed       int        `json:"completed"`
	CompletedErrors int        `json:"completed_with_errors"`
	Failed          int        `json:"failed"`
	Processing      int        `json:"processing"`
	TotalRows       int        `json:"total_rows"`
	RowsOK          int        `json:"rows_ok"`
	RowsError       int        `json:"rows_error"`
	DuplicatesSkip  int        `json:"duplicates_skipped"`
	SuccessRatePct  float64    `json:"success_rate_pct"`
	LastImportAt    *time.Time `json:"last_import_at"`
	Last30Days      int        `json:"imports_last_30_days"`
}

// Summary returns statistics of the imports of a tenant.
func (r *ImportRepository) Summary(ctx context.Context, tenantID domain.UUID) (*ImportSummary, error) {
	s := &ImportSummary{}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*),
			COUNT(*) FILTER (WHERE status = 'COMPLETED' AND COALESCE(rows_error, 0) = 0),
			COUNT(*) FILTER (WHERE status = 'COMPLETED' AND COALESCE(rows_error, 0) > 0),
			COUNT(*) FILTER (WHERE status = 'ERROR'),
			COUNT(*) FILTER (WHERE status = 'PROCESSING'),
			COALESCE(SUM(total_rows), 0), COALESCE(SUM(rows_ok), 0), COALESCE(SUM(rows_error), 0),
			COALESCE(SUM(duplicates_skipped), 0),
			MAX(created_at),
			COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '30 days')
		FROM imports WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID,
	).Scan(&s.TotalImports, &s.Completed, &s.CompletedErrors, &s.Failed, &s.Processing,
		&s.TotalRows, &s.RowsOK, &s.RowsError, &s.DuplicatesSkip, &s.LastImportAt, &s.Last30Days)
	if err != nil {
		return nil, err
	}
	if s.TotalRows > 0 {
		s.SuccessRatePct = float64(s.RowsOK) / float64(s.TotalRows) * 100
	}
	return s, nil
}
