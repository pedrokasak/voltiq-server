package repository

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/energybalance/server/internal/domain"
)

// ImportRepository handles import history data access
type ImportRepository struct {
	db *Database
}

// NewImportRepository creates a new ImportRepository
func NewImportRepository(db *Database) *ImportRepository {
	return &ImportRepository{db: db}
}

// Create inserts a new import record
func (r *ImportRepository) Create(ctx context.Context, importRecord *domain.Import) error {
	query := `
		INSERT INTO imports (id, tenant_id, user_id, file_name, total_rows, rows_ok, rows_error, status, errors_json, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	var errorsJSON []byte
	if importRecord.ErrorsJSON != nil {
		var err error
		errorsJSON, err = json.Marshal(importRecord.ErrorsJSON)
		if err != nil {
			return err
		}
	}

	_, err := r.db.Pool.Exec(ctx, query,
		importRecord.ID,
		importRecord.TenantID,
		importRecord.UserID,
		importRecord.FileName,
		importRecord.TotalRows,
		importRecord.RowsOK,
		importRecord.RowsError,
		importRecord.Status,
		errorsJSON,
		importRecord.CreatedAt,
	)

	return err
}

// GetByID retrieves an import record by ID
func (r *ImportRepository) GetByID(ctx context.Context, id domain.UUID) (*domain.Import, error) {
	query := `
		SELECT id, tenant_id, user_id, file_name, total_rows, rows_ok, rows_error, status, errors_json, created_at, completed_at
		FROM imports
		WHERE id = $1
	`

	importRecord := &domain.Import{}
	var totalRows, rowsOK, rowsError pgtype.Int4
	var errorsJSON []byte
	var completedAt pgtype.Timestamptz

	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&importRecord.ID,
		&importRecord.TenantID,
		&importRecord.UserID,
		&importRecord.FileName,
		&totalRows,
		&rowsOK,
		&rowsError,
		&importRecord.Status,
		&errorsJSON,
		&importRecord.CreatedAt,
		&completedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if totalRows.Valid {
		val := int(totalRows.Int32)
		importRecord.TotalRows = &val
	}
	if rowsOK.Valid {
		val := int(rowsOK.Int32)
		importRecord.RowsOK = &val
	}
	if rowsError.Valid {
		val := int(rowsError.Int32)
		importRecord.RowsError = &val
	}

	if len(errorsJSON) > 0 {
		if err := json.Unmarshal(errorsJSON, &importRecord.ErrorsJSON); err != nil {
			return nil, err
		}
	}

	if completedAt.Valid {
		importRecord.CompletedAt = &completedAt.Time
	}

	return importRecord, nil
}

// GetByTenant retrieves all imports for a tenant
func (r *ImportRepository) GetByTenant(ctx context.Context, tenantID domain.UUID) ([]*domain.Import, error) {
	query := `
		SELECT id, tenant_id, user_id, file_name, total_rows, rows_ok, rows_error, status, errors_json, created_at, completed_at
		FROM imports
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var imports []*domain.Import
	for rows.Next() {
		importRecord := &domain.Import{}
		var totalRows, rowsOK, rowsError pgtype.Int4
		var errorsJSON []byte
		var completedAt pgtype.Timestamptz

		err := rows.Scan(
			&importRecord.ID,
			&importRecord.TenantID,
			&importRecord.UserID,
			&importRecord.FileName,
			&totalRows,
			&rowsOK,
			&rowsError,
			&importRecord.Status,
			&errorsJSON,
			&importRecord.CreatedAt,
			&completedAt,
		)
		if err != nil {
			return nil, err
		}

		if totalRows.Valid {
			val := int(totalRows.Int32)
			importRecord.TotalRows = &val
		}
		if rowsOK.Valid {
			val := int(rowsOK.Int32)
			importRecord.RowsOK = &val
		}
		if rowsError.Valid {
			val := int(rowsError.Int32)
			importRecord.RowsError = &val
		}

		if len(errorsJSON) > 0 {
			if err := json.Unmarshal(errorsJSON, &importRecord.ErrorsJSON); err != nil {
				return nil, err
			}
		}

		if completedAt.Valid {
			importRecord.CompletedAt = &completedAt.Time
		}

		imports = append(imports, importRecord)
	}

	return imports, nil
}

// Update updates an import record
func (r *ImportRepository) Update(ctx context.Context, importRecord *domain.Import) error {
	query := `
		UPDATE imports
		SET total_rows = $1, rows_ok = $2, rows_error = $3, status = $4, errors_json = $5, completed_at = $6
		WHERE id = $7
	`

	var errorsJSON []byte
	if importRecord.ErrorsJSON != nil {
		var err error
		errorsJSON, err = json.Marshal(importRecord.ErrorsJSON)
		if err != nil {
			return err
		}
	}

	_, err := r.db.Pool.Exec(ctx, query,
		importRecord.TotalRows,
		importRecord.RowsOK,
		importRecord.RowsError,
		importRecord.Status,
		errorsJSON,
		importRecord.CompletedAt,
		importRecord.ID,
	)

	return err
}
