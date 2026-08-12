package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ProrationRepository handles proration credit storage
type ProrationRepository struct {
	db *Database
}

// NewProrationRepository creates a new ProrationRepository
func NewProrationRepository(db *Database) *ProrationRepository {
	return &ProrationRepository{db: db}
}

// Create creates a new proration credit/debit record
func (r *ProrationRepository) Create(ctx context.Context, credit *ProrationCredit) error {
	query := `
		INSERT INTO billing_proration_credits (
			tenant_id, subscription_gateway_id, amount_cents, reason,
			period_start, period_end, days_calculated,
			daily_rate_old_cents, daily_rate_new_cents, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`

	var id string
	err := r.db.Pool.QueryRow(ctx, query,
		credit.TenantID,
		credit.SubscriptionGatewayID,
		credit.AmountCents,
		credit.Reason,
		credit.PeriodStart,
		credit.PeriodEnd,
		credit.DaysCalculated,
		credit.DailyRateOldCents,
		credit.DailyRateNewCents,
		time.Now(),
	).Scan(&id)

	if err != nil {
		return err
	}
	credit.ID = id
	return nil
}

// GetPendingCredits retrieves unapplied proration credits for a subscription
func (r *ProrationRepository) GetPendingCredits(ctx context.Context, subscriptionGatewayID string) ([]*ProrationCredit, error) {
	query := `
		SELECT id, tenant_id, subscription_gateway_id, amount_cents, reason,
		       period_start, period_end, days_calculated,
		       daily_rate_old_cents, daily_rate_new_cents,
		       applied_at, applied_payment_gateway_id, created_at
		FROM billing_proration_credits
		WHERE subscription_gateway_id = $1 AND applied_at IS NULL
		ORDER BY created_at ASC
	`

	rows, err := r.db.Pool.Query(ctx, query, subscriptionGatewayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var credits []*ProrationCredit
	for rows.Next() {
		var c ProrationCredit
		var appliedAt pgtype.Timestamptz
		var appliedPaymentGatewayID pgtype.Text

		err := rows.Scan(
			&c.ID,
			&c.TenantID,
			&c.SubscriptionGatewayID,
			&c.AmountCents,
			&c.Reason,
			&c.PeriodStart,
			&c.PeriodEnd,
			&c.DaysCalculated,
			&c.DailyRateOldCents,
			&c.DailyRateNewCents,
			&appliedAt,
			&appliedPaymentGatewayID,
			&c.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if appliedAt.Valid {
			c.AppliedAt = &appliedAt.Time
		}
		if appliedPaymentGatewayID.Valid {
			c.AppliedPaymentGatewayID = &appliedPaymentGatewayID.String
		}

		credits = append(credits, &c)
	}

	return credits, nil
}

// MarkApplied marks a proration credit as applied to a payment
func (r *ProrationRepository) MarkApplied(ctx context.Context, creditID string, paymentGatewayID string) error {
	query := `
		UPDATE billing_proration_credits
		SET applied_at = $1, applied_payment_gateway_id = $2
		WHERE id = $3
	`

	_, err := r.db.Pool.Exec(ctx, query, time.Now(), paymentGatewayID, creditID)
	return err
}

// GetTenantCredits retrieves all proration credits for a tenant
func (r *ProrationRepository) GetTenantCredits(ctx context.Context, tenantID string) ([]*ProrationCredit, error) {
	query := `
		SELECT id, tenant_id, subscription_gateway_id, amount_cents, reason,
		       period_start, period_end, days_calculated,
		       daily_rate_old_cents, daily_rate_new_cents,
		       applied_at, applied_payment_gateway_id, created_at
		FROM billing_proration_credits
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var credits []*ProrationCredit
	for rows.Next() {
		var c ProrationCredit
		var appliedAt pgtype.Timestamptz
		var appliedPaymentGatewayID pgtype.Text

		err := rows.Scan(
			&c.ID,
			&c.TenantID,
			&c.SubscriptionGatewayID,
			&c.AmountCents,
			&c.Reason,
			&c.PeriodStart,
			&c.PeriodEnd,
			&c.DaysCalculated,
			&c.DailyRateOldCents,
			&c.DailyRateNewCents,
			&appliedAt,
			&appliedPaymentGatewayID,
			&c.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if appliedAt.Valid {
			c.AppliedAt = &appliedAt.Time
		}
		if appliedPaymentGatewayID.Valid {
			c.AppliedPaymentGatewayID = &appliedPaymentGatewayID.String
		}

		credits = append(credits, &c)
	}

	return credits, nil
}

// ProrationCredit represents a proration credit/debit record
type ProrationCredit struct {
	ID                        string
	TenantID                  string
	SubscriptionGatewayID     string
	AmountCents               int64    // positivo = credito, negativo = debito
	Reason                    string   // 'upgrade' | 'downgrade'
	PeriodStart               time.Time
	PeriodEnd                 time.Time
	DaysCalculated            int
	DailyRateOldCents         int64
	DailyRateNewCents         int64
	AppliedAt                 *time.Time
	AppliedPaymentGatewayID   *string
	CreatedAt                 time.Time
}

// CalculateProration calculates the proration amount for a plan change
// Returns amount in cents (positive = credit to tenant, negative = charge to tenant)
func CalculateProration(
	oldPlanPriceCents int64,
	newPlanPriceCents int64,
	currentPeriodStart time.Time,
	currentPeriodEnd time.Time,
	changeDate time.Time,
) (amountCents int64, daysCalculated int, dailyRateOldCents int64, dailyRateNewCents int64) {
	// Calculate days remaining in current period
	daysInPeriod := int(currentPeriodEnd.Sub(currentPeriodStart).Hours() / 24)
	daysElapsed := int(changeDate.Sub(currentPeriodStart).Hours() / 24)
	daysRemaining := daysInPeriod - daysElapsed

	if daysRemaining <= 0 {
		return 0, 0, 0, 0
	}

	// Daily rates
	dailyRateOld := float64(oldPlanPriceCents) / float64(daysInPeriod)
	dailyRateNew := float64(newPlanPriceCents) / float64(daysInPeriod)

	dailyRateOldCents = int64(dailyRateOld)
	dailyRateNewCents = int64(dailyRateNew)

	// Difference per day * days remaining
	diffPerDay := dailyRateNew - dailyRateOld
	amountCents = int64(diffPerDay * float64(daysRemaining))

	return amountCents, daysRemaining, dailyRateOldCents, dailyRateNewCents
}