package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
)

// AlertRepository handles alert data access
type AlertRepository struct {
	db *Database
}

// NewAlertRepository creates a new AlertRepository
func NewAlertRepository(db *Database) *AlertRepository {
	return &AlertRepository{db: db}
}

// Create inserts a new alert into the database
func (r *AlertRepository) Create(ctx context.Context, alert *domain.Alert) error {
	query := `
		INSERT INTO alerts (
			id, tenant_id, transformer_id, balance_id, type, channel, recipient,
			delivery_status, sent_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		alert.ID,
		alert.TenantID,
		alert.TransformerID,
		alert.BalanceID,
		alert.Type,
		alert.Channel,
		alert.Recipient,
		alert.DeliveryStatus,
		alert.SentAt,
		alert.CreatedAt,
	)

	return err
}

// GetByID retrieves an alert by ID
func (r *AlertRepository) GetByID(ctx context.Context, id domain.UUID) (*domain.Alert, error) {
	query := `
		SELECT id, tenant_id, transformer_id, balance_id, type, channel, recipient,
			delivery_status, sent_at, created_at
		FROM alerts
		WHERE id = $1
	`

	alert := &domain.Alert{}

	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&alert.ID,
		&alert.TenantID,
		&alert.TransformerID,
		&alert.BalanceID,
		&alert.Type,
		&alert.Channel,
		&alert.Recipient,
		&alert.DeliveryStatus,
		&alert.SentAt,
		&alert.CreatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return alert, nil
}

// GetByTransformer retrieves all alerts for a transformer
func (r *AlertRepository) GetByTransformer(ctx context.Context, transformerID domain.UUID) ([]*domain.Alert, error) {
	query := `
		SELECT id, tenant_id, transformer_id, balance_id, type, channel, recipient,
			delivery_status, sent_at, created_at
		FROM alerts
		WHERE transformer_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, transformerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*domain.Alert
	for rows.Next() {
		alert := &domain.Alert{}

		err := rows.Scan(
			&alert.ID,
			&alert.TenantID,
			&alert.TransformerID,
			&alert.BalanceID,
			&alert.Type,
			&alert.Channel,
			&alert.Recipient,
			&alert.DeliveryStatus,
			&alert.SentAt,
			&alert.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// GetByTenant retrieves all alerts for a tenant
func (r *AlertRepository) GetByTenant(ctx context.Context, tenantID domain.UUID) ([]*domain.Alert, error) {
	query := `
		SELECT id, tenant_id, transformer_id, balance_id, type, channel, recipient,
			delivery_status, sent_at, created_at
		FROM alerts
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*domain.Alert
	for rows.Next() {
		alert := &domain.Alert{}

		err := rows.Scan(
			&alert.ID,
			&alert.TenantID,
			&alert.TransformerID,
			&alert.BalanceID,
			&alert.Type,
			&alert.Channel,
			&alert.Recipient,
			&alert.DeliveryStatus,
			&alert.SentAt,
			&alert.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// Update updates an existing alert
func (r *AlertRepository) Update(ctx context.Context, alert *domain.Alert) error {
	query := `
		UPDATE alerts
		SET type = $1, channel = $2, recipient = $3, delivery_status = $4, sent_at = $5
		WHERE id = $6
	`

	_, err := r.db.Pool.Exec(ctx, query,
		alert.Type,
		alert.Channel,
		alert.Recipient,
		alert.DeliveryStatus,
		alert.SentAt,
		alert.ID,
	)

	return err
}

// Delete soft deletes an alert
func (r *AlertRepository) Delete(ctx context.Context, id domain.UUID) error {
	query := `
		DELETE FROM alerts WHERE id = $1
	`

	_, err := r.db.Pool.Exec(ctx, query, id)
	return err
}