package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/voltiq/server/internal/payment"
)

// WebhookEventRepository handles webhook event storage for idempotency
type WebhookEventRepository struct {
	db *Database
}

// NewWebhookEventRepository creates a new WebhookEventRepository
func NewWebhookEventRepository(db *Database) *WebhookEventRepository {
	return &WebhookEventRepository{db: db}
}

// CreatePending creates a new webhook event record with pending status
func (r *WebhookEventRepository) CreatePending(ctx context.Context, event *payment.WebhookEvent) error {
	query := `
		INSERT INTO payment_webhook_events (gateway, event_id, event_type, payload, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (gateway, event_id) DO NOTHING
	`

	_, err := r.db.Pool.Exec(ctx, query,
		"asaas", // hardcoded for now, can be parameterized
		event.ID,
		string(event.Type),
		event.Raw,
		time.Now(),
	)

	return err
}

// Exists checks if a webhook event was already processed
func (r *WebhookEventRepository) Exists(ctx context.Context, gateway, eventID string) (bool, error) {
	query := `SELECT 1 FROM payment_webhook_events WHERE gateway = $1 AND event_id = $2 AND processed_at IS NOT NULL`

	var exists int
	err := r.db.Pool.QueryRow(ctx, query, gateway, eventID).Scan(&exists)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MarkProcessed marks a webhook event as successfully processed
func (r *WebhookEventRepository) MarkProcessed(ctx context.Context, gateway, eventID string) error {
	query := `
		UPDATE payment_webhook_events
		SET processed_at = $1
		WHERE gateway = $2 AND event_id = $3
	`

	_, err := r.db.Pool.Exec(ctx, query, time.Now(), gateway, eventID)
	return err
}

// MarkFailed marks a webhook event as failed with error message
func (r *WebhookEventRepository) MarkFailed(ctx context.Context, gateway, eventID, errorMsg string) error {
	query := `
		UPDATE payment_webhook_events
		SET processed_at = $1, processing_error = $2
		WHERE gateway = $3 AND event_id = $4
	`

	_, err := r.db.Pool.Exec(ctx, query, time.Now(), errorMsg, gateway, eventID)
	return err
}

// GetPendingEvents retrieves unprocessed webhook events for retry
func (r *WebhookEventRepository) GetPendingEvents(ctx context.Context, limit int) ([]*payment.WebhookEvent, error) {
	query := `
		SELECT gateway, event_id, event_type, payload, created_at
		FROM payment_webhook_events
		WHERE processed_at IS NULL
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := r.db.Pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*payment.WebhookEvent
	for rows.Next() {
		var event payment.WebhookEvent
		var gateway, eventType string
		var createdAt pgtype.Timestamptz

		err := rows.Scan(
			&gateway,
			&event.ID,
			&eventType,
			&event.Raw,
			&createdAt,
		)
		if err != nil {
			return nil, err
		}

		event.Type = payment.WebhookEventType(eventType)
		if createdAt.Valid {
			event.Timestamp = createdAt.Time
		}

		events = append(events, &event)
	}

	return events, nil
}

// GetPendingRetries retrieves webhook events that failed processing and are ready for retry
func (r *WebhookEventRepository) GetPendingRetries(ctx context.Context, gateway string, cutoff time.Time) ([]*payment.WebhookEvent, error) {
	query := `
		SELECT gateway, event_id, event_type, payload, created_at, processing_error
		FROM payment_webhook_events
		WHERE gateway = $1 AND processed_at IS NULL AND processing_error IS NOT NULL AND created_at <= $2
		ORDER BY created_at ASC
		LIMIT 100
	`

	rows, err := r.db.Pool.Query(ctx, query, gateway, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*payment.WebhookEvent
	for rows.Next() {
		var event payment.WebhookEvent
		var createdAt pgtype.Timestamptz
		var processingError pgtype.Text

		err := rows.Scan(
			&event.Gateway,
			&event.ID,
			&event.Type,
			&event.Raw,
			&createdAt,
			&processingError,
		)
		if err != nil {
			return nil, err
		}

		if createdAt.Valid {
			event.Timestamp = createdAt.Time
		}
		if processingError.Valid {
			event.Raw = []byte(processingError.String)
		}

		events = append(events, &event)
	}

	return events, nil
}