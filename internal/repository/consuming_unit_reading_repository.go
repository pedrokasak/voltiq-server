package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
)

// UCReadingRepository handles consuming unit reading data access
type UCReadingRepository struct {
	db *Database
}

// NewUCReadingRepository creates a new UCReadingRepository
func NewUCReadingRepository(db *Database) *UCReadingRepository {
	return &UCReadingRepository{db: db}
}

// Create inserts a new UC reading into the database
func (r *UCReadingRepository) Create(ctx context.Context, reading *domain.UCReading) error {
	query := `
		INSERT INTO consuming_unit_readings (id, tenant_id, uc_id, transformer_id, reading_at, consumption_kwh, import_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		reading.ID,
		reading.TenantID,
		reading.UCID,
		reading.TransformerID,
		reading.ReadingAt,
		reading.ConsumptionKWh,
		reading.ImportID,
		reading.CreatedAt,
	)

	return err
}

// CreateBatch inserts multiple UC readings
func (r *UCReadingRepository) CreateBatch(ctx context.Context, readings []*domain.UCReading) error {
	batch := &pgx.Batch{}

	for _, reading := range readings {
		query := `
			INSERT INTO consuming_unit_readings (id, tenant_id, uc_id, transformer_id, reading_at, consumption_kwh, import_id, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`

		batch.Queue(query,
			reading.ID,
			reading.TenantID,
			reading.UCID,
			reading.TransformerID,
			reading.ReadingAt,
			reading.ConsumptionKWh,
			reading.ImportID,
			reading.CreatedAt,
		)
	}

	br := r.db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	var firstErr error
	for i := 0; i < batch.Len(); i++ {
		_, err := br.Exec()
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// GetByTransformerAndPeriod retrieves UC readings for a transformer in a period
func (r *UCReadingRepository) GetByTransformerAndPeriod(
	ctx context.Context,
	transformerID domain.UUID,
	start, end time.Time,
) ([]*domain.UCReading, error) {
	query := `
		SELECT id, tenant_id, uc_id, transformer_id, reading_at, consumption_kwh, import_id, created_at
		FROM consuming_unit_readings
		WHERE transformer_id = $1 AND reading_at >= $2 AND reading_at <= $3 /*tenant*/
		ORDER BY reading_at ASC
	`

	q, args := scoped(ctx, query, transformerID, start, end)
	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var readings []*domain.UCReading
	for rows.Next() {
		reading := &domain.UCReading{}

		err := rows.Scan(
			&reading.ID,
			&reading.TenantID,
			&reading.UCID,
			&reading.TransformerID,
			&reading.ReadingAt,
			&reading.ConsumptionKWh,
			&reading.ImportID,
			&reading.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		readings = append(readings, reading)
	}

	return readings, nil
}

// GetTotalByTransformerAndPeriod retrieves total consumption for a transformer in a period
func (r *UCReadingRepository) GetTotalByTransformerAndPeriod(
	ctx context.Context,
	transformerID domain.UUID,
	start, end time.Time,
) (float64, error) {
	query := `
		SELECT COALESCE(SUM(consumption_kwh), 0)
		FROM consuming_unit_readings
		WHERE transformer_id = $1 AND reading_at >= $2 AND reading_at <= $3 /*tenant*/
	`

	var total float64
	q, args := scoped(ctx, query, transformerID, start, end)
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(&total)
	if err != nil {
		return 0, err
	}

	return total, nil
}

// CountByTransformerAndPeriod counts the UC readings of a transformer in a period.
func (r *UCReadingRepository) CountByTransformerAndPeriod(ctx context.Context, transformerID domain.UUID, start, end time.Time) (int, error) {
	q, args := scoped(ctx, `SELECT COUNT(*) FROM consuming_unit_readings
		WHERE transformer_id = $1 AND reading_at >= $2 AND reading_at <= $3 /*tenant*/`, transformerID, start, end)
	var n int
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(&n)
	return n, err
}

// GetByUCAndPeriod retrieves readings for a UC in a period
func (r *UCReadingRepository) GetByUCAndPeriod(
	ctx context.Context,
	ucID domain.UUID,
	start, end time.Time,
) ([]*domain.UCReading, error) {
	query := `
		SELECT id, tenant_id, uc_id, transformer_id, reading_at, consumption_kwh, import_id, created_at
		FROM consuming_unit_readings
		WHERE uc_id = $1 AND reading_at >= $2 AND reading_at <= $3 /*tenant*/
		ORDER BY reading_at ASC
	`

	q, args := scoped(ctx, query, ucID, start, end)
	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var readings []*domain.UCReading
	for rows.Next() {
		reading := &domain.UCReading{}

		err := rows.Scan(
			&reading.ID,
			&reading.TenantID,
			&reading.UCID,
			&reading.TransformerID,
			&reading.ReadingAt,
			&reading.ConsumptionKWh,
			&reading.ImportID,
			&reading.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		readings = append(readings, reading)
	}

	return readings, nil
}

// ExistingKeys returns which (UC, instant) pairs already have a reading.
func (r *UCReadingRepository) ExistingKeys(ctx context.Context, tenantID domain.UUID, ids []domain.UUID, start, end time.Time) (map[ReadingKey]bool, error) {
	out := map[ReadingKey]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	strIDs := make([]string, len(ids))
	for i, id := range ids {
		strIDs[i] = string(id)
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT uc_id, reading_at FROM consuming_unit_readings
		WHERE tenant_id = $1 AND uc_id = ANY($2::uuid[]) AND reading_at BETWEEN $3 AND $4`,
		tenantID, strIDs, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k ReadingKey
		if err := rows.Scan(&k.MeterID, &k.At); err != nil {
			return nil, err
		}
		out[ReadingKey{MeterID: k.MeterID, At: k.At.UTC()}] = true
	}
	return out, rows.Err()
}

// CopyInsert bulk-inserts UC readings inside tx using COPY.
func (r *UCReadingRepository) CopyInsert(ctx context.Context, tx pgx.Tx, readings []*domain.UCReading) (int64, error) {
	if len(readings) == 0 {
		return 0, nil
	}
	return tx.CopyFrom(ctx,
		pgx.Identifier{"consuming_unit_readings"},
		[]string{"id", "tenant_id", "uc_id", "transformer_id", "reading_at", "consumption_kwh", "import_id", "created_at"},
		pgx.CopyFromSlice(len(readings), func(i int) ([]any, error) {
			rd := readings[i]
			var importID any
			if rd.ImportID != nil {
				importID = string(*rd.ImportID)
			}
			return []any{string(rd.ID), string(rd.TenantID), string(rd.UCID), string(rd.TransformerID),
				rd.ReadingAt, rd.ConsumptionKWh, importID, rd.CreatedAt}, nil
		}),
	)
}

// DeleteByImport removes the UC readings created by an import.
func (r *UCReadingRepository) DeleteByImport(ctx context.Context, tx pgx.Tx, tenantID, importID domain.UUID) (int64, error) {
	tag, err := tx.Exec(ctx, `DELETE FROM consuming_unit_readings WHERE tenant_id = $1 AND import_id = $2`, tenantID, importID)
	return tag.RowsAffected(), err
}

// UCConsumption is the consumption of one UC in a period.
type UCConsumption struct {
	UCID           domain.UUID `json:"uc_id"`
	UCCode         string      `json:"uc_code"`
	Name           string      `json:"name"`
	Class          *string     `json:"class"`
	ConsumptionKWh float64     `json:"consumption_kwh"`
	Readings       int         `json:"readings"`
}

// ConsumptionByUC returns the consumption of each UC of a transformer in a
// period, largest first (top consumidores do trafo).
func (r *UCReadingRepository) ConsumptionByUC(ctx context.Context, transformerID domain.UUID, start, end time.Time, limit int) ([]UCConsumption, error) {
	if limit <= 0 {
		limit = 1000
	}
	q, args := scopedColumn(ctx, "u.tenant_id", `
		SELECT u.id, u.uc_code, COALESCE(u.name, ''), u.class,
			COALESCE(SUM(r.consumption_kwh), 0)::float8, COUNT(r.id)
		FROM consuming_units u
		LEFT JOIN consuming_unit_readings r ON r.uc_id = u.id AND r.reading_at >= $2 AND r.reading_at <= $3
		WHERE u.transformer_id = $1 AND u.deleted_at IS NULL /*tenant*/
		GROUP BY u.id, u.uc_code, u.name, u.class
		ORDER BY 5 DESC, u.uc_code
		LIMIT $4`, transformerID, start, end, limit)
	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UCConsumption
	for rows.Next() {
		var c UCConsumption
		if err := rows.Scan(&c.UCID, &c.UCCode, &c.Name, &c.Class, &c.ConsumptionKWh, &c.Readings); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UCReadingWithCode is a UC reading with the UC code.
type UCReadingWithCode struct {
	domain.UCReading
	UCCode string
}

// ListWithCodes returns the UC readings of a transformer in a period with
// the UC codes, ordered by UC and time.
func (r *UCReadingRepository) ListWithCodes(ctx context.Context, transformerID domain.UUID, start, end time.Time) ([]UCReadingWithCode, error) {
	q, args := scopedColumn(ctx, "r.tenant_id", `
		SELECT r.id, r.tenant_id, r.uc_id, r.transformer_id, r.reading_at, r.consumption_kwh::float8, u.uc_code
		FROM consuming_unit_readings r
		JOIN consuming_units u ON u.id = r.uc_id
		WHERE r.transformer_id = $1 AND r.reading_at >= $2 AND r.reading_at <= $3 /*tenant*/
		ORDER BY u.uc_code, r.reading_at`, transformerID, start, end)
	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UCReadingWithCode
	for rows.Next() {
		var x UCReadingWithCode
		if err := rows.Scan(&x.ID, &x.TenantID, &x.UCID, &x.TransformerID, &x.ReadingAt, &x.ConsumptionKWh, &x.UCCode); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
