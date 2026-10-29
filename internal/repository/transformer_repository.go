package repository

import (
	"context"
	"fmt"
	"strings"
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
			loss_limit_pct, active, created_at, COALESCE(updated_at, created_at), deleted_at
		FROM transformers
		WHERE id = $1 AND deleted_at IS NULL /*tenant*/
	`

	transformer := &domain.Transformer{}

	q, args := scoped(ctx, query, id)
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(
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
			loss_limit_pct, active, created_at, COALESCE(updated_at, created_at), deleted_at
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
		WHERE id = $13 /*tenant*/
	`

	q, args := scoped(ctx, query,
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
	_, err := r.db.Pool.Exec(ctx, q, args...)

	return err
}

// Delete soft deletes a transformer
func (r *TransformerRepository) Delete(ctx context.Context, id domain.UUID) error {
	query := `
		UPDATE transformers
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

const transformerColumns = `id, tenant_id, substation_id, code, power_kva, primary_voltage_kv,
	secondary_voltage_v, lat, lng, core_loss_kw, winding_loss_kw,
	loss_limit_pct, active, created_at, COALESCE(updated_at, created_at), deleted_at`

func scanTransformer(row pgx.Row) (*domain.Transformer, error) {
	t := &domain.Transformer{}
	err := row.Scan(
		&t.ID, &t.TenantID, &t.SubstationID, &t.Code, &t.PowerKVA, &t.PrimaryVoltageKV,
		&t.SecondaryVoltageV, &t.Lat, &t.Lng, &t.CoreLossKW, &t.WindingLossKW,
		&t.LossLimitPct, &t.Active, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetByCode retrieves a transformer of a tenant by its code.
func (r *TransformerRepository) GetByCode(ctx context.Context, tenantID domain.UUID, code string) (*domain.Transformer, error) {
	query := `SELECT ` + transformerColumns + `
		FROM transformers
		WHERE tenant_id = $1 AND code = $2 AND deleted_at IS NULL`
	return scanTransformer(r.db.Pool.QueryRow(ctx, query, tenantID, code))
}

// CountByTenant returns the number of non-deleted transformers of a tenant.
func (r *TransformerRepository) CountByTenant(ctx context.Context, tenantID domain.UUID) (int, error) {
	var n int
	err := r.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM transformers WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&n)
	return n, err
}

// ListBySubstation retrieves the transformers fed by a substation.
func (r *TransformerRepository) ListBySubstation(ctx context.Context, tenantID, substationID domain.UUID) ([]*domain.Transformer, error) {
	query := `SELECT ` + transformerColumns + `
		FROM transformers
		WHERE tenant_id = $1 AND substation_id = $2 AND deleted_at IS NULL
		ORDER BY code`
	rows, err := r.db.Pool.Query(ctx, query, tenantID, substationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Transformer
	for rows.Next() {
		t, err := scanTransformer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TransformerFilter filters and paginates transformer listings.
type TransformerFilter struct {
	Search       string
	Status       string // NORMAL, WARNING, CRITICAL, NO_DATA
	SubstationID string
	GroupID      string
	Active       *bool
	Limit        int // 0 = no limit
	Offset       int
	OrderBy      string // code, loss_pct, power_kva, updated_at
}

// TransformerSummary is a transformer with its latest balance, UC count and
// substation, used by listings and the map.
type TransformerSummary struct {
	domain.Transformer
	SubstationCode      *string    `json:"substation_code"`
	SubstationName      *string    `json:"substation_name"`
	UCCount             int        `json:"uc_count"`
	Status              string     `json:"status"`
	LossPct             *float64   `json:"loss_pct"`
	LossKWh             *float64   `json:"loss_kwh"`
	EnergyInjectedKWh   *float64   `json:"energy_injected_kwh"`
	TotalConsumptionKWh *float64   `json:"total_consumption_kwh"`
	TechnicalLossKWh    *float64   `json:"technical_loss_kwh"`
	NonTechnicalLossKWh *float64   `json:"non_technical_loss_kwh"`
	LastPeriodStart     *time.Time `json:"last_period_start"`
	LastPeriodEnd       *time.Time `json:"last_period_end"`
	LastCalculatedAt    *time.Time `json:"last_calculated_at"`
	LastReadingAt       *time.Time `json:"last_reading_at"`
	AlertsLast30d       int        `json:"alerts_last_30d"`
}

// AvgLossLimit returns the average regulatory loss limit of the active
// network (10% quando o trafo nao define limite).
func (r *TransformerRepository) AvgLossLimit(ctx context.Context, tenantID domain.UUID) (float64, error) {
	var avg float64
	err := r.db.Pool.QueryRow(ctx, `SELECT COALESCE(AVG(COALESCE(NULLIF(loss_limit_pct, 0), 10)), 10)::float8
		FROM transformers WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&avg)
	return avg, err
}

// ListSummaries lists transformers of a tenant with their latest balance.
// Returns the page and the total matching the filter.
func (r *TransformerRepository) ListSummaries(ctx context.Context, tenantID domain.UUID, f TransformerFilter) ([]*TransformerSummary, int, error) {
	args := []any{tenantID}
	where := []string{"t.tenant_id = $1", "t.deleted_at IS NULL"}

	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		where = append(where, fmt.Sprintf("(t.code ILIKE $%d OR s.name ILIKE $%d OR s.code ILIKE $%d)", len(args), len(args), len(args)))
	}
	if f.SubstationID != "" {
		args = append(args, f.SubstationID)
		where = append(where, fmt.Sprintf("t.substation_id = $%d", len(args)))
	}
	if f.GroupID != "" {
		args = append(args, f.GroupID)
		where = append(where, fmt.Sprintf("t.id IN (SELECT transformer_id FROM transformer_group_members WHERE group_id = $%d)", len(args)))
	}
	if f.Active != nil {
		args = append(args, *f.Active)
		where = append(where, fmt.Sprintf("t.active = $%d", len(args)))
	}
	switch f.Status {
	case "NORMAL", "WARNING", "CRITICAL":
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("b.status = $%d", len(args)))
	case "NO_DATA":
		where = append(where, "b.status IS NULL")
	}

	order := "t.code"
	switch f.OrderBy {
	case "loss_pct":
		order = "b.loss_pct DESC NULLS LAST, t.code"
	case "power_kva":
		order = "t.power_kva DESC, t.code"
	case "updated_at":
		order = "t.updated_at DESC NULLS LAST, t.code"
	}

	// Agregados calculados uma vez por tenant e unidos por hash: com dezenas de
	// milhares de trafos, subconsultas correlacionadas por linha (ultimo
	// balanco, UCs, ultima leitura, alertas) levavam segundos.
	ctes := `
		WITH b AS (
			SELECT DISTINCT ON (transformer_id) transformer_id, status,
				loss_pct::float8 AS loss_pct, loss_kwh::float8 AS loss_kwh,
				energy_injected_kwh::float8 AS energy_injected_kwh,
				total_consumption_kwh::float8 AS total_consumption_kwh,
				technical_loss_kwh::float8 AS technical_loss_kwh,
				non_technical_loss_kwh::float8 AS non_technical_loss_kwh,
				period_start, period_end, calculated_at
			FROM transformer_balance
			WHERE tenant_id = $1
			ORDER BY transformer_id, period_start DESC, calculated_at DESC
		), uc AS (
			SELECT transformer_id, COUNT(*) AS n FROM consuming_units
			WHERE tenant_id = $1 AND deleted_at IS NULL GROUP BY transformer_id
		), rd AS (
			SELECT transformer_id, MAX(reading_at) AS last_reading_at FROM transformer_readings
			WHERE tenant_id = $1 GROUP BY transformer_id
		), al AS (
			SELECT transformer_id, COUNT(*) AS n FROM alerts
			WHERE tenant_id = $1 AND deleted_at IS NULL AND created_at >= NOW() - INTERVAL '30 days'
			GROUP BY transformer_id
		)`
	base := `
		FROM transformers t
		LEFT JOIN substations s ON s.id = t.substation_id
		LEFT JOIN b ON b.transformer_id = t.id
		LEFT JOIN uc ON uc.transformer_id = t.id
		LEFT JOIN rd ON rd.transformer_id = t.id
		LEFT JOIN al ON al.transformer_id = t.id
		WHERE ` + strings.Join(where, " AND ")

	var total int
	if err := r.db.Pool.QueryRow(ctx, ctes+" SELECT COUNT(*) "+base, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := ctes + ` SELECT t.id, t.tenant_id, t.substation_id, t.code, t.power_kva, t.primary_voltage_kv,
			t.secondary_voltage_v, t.lat, t.lng, t.core_loss_kw, t.winding_loss_kw,
			t.loss_limit_pct, t.active, t.created_at, COALESCE(t.updated_at, t.created_at), t.deleted_at,
			s.code, s.name,
			COALESCE(uc.n, 0),
			COALESCE(b.status, 'NO_DATA'), b.loss_pct, b.loss_kwh, b.energy_injected_kwh, b.total_consumption_kwh,
			b.technical_loss_kwh, b.non_technical_loss_kwh, b.period_start, b.period_end, b.calculated_at,
			rd.last_reading_at, COALESCE(al.n, 0)
		` + base + " ORDER BY " + order
	if f.Limit > 0 {
		args = append(args, f.Limit, f.Offset)
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	}

	rows, err := r.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*TransformerSummary
	for rows.Next() {
		s := &TransformerSummary{}
		t := &s.Transformer
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.SubstationID, &t.Code, &t.PowerKVA, &t.PrimaryVoltageKV,
			&t.SecondaryVoltageV, &t.Lat, &t.Lng, &t.CoreLossKW, &t.WindingLossKW,
			&t.LossLimitPct, &t.Active, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
			&s.SubstationCode, &s.SubstationName, &s.UCCount,
			&s.Status, &s.LossPct, &s.LossKWh, &s.EnergyInjectedKWh, &s.TotalConsumptionKWh,
			&s.TechnicalLossKWh, &s.NonTechnicalLossKWh, &s.LastPeriodStart, &s.LastPeriodEnd, &s.LastCalculatedAt,
			&s.LastReadingAt, &s.AlertsLast30d,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}

// UpsertByCode creates the transformer or, when the tenant already has one
// with the same code (including a soft-deleted one), updates and restores it.
// Returns true when a new row was inserted.
func (r *TransformerRepository) UpsertByCode(ctx context.Context, tx pgx.Tx, t *domain.Transformer) (bool, error) {
	query := `
		INSERT INTO transformers (
			id, tenant_id, substation_id, code, power_kva, primary_voltage_kv,
			secondary_voltage_v, lat, lng, core_loss_kw, winding_loss_kw,
			loss_limit_pct, active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)
		ON CONFLICT (tenant_id, code) DO UPDATE SET
			substation_id = COALESCE(EXCLUDED.substation_id, transformers.substation_id),
			power_kva = EXCLUDED.power_kva,
			primary_voltage_kv = EXCLUDED.primary_voltage_kv,
			secondary_voltage_v = EXCLUDED.secondary_voltage_v,
			lat = COALESCE(EXCLUDED.lat, transformers.lat),
			lng = COALESCE(EXCLUDED.lng, transformers.lng),
			core_loss_kw = COALESCE(EXCLUDED.core_loss_kw, transformers.core_loss_kw),
			winding_loss_kw = COALESCE(EXCLUDED.winding_loss_kw, transformers.winding_loss_kw),
			loss_limit_pct = COALESCE(EXCLUDED.loss_limit_pct, transformers.loss_limit_pct),
			active = true,
			deleted_at = NULL,
			updated_at = EXCLUDED.updated_at
		RETURNING id, (xmax = 0) AS inserted
	`
	var inserted bool
	err := tx.QueryRow(ctx, query,
		t.ID, t.TenantID, t.SubstationID, t.Code, t.PowerKVA, t.PrimaryVoltageKV,
		t.SecondaryVoltageV, t.Lat, t.Lng, t.CoreLossKW, t.WindingLossKW,
		t.LossLimitPct, true, time.Now(),
	).Scan(&t.ID, &inserted)
	return inserted, err
}

// TransformerRef identifies a transformer during imports.
type TransformerRef struct {
	ID   domain.UUID
	Code string
}

// RefsByTenant returns every non-deleted transformer of a tenant indexed by
// upper-cased code and by lower-cased ID.
func (r *TransformerRepository) RefsByTenant(ctx context.Context, tenantID domain.UUID) (map[string]TransformerRef, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, code FROM transformers WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]TransformerRef{}
	for rows.Next() {
		var ref TransformerRef
		if err := rows.Scan(&ref.ID, &ref.Code); err != nil {
			return nil, err
		}
		out[strings.ToUpper(ref.Code)] = ref
		out[strings.ToLower(string(ref.ID))] = ref
	}
	return out, rows.Err()
}
