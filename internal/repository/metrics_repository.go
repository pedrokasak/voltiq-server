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
	Date                  time.Time
	MRRCents              int64
	ARRCents              int64
	ActiveSubscriptions   int
	ChurnedSubscriptions  int
	NewSubscriptions      int
	ExpansionCents        int64
	ContractionCents      int64
	TrialConversions      int
	TrialExpired          int
	ByPlan                map[string]PlanMetrics
}

// PlanMetrics represents metrics broken down by plan
type PlanMetrics struct {
	Count        int   `json:"count"`
	MRRCents     int64 `json:"mrr_cents"`
	Subscriptions int  `json:"subscriptions"`
}

// ComputeDailyMetrics calculates billing metrics for a given date
func (r *MetricsRepository) ComputeDailyMetrics(ctx context.Context, date time.Time) (*DailyBillingMetrics, error) {
	// Get all active tenants with their subscriptions
	query := `
		SELECT t.id, t.plan, t.status, t.payment_subscription_id,
		       t.max_users, t.seat_count, t.created_at, t.updated_at
		FROM tenants t
		WHERE t.deleted_at IS NULL
		AND t.status IN ('TRIAL', 'ACTIVE', 'SUSPENDED', 'PENDING_PAYMENT')
	`

	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type tenantSub struct {
		id                    string
		plan                  string
		status                string
		paymentSubscriptionID *string
		maxUsers              int
		seatCount             int
		createdAt             time.Time
		updatedAt             time.Time
	}

	var tenants []tenantSub
	for rows.Next() {
		var t tenantSub
		var paymentSubID pgtype.Text

		err := rows.Scan(
			&t.id,
			&t.plan,
			&t.status,
			&paymentSubID,
			&t.maxUsers,
			&t.seatCount,
			&t.createdAt,
			&t.updatedAt,
		)
		if err != nil {
			return nil, err
		}

		if paymentSubID.Valid {
			t.paymentSubscriptionID = &paymentSubID.String
		}

		tenants = append(tenants, t)
	}

	// Plan prices in cents
	planPrices := map[string]int64{
		"starter":    9900,
		"pro":        29900,
		"enterprise": 99900,
	}

	// Calculate metrics
	var mrrCents int64
	var activeSubs int
	var churnedSubs int
	var newSubs int
	var trialConversions int
	var trialExpired int
	byPlan := make(map[string]PlanMetrics)

	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)
	monthStart := time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, time.UTC)

	for _, t := range tenants {
		planPrice := planPrices[t.plan]

		if t.status == "ACTIVE" && t.paymentSubscriptionID != nil {
			activeSubs++
			mrrCents += planPrice

			// By plan breakdown
			bp := byPlan[t.plan]
			bp.Count++
			bp.MRRCents += planPrice
			bp.Subscriptions++
			byPlan[t.plan] = bp
		}

		if t.status == "CANCELLED" && t.updatedAt.After(monthStart) && t.updatedAt.Before(dayEnd) {
			churnedSubs++
		}

		if t.status == "ACTIVE" && t.createdAt.After(monthStart) && t.createdAt.Before(dayEnd) {
			newSubs++
		}

		if t.status == "ACTIVE" && t.paymentSubscriptionID != nil {
			// Check if was TRIAL last month
			// This is simplified - in reality would check historical data
		}

		if t.status != "TRIAL" && t.paymentSubscriptionID != nil && t.createdAt.After(monthStart) && t.createdAt.Before(dayEnd) {
			// Could be trial conversion
		}
	}

	// ARR = MRR * 12
	arrCents := mrrCents * 12

	// TODO: Calculate expansion/contraction from plan changes
	// This would require historical plan data

	return &DailyBillingMetrics{
		Date:                  dayStart,
		MRRCents:              mrrCents,
		ARRCents:              arrCents,
		ActiveSubscriptions:   activeSubs,
		ChurnedSubscriptions:  churnedSubs,
		NewSubscriptions:      newSubs,
		TrialConversions:      trialConversions,
		TrialExpired:          trialExpired,
		ByPlan:                byPlan,
		ExpansionCents:        0,
		ContractionCents:      0,
	}, nil
}