package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/energybalance/server/internal/domain"
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
		WHERE transformer_id = $1 AND reading_at >= $2 AND reading_at <= $3
		ORDER BY reading_at ASC
	`

	rows, err := r.db.Pool.Query(ctx, query, transformerID, start, end)
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
		WHERE transformer_id = $1 AND reading_at >= $2 AND reading_at <= $3
	`

	var total float64
	err := r.db.Pool.QueryRow(ctx, query, transformerID, start, end).Scan(&total)
	if err != nil {
		return 0, err
	}

	return total, nil
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
		WHERE uc_id = $1 AND reading_at >= $2 AND reading_at <= $3
		ORDER BY reading_at ASC
	`

	rows, err := r.db.Pool.Query(ctx, query, ucID, start, end)
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
