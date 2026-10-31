package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
)

// BalanceRepository handles transformer balance data access
type BalanceRepository struct {
	db *Database
}

// NewBalanceRepository creates a new BalanceRepository
func NewBalanceRepository(db *Database) *BalanceRepository {
	return &BalanceRepository{db: db}
}

// Create inserts a new transformer balance into the database
func (r *BalanceRepository) Create(ctx context.Context, balance *domain.TransformerBalance) error {
	defer invalidateFromContext(ctx)
	query := `
		INSERT INTO transformer_balance (
			id, tenant_id, transformer_id, period_start, period_end,
			energy_injected_kwh, total_consumption_kwh, loss_kwh, loss_pct,
			technical_loss_kwh, non_technical_loss_kwh, status, uc_count, calculated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		balance.ID,
		balance.TenantID,
		balance.TransformerID,
		balance.PeriodStart,
		balance.PeriodEnd,
		balance.EnergyInjectedKWh,
		balance.TotalConsumptionKWh,
		balance.LossKWh,
		balance.LossPct,
		balance.TechnicalLossKWh,
		balance.NonTechnicalLossKWh,
		balance.Status,
		balance.UCCount,
		balance.CalculatedAt,
	)

	return err
}

// GetByTransformerAndPeriod retrieves balances for a transformer in a period
func (r *BalanceRepository) GetByTransformerAndPeriod(
	ctx context.Context,
	transformerID domain.UUID,
	start, end time.Time,
) ([]*domain.TransformerBalance, error) {
	query := `
		SELECT id, tenant_id, transformer_id, period_start, period_end,
			energy_injected_kwh, total_consumption_kwh, loss_kwh, loss_pct,
			technical_loss_kwh, non_technical_loss_kwh, status, uc_count, calculated_at
		FROM transformer_balance
		WHERE transformer_id = $1 AND period_start >= $2 AND period_start <= $3 /*tenant*/
		ORDER BY period_start DESC
	`

	q, args := scoped(ctx, query, transformerID, start, end)
	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var balances []*domain.TransformerBalance
	for rows.Next() {
		balance := &domain.TransformerBalance{}

		err := rows.Scan(
			&balance.ID,
			&balance.TenantID,
			&balance.TransformerID,
			&balance.PeriodStart,
			&balance.PeriodEnd,
			&balance.EnergyInjectedKWh,
			&balance.TotalConsumptionKWh,
			&balance.LossKWh,
			&balance.LossPct,
			&balance.TechnicalLossKWh,
			&balance.NonTechnicalLossKWh,
			&balance.Status,
			&balance.UCCount,
			&balance.CalculatedAt,
		)
		if err != nil {
			return nil, err
		}

		balances = append(balances, balance)
	}

	return balances, nil
}

// ListByTenantSince returns every balance of the tenant with period_start in
// [start, end], grouped by transformer (mais recente primeiro, como
// GetByTransformerAndPeriod). Uma consulta para a rede toda evita N+1.
func (r *BalanceRepository) ListByTenantSince(ctx context.Context, tenantID domain.UUID, start, end time.Time) (map[domain.UUID][]*domain.TransformerBalance, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT b.id, b.tenant_id, b.transformer_id, b.period_start, b.period_end,
			b.energy_injected_kwh, b.total_consumption_kwh, b.loss_kwh, b.loss_pct,
			b.technical_loss_kwh, b.non_technical_loss_kwh, b.status, b.uc_count, b.calculated_at
		FROM transformer_balance b
		JOIN transformers t ON t.id = b.transformer_id AND t.deleted_at IS NULL
		WHERE b.tenant_id = $1 AND b.period_start >= $2 AND b.period_start <= $3
		ORDER BY b.transformer_id, b.period_start DESC, b.calculated_at DESC`, tenantID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[domain.UUID][]*domain.TransformerBalance)
	for rows.Next() {
		b := &domain.TransformerBalance{}
		if err := rows.Scan(&b.ID, &b.TenantID, &b.TransformerID, &b.PeriodStart, &b.PeriodEnd,
			&b.EnergyInjectedKWh, &b.TotalConsumptionKWh, &b.LossKWh, &b.LossPct,
			&b.TechnicalLossKWh, &b.NonTechnicalLossKWh, &b.Status, &b.UCCount, &b.CalculatedAt); err != nil {
			return nil, err
		}
		out[b.TransformerID] = append(out[b.TransformerID], b)
	}
	return out, rows.Err()
}

// LatestByTenant returns the latest balance of each transformer of the tenant.
func (r *BalanceRepository) LatestByTenant(ctx context.Context, tenantID domain.UUID) (map[domain.UUID]*domain.TransformerBalance, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT DISTINCT ON (b.transformer_id) b.id, b.tenant_id, b.transformer_id, b.period_start, b.period_end,
			b.energy_injected_kwh, b.total_consumption_kwh, b.loss_kwh, b.loss_pct,
			b.technical_loss_kwh, b.non_technical_loss_kwh, b.status, b.uc_count, b.calculated_at
		FROM transformer_balance b
		WHERE b.tenant_id = $1
		ORDER BY b.transformer_id, b.period_start DESC, b.calculated_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[domain.UUID]*domain.TransformerBalance)
	for rows.Next() {
		b := &domain.TransformerBalance{}
		if err := rows.Scan(&b.ID, &b.TenantID, &b.TransformerID, &b.PeriodStart, &b.PeriodEnd,
			&b.EnergyInjectedKWh, &b.TotalConsumptionKWh, &b.LossKWh, &b.LossPct,
			&b.TechnicalLossKWh, &b.NonTechnicalLossKWh, &b.Status, &b.UCCount, &b.CalculatedAt); err != nil {
			return nil, err
		}
		out[b.TransformerID] = b
	}
	return out, rows.Err()
}

// GetLatestByTransformer retrieves the latest balance for a transformer
func (r *BalanceRepository) GetLatestByTransformer(
	ctx context.Context,
	transformerID domain.UUID,
) (*domain.TransformerBalance, error) {
	query := `
		SELECT id, tenant_id, transformer_id, period_start, period_end,
			energy_injected_kwh, total_consumption_kwh, loss_kwh, loss_pct,
			technical_loss_kwh, non_technical_loss_kwh, status, uc_count, calculated_at
		FROM transformer_balance
		WHERE transformer_id = $1 /*tenant*/
		ORDER BY period_start DESC
		LIMIT 1
	`

	balance := &domain.TransformerBalance{}

	q, args := scoped(ctx, query, transformerID)
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(
		&balance.ID,
		&balance.TenantID,
		&balance.TransformerID,
		&balance.PeriodStart,
		&balance.PeriodEnd,
		&balance.EnergyInjectedKWh,
		&balance.TotalConsumptionKWh,
		&balance.LossKWh,
		&balance.LossPct,
		&balance.TechnicalLossKWh,
		&balance.NonTechnicalLossKWh,
		&balance.Status,
		&balance.UCCount,
		&balance.CalculatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return balance, nil
}

// GetByTenant retrieves balances for a tenant
func (r *BalanceRepository) GetByTenant(
	ctx context.Context,
	tenantID domain.UUID,
	limit int,
) ([]*domain.TransformerBalance, error) {
	query := `
		SELECT id, tenant_id, transformer_id, period_start, period_end,
			energy_injected_kwh, total_consumption_kwh, loss_kwh, loss_pct,
			technical_loss_kwh, non_technical_loss_kwh, status, uc_count, calculated_at
		FROM transformer_balance
		WHERE tenant_id = $1
		ORDER BY period_start DESC
		LIMIT $2
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var balances []*domain.TransformerBalance
	for rows.Next() {
		balance := &domain.TransformerBalance{}

		err := rows.Scan(
			&balance.ID,
			&balance.TenantID,
			&balance.TransformerID,
			&balance.PeriodStart,
			&balance.PeriodEnd,
			&balance.EnergyInjectedKWh,
			&balance.TotalConsumptionKWh,
			&balance.LossKWh,
			&balance.LossPct,
			&balance.TechnicalLossKWh,
			&balance.NonTechnicalLossKWh,
			&balance.Status,
			&balance.UCCount,
			&balance.CalculatedAt,
		)
		if err != nil {
			return nil, err
		}

		balances = append(balances, balance)
	}

	return balances, nil
}

// MonthlyAggregate represents monthly aggregated balance data
type MonthlyAggregate struct {
	Month                 time.Time
	TotalEnergyInjected   float64
	TotalConsumption      float64
	TotalLoss             float64
	TotalTechnicalLoss    float64
	TotalNonTechnicalLoss float64
	AvgLossPct            float64
	BalanceCount          int
}

// GetMonthlyAggregatesByTenant returns monthly aggregates for the last N months
func (r *BalanceRepository) GetMonthlyAggregatesByTenant(
	ctx context.Context,
	tenantID domain.UUID,
	months int,
) ([]MonthlyAggregate, error) {
	query := `
		SELECT
			date_trunc('month', period_start) as month,
			COALESCE(SUM(energy_injected_kwh), 0) as total_energy_injected,
			COALESCE(SUM(total_consumption_kwh), 0) as total_consumption,
			COALESCE(SUM(loss_kwh), 0) as total_loss,
			COALESCE(SUM(technical_loss_kwh), 0) as total_technical_loss,
			COALESCE(SUM(non_technical_loss_kwh), 0) as total_non_technical_loss,
			COALESCE(AVG(loss_pct), 0) as avg_loss_pct,
			COUNT(*) as balance_count
		FROM transformer_balance
		WHERE tenant_id = $1
			AND period_start >= date_trunc('month', NOW()) - make_interval(months => $2::int)
		GROUP BY date_trunc('month', period_start)
		ORDER BY month
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID, months)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aggregates []MonthlyAggregate
	for rows.Next() {
		var agg MonthlyAggregate
		err := rows.Scan(
			&agg.Month,
			&agg.TotalEnergyInjected,
			&agg.TotalConsumption,
			&agg.TotalLoss,
			&agg.TotalTechnicalLoss,
			&agg.TotalNonTechnicalLoss,
			&agg.AvgLossPct,
			&agg.BalanceCount,
		)
		if err != nil {
			return nil, err
		}
		aggregates = append(aggregates, agg)
	}

	return aggregates, nil
}

// LossComposition represents the technical vs non-technical loss breakdown
type LossComposition struct {
	TotalTechnicalLoss    float64
	TotalNonTechnicalLoss float64
}

// GetLossCompositionByTenant returns loss composition for a tenant
func (r *BalanceRepository) GetLossCompositionByTenant(
	ctx context.Context,
	tenantID domain.UUID,
) (LossComposition, error) {
	query := `
		SELECT
			COALESCE(SUM(technical_loss_kwh), 0) as total_technical_loss,
			COALESCE(SUM(non_technical_loss_kwh), 0) as total_non_technical_loss
		FROM transformer_balance
		WHERE tenant_id = $1
			AND period_start >= date_trunc('month', NOW()) - interval '12 months'
	`

	var comp LossComposition
	err := r.db.Pool.QueryRow(ctx, query, tenantID).Scan(
		&comp.TotalTechnicalLoss,
		&comp.TotalNonTechnicalLoss,
	)

	return comp, err
}

// TransformerStatusCounts represents the count of transformers by status
type TransformerStatusCounts struct {
	Normal   int
	Warning  int
	Critical int
}

// GetTransformerStatusCounts returns counts of transformers by status
func (r *BalanceRepository) GetTransformerStatusCounts(
	ctx context.Context,
	tenantID domain.UUID,
) (TransformerStatusCounts, error) {
	query := `
		SELECT
		COUNT(*) FILTER (WHERE b.status = 'NORMAL') as normal,
		COUNT(*) FILTER (WHERE b.status = 'WARNING') as warning,
		COUNT(*) FILTER (WHERE b.status = 'CRITICAL') as critical
		FROM (
			SELECT DISTINCT ON (transformer_id) transformer_id, status
			FROM transformer_balance
			WHERE tenant_id = $1
				AND transformer_id IN (SELECT id FROM transformers WHERE tenant_id = $1 AND deleted_at IS NULL)
			ORDER BY transformer_id, period_start DESC
		) b
	`

	var counts TransformerStatusCounts
	err := r.db.Pool.QueryRow(ctx, query, tenantID).Scan(
		&counts.Normal,
		&counts.Warning,
		&counts.Critical,
	)

	return counts, err
}

// QuarterlyAggregate represents quarterly aggregated data
type QuarterlyAggregate struct {
	Quarter             time.Time
	TotalEnergyInjected float64
	TotalConsumption    float64
}

// GetQuarterlyAggregatesByTenant returns quarterly aggregates for the last N quarters
func (r *BalanceRepository) GetQuarterlyAggregatesByTenant(
	ctx context.Context,
	tenantID domain.UUID,
	quarters int,
) ([]QuarterlyAggregate, error) {
	query := `
		SELECT
			date_trunc('quarter', period_start) as quarter,
			COALESCE(SUM(energy_injected_kwh), 0) as total_energy_injected,
			COALESCE(SUM(total_consumption_kwh), 0) as total_consumption
		FROM transformer_balance
		WHERE tenant_id = $1
			AND period_start >= date_trunc('quarter', NOW()) - make_interval(months => $2::int * 3)
		GROUP BY date_trunc('quarter', period_start)
		ORDER BY quarter
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID, quarters)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aggregates []QuarterlyAggregate
	for rows.Next() {
		var agg QuarterlyAggregate
		err := rows.Scan(
			&agg.Quarter,
			&agg.TotalEnergyInjected,
			&agg.TotalConsumption,
		)
		if err != nil {
			return nil, err
		}
		aggregates = append(aggregates, agg)
	}

	return aggregates, nil
}

// AccumulatedCostPoint represents a point in the accumulated loss cost chart
type AccumulatedCostPoint struct {
	Month              time.Time
	MonthlyLossKWh     float64
	AccumulatedLossKWh float64
	MonthlyCostBRL     float64
	AccumulatedCostBRL float64
}

// GetAccumulatedLossCost returns accumulated loss cost over time
func (r *BalanceRepository) GetAccumulatedLossCost(
	ctx context.Context,
	tenantID domain.UUID,
	tariffPerMWh float64,
	months int,
) ([]AccumulatedCostPoint, error) {
	query := `
		SELECT
			date_trunc('month', period_start) as month,
			COALESCE(SUM(loss_kwh), 0) as monthly_loss_kwh,
			COALESCE(SUM(SUM(loss_kwh)) OVER (ORDER BY date_trunc('month', period_start)), 0) as accumulated_loss_kwh
		FROM transformer_balance
		WHERE tenant_id = $1
			AND period_start >= date_trunc('month', NOW()) - make_interval(months => $2::int)
		GROUP BY date_trunc('month', period_start)
		ORDER BY month
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID, months)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []AccumulatedCostPoint
	for rows.Next() {
		var p AccumulatedCostPoint
		err := rows.Scan(
			&p.Month,
			&p.MonthlyLossKWh,
			&p.AccumulatedLossKWh,
		)
		if err != nil {
			return nil, err
		}
		p.MonthlyCostBRL = (p.MonthlyLossKWh / 1000.0) * tariffPerMWh
		p.AccumulatedCostBRL = (p.AccumulatedLossKWh / 1000.0) * tariffPerMWh
		points = append(points, p)
	}

	return points, nil
}

// TopTransformerLoss represents a transformer with its loss metrics
type TopTransformerLoss struct {
	TransformerID       domain.UUID
	Code                string
	LossPct             float64
	LossKWh             float64
	EnergyInjectedKWh   float64
	TechnicalLossKWh    float64
	NonTechnicalLossKWh float64
}

// GetTopTransformersByLoss returns top N transformers by loss percentage
func (r *BalanceRepository) GetTopTransformersByLoss(
	ctx context.Context,
	tenantID domain.UUID,
	limit int,
) ([]TopTransformerLoss, error) {
	query := `
		SELECT
			t.id as transformer_id,
			t.code,
			b.loss_pct,
			b.loss_kwh,
			b.energy_injected_kwh,
			b.technical_loss_kwh,
			b.non_technical_loss_kwh
		FROM (
			SELECT DISTINCT ON (transformer_id)
				transformer_id,
				loss_pct,
				loss_kwh,
				energy_injected_kwh,
				COALESCE(technical_loss_kwh, 0) AS technical_loss_kwh,
				COALESCE(non_technical_loss_kwh, 0) AS non_technical_loss_kwh
			FROM transformer_balance
			WHERE tenant_id = $1
			ORDER BY transformer_id, period_start DESC
		) b
		JOIN transformers t ON t.id = b.transformer_id AND t.deleted_at IS NULL
		ORDER BY b.loss_pct DESC
		LIMIT $2
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transformers []TopTransformerLoss
	for rows.Next() {
		var t TopTransformerLoss
		err := rows.Scan(
			&t.TransformerID,
			&t.Code,
			&t.LossPct,
			&t.LossKWh,
			&t.EnergyInjectedKWh,
			&t.TechnicalLossKWh,
			&t.NonTechnicalLossKWh,
		)
		if err != nil {
			return nil, err
		}
		transformers = append(transformers, t)
	}

	return transformers, nil
}

// UpsertForPeriod stores a balance, replacing the result previously
// calculated for the same transformer and period so recalculations do not
// duplicate history.
func (r *BalanceRepository) UpsertForPeriod(ctx context.Context, balance *domain.TransformerBalance) error {
	defer invalidateFromContext(ctx)
	update := `
		UPDATE transformer_balance
		SET energy_injected_kwh = $4, total_consumption_kwh = $5, loss_kwh = $6, loss_pct = $7,
			technical_loss_kwh = $8, non_technical_loss_kwh = $9, status = $10, uc_count = $11,
			calculated_at = $12
		WHERE transformer_id = $1 AND period_start = $2 AND period_end = $3 /*tenant*/
		RETURNING id
	`
	q, args := scoped(ctx, update,
		balance.TransformerID, balance.PeriodStart, balance.PeriodEnd,
		balance.EnergyInjectedKWh, balance.TotalConsumptionKWh, balance.LossKWh, balance.LossPct,
		balance.TechnicalLossKWh, balance.NonTechnicalLossKWh, balance.Status, balance.UCCount,
		balance.CalculatedAt,
	)

	var existingID domain.UUID
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(&existingID)
	if err == nil {
		balance.ID = existingID
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}
	return r.Create(ctx, balance)
}

// BalanceRow is a balance joined with its transformer and substation, used
// by reports.
type BalanceRow struct {
	domain.TransformerBalance
	TransformerCode string   `json:"transformer_code"`
	PowerKVA        float64  `json:"power_kva"`
	SubstationCode  *string  `json:"substation_code"`
	SubstationName  *string  `json:"substation_name"`
	Lat             *float64 `json:"lat"`
	Lng             *float64 `json:"lng"`
}

// ListRowsByTenantPeriod returns the balances of a tenant whose period
// starts in [start, end], with transformer data, ordered by month and code.
func (r *BalanceRepository) ListRowsByTenantPeriod(ctx context.Context, tenantID domain.UUID, start, end time.Time) ([]*BalanceRow, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT b.id, b.tenant_id, b.transformer_id, b.period_start, b.period_end,
			b.energy_injected_kwh::float8, b.total_consumption_kwh::float8, b.loss_kwh::float8, b.loss_pct::float8,
			b.technical_loss_kwh::float8, b.non_technical_loss_kwh::float8, b.status, b.uc_count, b.calculated_at,
			COALESCE(t.loss_limit_pct, 10)::float8,
			t.code, t.power_kva::float8, s.code, s.name, t.lat::float8, t.lng::float8
		FROM transformer_balance b
		JOIN transformers t ON t.id = b.transformer_id
		LEFT JOIN substations s ON s.id = t.substation_id
		WHERE b.tenant_id = $1 AND b.period_start >= $2 AND b.period_start <= $3 AND t.deleted_at IS NULL
		ORDER BY b.period_start, t.code`, tenantID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*BalanceRow
	for rows.Next() {
		row := &BalanceRow{}
		b := &row.TransformerBalance
		var limit float64
		if err := rows.Scan(&b.ID, &b.TenantID, &b.TransformerID, &b.PeriodStart, &b.PeriodEnd,
			&b.EnergyInjectedKWh, &b.TotalConsumptionKWh, &b.LossKWh, &b.LossPct,
			&b.TechnicalLossKWh, &b.NonTechnicalLossKWh, &b.Status, &b.UCCount, &b.CalculatedAt,
			&limit, &row.TransformerCode, &row.PowerKVA, &row.SubstationCode, &row.SubstationName, &row.Lat, &row.Lng); err != nil {
			return nil, err
		}
		b.LossLimitPct = &limit
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetByID retrieves a balance by ID.
func (r *BalanceRepository) GetByID(ctx context.Context, id domain.UUID) (*domain.TransformerBalance, error) {
	b := &domain.TransformerBalance{}
	q, args := scoped(ctx, `
		SELECT id, tenant_id, transformer_id, period_start, period_end,
			energy_injected_kwh, total_consumption_kwh, loss_kwh, loss_pct,
			technical_loss_kwh, non_technical_loss_kwh, status, uc_count, calculated_at
		FROM transformer_balance WHERE id = $1 /*tenant*/ LIMIT 1`, id)
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(&b.ID, &b.TenantID, &b.TransformerID, &b.PeriodStart, &b.PeriodEnd,
		&b.EnergyInjectedKWh, &b.TotalConsumptionKWh, &b.LossKWh, &b.LossPct,
		&b.TechnicalLossKWh, &b.NonTechnicalLossKWh, &b.Status, &b.UCCount, &b.CalculatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}
