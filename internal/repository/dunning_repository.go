package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// DunningRepository handles dunning event storage
type DunningRepository struct {
	db *Database
}

// NewDunningRepository creates a new DunningRepository
func NewDunningRepository(db *Database) *DunningRepository {
	return &DunningRepository{db: db}
}

// Create creates a new dunning event record
func (r *DunningRepository) Create(ctx context.Context, tenantID string, paymentGatewayID string, stage int) error {
	query := `
		INSERT INTO billing_dunning_events (tenant_id, payment_gateway_id, stage, created_at)
		VALUES ($1, $2, $3, $4)
	`

	_, err := r.db.Pool.Exec(ctx, query, tenantID, paymentGatewayID, stage, time.Now())
	return err
}

// MarkEmailSent marks a dunning event as email sent
func (r *DunningRepository) MarkEmailSent(ctx context.Context, tenantID string, paymentGatewayID string, stage int, template string) error {
	query := `
		UPDATE billing_dunning_events
		SET email_sent_at = $1, email_template = $2
		WHERE tenant_id = $3 AND payment_gateway_id = $4 AND stage = $5
	`

	_, err := r.db.Pool.Exec(ctx, query, time.Now(), template, tenantID, paymentGatewayID, stage)
	return err
}

// GetPendingDunning retrieves tenants that need dunning emails
func (r *DunningRepository) GetPendingDunning(ctx context.Context, stage int, before time.Time) ([]*DunningPending, error) {
	query := `
		SELECT DISTINCT ON (bde.tenant_id) bde.tenant_id, bde.payment_gateway_id, bde.stage, bde.created_at,
		       t.payment_customer_id, t.payment_subscription_id, t.email as tenant_email, t.name as tenant_name
		FROM billing_dunning_events bde
		JOIN tenants t ON t.id = bde.tenant_id
		WHERE bde.stage = $1
		AND bde.email_sent_at IS NULL
		AND bde.created_at <= $2
		AND t.status = 'PENDING_PAYMENT'
		AND t.payment_subscription_id IS NOT NULL
		ORDER BY bde.tenant_id, bde.created_at
	`

	rows, err := r.db.Pool.Query(ctx, query, stage, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pending []*DunningPending
	for rows.Next() {
		var p DunningPending
		var createdAt pgtype.Timestamptz
		var paymentCustomerID, paymentSubscriptionID, tenantEmail, tenantName *string

		err := rows.Scan(
			&p.TenantID,
			&p.PaymentGatewayID,
			&p.Stage,
			&createdAt,
			&paymentCustomerID,
			&paymentSubscriptionID,
			&tenantEmail,
			&tenantName,
		)
		if err != nil {
			return nil, err
		}

		if createdAt.Valid {
			p.CreatedAt = createdAt.Time
		}
		p.PaymentCustomerID = paymentCustomerID
		p.PaymentSubscriptionID = paymentSubscriptionID
		p.TenantEmail = tenantEmail
		p.TenantName = tenantName

		pending = append(pending, &p)
	}

	return pending, nil
}

// DunningPending represents a tenant pending dunning email
type DunningPending struct {
	TenantID             string
	PaymentGatewayID     string
	Stage                int
	CreatedAt            time.Time
	PaymentCustomerID    *string
	PaymentSubscriptionID *string
	TenantEmail          *string
	TenantName           *string
}

// GetDunningHistory retrieves dunning history for a tenant
func (r *DunningRepository) GetDunningHistory(ctx context.Context, tenantID string) ([]*DunningEvent, error) {
	query := `
		SELECT id, tenant_id, payment_gateway_id, stage, email_sent_at, email_template, created_at
		FROM billing_dunning_events
		WHERE tenant_id = $1
		ORDER BY stage, created_at
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*DunningEvent
	for rows.Next() {
		var e DunningEvent
		var emailSentAt, createdAt pgtype.Timestamptz
		var emailTemplate *string

		err := rows.Scan(
			&e.ID,
			&e.TenantID,
			&e.PaymentGatewayID,
			&e.Stage,
			&emailSentAt,
			&emailTemplate,
			&createdAt,
		)
		if err != nil {
			return nil, err
		}

		if emailSentAt.Valid {
			e.EmailSentAt = &emailSentAt.Time
		}
		e.EmailTemplate = emailTemplate
		if createdAt.Valid {
			e.CreatedAt = createdAt.Time
		}

		events = append(events, &e)
	}

	return events, nil
}

// DunningEvent represents a dunning event record
type DunningEvent struct {
	ID                   string
	TenantID             string
	PaymentGatewayID     string
	Stage                int
	EmailSentAt          *time.Time
	EmailTemplate        *string
	CreatedAt            time.Time
}