package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// MetricsRepository handles billing metrics aggregation
type MetricsRepository struct {
	db *Database
}

// NewMetricsRepository creates a new MetricsRepository
func NewMetricsRepository(db *Database) *MetricsRepository {
	return &MetricsRepository{db: db}
}

// UpsertDailyMetrics inserts or updates daily billing metrics
func (r *MetricsRepository) UpsertDailyMetrics(ctx context.Context, metrics *DailyBillingMetrics) error {
	byPlanJSON, _ := json.Marshal(metrics.ByPlan)

	query := `
		INSERT INTO billing_metrics_daily (
			date, mrr_cents, arr_cents, active_subscriptions, churned_subscriptions,
			new_subscriptions, expansion_cents, contraction_cents,
			trial_conversions, trial_expired, by_plan, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (date) DO UPDATE SET
			mrr_cents = EXCLUDED.mrr_cents,
			arr_cents = EXCLUDED.arr_cents,
			active_subscriptions = EXCLUDED.active_subscriptions,
			churned_subscriptions = EXCLUDED.churned_subscriptions,
			new_subscriptions = EXCLUDED.new_subscriptions,
			expansion_cents = EXCLUDED.expansion_cents,
			contraction_cents = EXCLUDED.contraction_cents,
			trial_conversions = EXCLUDED.trial_conversions,
			trial_expired = EXCLUDED.trial_expired,
			by_plan = EXCLUDED.by_plan,
			updated_at = EXCLUDED.updated_at
	`

	_, err := r.db.Pool.Exec(ctx, query,
		metrics.Date,
		metrics.MRRCents,
		metrics.ARRCents,
		metrics.ActiveSubscriptions,
		metrics.ChurnedSubscriptions,
		metrics.NewSubscriptions,
		metrics.ExpansionCents,
		metrics.ContractionCents,
		metrics.TrialConversions,
		metrics.TrialExpired,
		byPlanJSON,
		time.Now(),
	)

	return err
}

// GetMetricsRange retrieves metrics for a date range
func (r *MetricsRepository) GetMetricsRange(ctx context.Context, from, to time.Time) ([]*DailyBillingMetrics, error) {
	query := `
		SELECT date, mrr_cents, arr_cents, active_subscriptions, churned_subscriptions,
		       new_subscriptions, expansion_cents, contraction_cents,
		       trial_conversions, trial_expired, by_plan
		FROM billing_metrics_daily
		WHERE date >= $1 AND date <= $2
		ORDER BY date ASC
	`

	rows, err := r.db.Pool.Query(ctx, query, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []*DailyBillingMetrics
	for rows.Next() {
		var m DailyBillingMetrics
		var byPlanBytes []byte
		var date pgtype.Date

		err := rows.Scan(
			&date,
			&m.MRRCents,
			&m.ARRCents,
			&m.ActiveSubscriptions,
			&m.ChurnedSubscriptions,
			&m.NewSubscriptions,
			&m.ExpansionCents,
			&m.ContractionCents,
			&m.TrialConversions,
			&m.TrialExpired,
			&byPlanBytes,
		)
		if err != nil {
			return nil, err
		}

		if date.Valid {
			m.Date = time.Date(int(date.Time.Year()), date.Time.Month(), date.Time.Day(), 0, 0, 0, 0, time.UTC)
		}

		if len(byPlanBytes) > 0 {
			json.Unmarshal(byPlanBytes, &m.ByPlan)
		}

		metrics = append(metrics, &m)
	}

	return metrics, nil
}

// GetLatestMetrics retrieves the most recent metrics
func (r *MetricsRepository) GetLatestMetrics(ctx context.Context) (*DailyBillingMetrics, error) {
	query := `
		SELECT date, mrr_cents, arr_cents, active_subscriptions, churned_subscriptions,
		       new_subscriptions, expansion_cents, contraction_cents,
		       trial_conversions, trial_expired, by_plan
		FROM billing_metrics_daily
		ORDER BY date DESC
		LIMIT 1
	`

	var m DailyBillingMetrics
	var byPlanBytes []byte
	var date pgtype.Date

	err := r.db.Pool.QueryRow(ctx, query).Scan(
		&date,
		&m.MRRCents,
		&m.ARRCents,
		&m.ActiveSubscriptions,
		&m.ChurnedSubscriptions,
		&m.NewSubscriptions,
		&m.ExpansionCents,
		&m.ContractionCents,
		&m.TrialConversions,
		&m.TrialExpired,
		&byPlanBytes,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if date.Valid {
		m.Date = time.Date(int(date.Time.Year()), date.Time.Month(), date.Time.Day(), 0, 0, 0, 0, time.UTC)
	}

	if len(byPlanBytes) > 0 {
		json.Unmarshal(byPlanBytes, &m.ByPlan)
	}

	return &m, nil
}

// DailyBillingMetrics represents daily aggregated billing metrics
type DailyBillingMetrics struct {
	Date                 time.Time              `json:"date"`
	MRRCents             int64                  `json:"mrr_cents"`
	ARRCents             int64                  `json:"arr_cents"`
	ActiveSubscriptions  int                    `json:"active_subscriptions"`
	ChurnedSubscriptions int                    `json:"churned_subscriptions"`
	NewSubscriptions     int                    `json:"new_subscriptions"`
	ExpansionCents       int64                  `json:"expansion_cents"`
	ContractionCents     int64                  `json:"contraction_cents"`
	TrialConversions     int                    `json:"trial_conversions"`
	TrialExpired         int                    `json:"trial_expired"`
	ByPlan               map[string]PlanMetrics `json:"by_plan"`
}

// PlanMetrics represents metrics broken down by plan
type PlanMetrics struct {
	Count         int   `json:"count"`
	MRRCents      int64 `json:"mrr_cents"`
	Subscriptions int   `json:"subscriptions"`
}

// ComputeDailyMetrics calculates billing metrics for a given date.
// MRR conta tenants ACTIVE pelo preco do plano (inclusive ativacoes manuais
// do super admin); churn e novas assinaturas usam cancelled_at/activated_at
// no mes corrente; expansao/contracao vem dos creditos de proration do dia.
func (r *MetricsRepository) ComputeDailyMetrics(ctx context.Context, date time.Time) (*DailyBillingMetrics, error) {
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)
	monthStart := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)

	rows, err := r.db.Pool.Query(ctx, `
		SELECT plan, status, activated_at, cancelled_at, COALESCE(trial_expires_at, trial_until)
		FROM tenants`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := &DailyBillingMetrics{Date: dayStart, ByPlan: map[string]PlanMetrics{}}
	in := func(t *time.Time, from, to time.Time) bool {
		return t != nil && !t.Before(from) && t.Before(to)
	}
	for rows.Next() {
		var plan, status string
		var activatedAt, cancelledAt, trialEnd *time.Time
		if err := rows.Scan(&plan, &status, &activatedAt, &cancelledAt, &trialEnd); err != nil {
			return nil, err
		}
		if status == "ACTIVE" {
			price := PlanPriceCents(plan)
			m.ActiveSubscriptions++
			m.MRRCents += price
			bp := m.ByPlan[plan]
			bp.Count++
			bp.MRRCents += price
			bp.Subscriptions++
			m.ByPlan[plan] = bp
		}
		if in(cancelledAt, monthStart, dayEnd) {
			m.ChurnedSubscriptions++
		}
		if in(activatedAt, monthStart, dayEnd) {
			m.NewSubscriptions++
		}
		if in(activatedAt, dayStart, dayEnd) && trialEnd != nil {
			m.TrialConversions++
		}
		if in(trialEnd, dayStart, dayEnd) && status != "ACTIVE" {
			m.TrialExpired++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	m.ARRCents = m.MRRCents * 12

	err = r.db.Pool.QueryRow(ctx, `SELECT
			COALESCE(SUM(-amount_cents) FILTER (WHERE reason = 'upgrade'), 0),
			COALESCE(SUM(amount_cents) FILTER (WHERE reason = 'downgrade'), 0)
		FROM billing_proration_credits WHERE created_at >= $1 AND created_at < $2`, dayStart, dayEnd).
		Scan(&m.ExpansionCents, &m.ContractionCents)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// PlanPriceCents returns the monthly price of a plan in cents.
func PlanPriceCents(plan string) int64 {
	switch plan {
	case "starter":
		return 9900
	case "pro":
		return 29900
	case "enterprise":
		return 99900
	}
	return 0
}
