package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/energybalance/server/internal/domain"
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
		WHERE transformer_id = $1 AND reading_at >= $2 AND reading_at <= $3
		ORDER BY reading_at ASC
	`

	rows, err := r.db.Pool.Query(ctx, query, transformerID, start, end)
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
		WHERE transformer_id = $1 AND reading_at >= $2 AND reading_at <= $3
	`

	var total float64
	err := r.db.Pool.QueryRow(ctx, query, transformerID, start, end).Scan(&total)
	if err != nil {
		return 0, err
	}

	return total, nil
}
