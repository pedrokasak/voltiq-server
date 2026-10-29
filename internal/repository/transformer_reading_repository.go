package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
)

// TransformerReadingRepository handles transformer reading data access
type TransformerReadingRepository struct {
	db *Database
}

// NewTransformerReadingRepository creates a new TransformerReadingRepository
func NewTransformerReadingRepository(db *Database) *TransformerReadingRepository {
	return &TransformerReadingRepository{db: db}
}

// Create inserts a new transformer reading into the database
func (r *TransformerReadingRepository) Create(ctx context.Context, reading *domain.TransformerReading) error {
	query := `
		INSERT INTO transformer_readings (id, tenant_id, transformer_id, reading_at, energy_kwh, demand_kw, power_factor, import_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		reading.ID,
		reading.TenantID,
		reading.TransformerID,
		reading.ReadingAt,
		reading.EnergyKWh,
		reading.DemandKW,
		reading.PowerFactor,
		reading.ImportID,
		reading.CreatedAt,
	)

	return err
}

// CreateBatch inserts multiple transformer readings
func (r *TransformerReadingRepository) CreateBatch(ctx context.Context, readings []*domain.TransformerReading) error {
	batch := &pgx.Batch{}

	for _, reading := range readings {
		query := `
			INSERT INTO transformer_readings (id, tenant_id, transformer_id, reading_at, energy_kwh, demand_kw, power_factor, import_id, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`

		batch.Queue(query,
			reading.ID,
			reading.TenantID,
			reading.TransformerID,
			reading.ReadingAt,
			reading.EnergyKWh,
			reading.DemandKW,
			reading.PowerFactor,
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

// GetByTransformerAndPeriod retrieves transformer readings for a period
func (r *TransformerReadingRepository) GetByTransformerAndPeriod(
	ctx context.Context,
	transformerID domain.UUID,
	start, end time.Time,
) ([]*domain.TransformerReading, error) {
	query := `
		SELECT id, tenant_id, transformer_id, reading_at, energy_kwh, demand_kw, power_factor, import_id, created_at
		FROM transformer_readings
		WHERE transformer_id = $1 AND reading_at >= $2 AND reading_at <= $3 /*tenant*/
		ORDER BY reading_at ASC
	`

	q, args := scoped(ctx, query, transformerID, start, end)
	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var readings []*domain.TransformerReading
	for rows.Next() {
		reading := &domain.TransformerReading{}

		err := rows.Scan(
			&reading.ID,
			&reading.TenantID,
			&reading.TransformerID,
			&reading.ReadingAt,
			&reading.EnergyKWh,
			&reading.DemandKW,
			&reading.PowerFactor,
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

// GetTotalByTransformerAndPeriod retrieves total energy for a transformer in a period
func (r *TransformerReadingRepository) GetTotalByTransformerAndPeriod(
	ctx context.Context,
	transformerID domain.UUID,
	start, end time.Time,
) (float64, error) {
	query := `
		SELECT COALESCE(SUM(energy_kwh), 0)
		FROM transformer_readings
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

// ReadingAggregate summarizes transformer readings of a period.
type ReadingAggregate struct {
	Count          int
	EnergyKWh      float64
	AvgDemandKW    *float64
	AvgPowerFactor *float64
	FirstReading   *time.Time
	LastReading    *time.Time
}

// AggregateByTransformerAndPeriod returns energy totals and average demand
// and power factor of a transformer in a period, in a single query.
func (r *TransformerReadingRepository) AggregateByTransformerAndPeriod(
	ctx context.Context,
	transformerID domain.UUID,
	start, end time.Time,
) (ReadingAggregate, error) {
	query := `
		SELECT COUNT(*), COALESCE(SUM(energy_kwh), 0)::float8,
			AVG(demand_kw)::float8, AVG(power_factor)::float8,
			MIN(reading_at), MAX(reading_at)
		FROM transformer_readings
		WHERE transformer_id = $1 AND reading_at >= $2 AND reading_at <= $3 /*tenant*/
	`

	var agg ReadingAggregate
	q, args := scoped(ctx, query, transformerID, start, end)
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(
		&agg.Count, &agg.EnergyKWh, &agg.AvgDemandKW, &agg.AvgPowerFactor,
		&agg.FirstReading, &agg.LastReading,
	)
	return agg, err
}

// ReadingKey identifies a reading of a meter at an instant (for dedupe).
type ReadingKey struct {
	MeterID domain.UUID
	At      time.Time
}

// ExistingKeys returns which (transformer, instant) pairs already have a
// reading, among the given transformers in [start, end].
func (r *TransformerReadingRepository) ExistingKeys(ctx context.Context, tenantID domain.UUID, ids []domain.UUID, start, end time.Time) (map[ReadingKey]bool, error) {
	out := map[ReadingKey]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	strIDs := make([]string, len(ids))
	for i, id := range ids {
		strIDs[i] = string(id)
	}
	rows, err := r.db.Pool.Query(ctx, `
		SELECT transformer_id, reading_at FROM transformer_readings
		WHERE tenant_id = $1 AND transformer_id = ANY($2::uuid[]) AND reading_at BETWEEN $3 AND $4`,
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

// CopyInsert bulk-inserts readings inside tx using COPY (RNF01.1).
func (r *TransformerReadingRepository) CopyInsert(ctx context.Context, tx pgx.Tx, readings []*domain.TransformerReading) (int64, error) {
	if len(readings) == 0 {
		return 0, nil
	}
	return tx.CopyFrom(ctx,
		pgx.Identifier{"transformer_readings"},
		[]string{"id", "tenant_id", "transformer_id", "reading_at", "energy_kwh", "demand_kw", "power_factor", "import_id", "created_at"},
		pgx.CopyFromSlice(len(readings), func(i int) ([]any, error) {
			rd := readings[i]
			var importID any
			if rd.ImportID != nil {
				importID = string(*rd.ImportID)
			}
			return []any{string(rd.ID), string(rd.TenantID), string(rd.TransformerID), rd.ReadingAt,
				rd.EnergyKWh, rd.DemandKW, rd.PowerFactor, importID, rd.CreatedAt}, nil
		}),
	)
}

// DeleteByImport removes the readings created by an import (desfazer
// importacao, solicitado pelo usuario).
func (r *TransformerReadingRepository) DeleteByImport(ctx context.Context, tx pgx.Tx, tenantID, importID domain.UUID) (int64, error) {
	tag, err := tx.Exec(ctx, `DELETE FROM transformer_readings WHERE tenant_id = $1 AND import_id = $2`, tenantID, importID)
	return tag.RowsAffected(), err
}

// SeriesPoint is one bucket of a reading time series.
type SeriesPoint struct {
	Bucket    time.Time `json:"bucket"`
	EnergyKWh float64   `json:"energy_kwh"`
	DemandKW  *float64  `json:"avg_demand_kw,omitempty"`
	Readings  int       `json:"readings"`
}

// Series returns energy per day or month of a transformer in a period.
func (r *TransformerReadingRepository) Series(ctx context.Context, transformerID domain.UUID, start, end time.Time, bucket string) ([]SeriesPoint, error) {
	if bucket != "day" && bucket != "week" {
		bucket = "month"
	}
	q, args := scoped(ctx, `
		SELECT date_trunc('`+bucket+`', reading_at) AS b, SUM(energy_kwh)::float8, AVG(demand_kw)::float8, COUNT(*)
		FROM transformer_readings
		WHERE transformer_id = $1 AND reading_at >= $2 AND reading_at <= $3 /*tenant*/
		GROUP BY b ORDER BY b`, transformerID, start, end)
	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeriesPoint
	for rows.Next() {
		var p SeriesPoint
		if err := rows.Scan(&p.Bucket, &p.EnergyKWh, &p.DemandKW, &p.Readings); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
