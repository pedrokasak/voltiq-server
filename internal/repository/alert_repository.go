package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
)

// AlertRepository handles alert recipients, transformer groups and the
// alert delivery history.
type AlertRepository struct {
	db *Database
}

// NewAlertRepository creates a new AlertRepository
func NewAlertRepository(db *Database) *AlertRepository {
	return &AlertRepository{db: db}
}

// ---------------------------------------------------------------- recipients

const recipientSelect = `
	SELECT r.id, r.tenant_id, r.transformer_id, t.code, r.group_id, g.name, r.channel, r.recipient,
		r.min_level, r.active, r.created_at, r.updated_at
	FROM alert_recipients r
	LEFT JOIN transformers t ON t.id = r.transformer_id
	LEFT JOIN transformer_groups g ON g.id = r.group_id`

func scanRecipient(row pgx.Row) (*domain.AlertRecipient, error) {
	a := &domain.AlertRecipient{}
	err := row.Scan(&a.ID, &a.TenantID, &a.TransformerID, &a.TransformerCode, &a.GroupID, &a.GroupName,
		&a.Channel, &a.Recipient, &a.MinLevel, &a.Active, &a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func collectRecipients(rows pgx.Rows, err error) ([]*domain.AlertRecipient, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.AlertRecipient{}
	for rows.Next() {
		a, err := scanRecipient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateRecipient inserts an alert recipient.
func (r *AlertRepository) CreateRecipient(ctx context.Context, a *domain.AlertRecipient) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO alert_recipients (id, tenant_id, transformer_id, group_id, channel, recipient, min_level, active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)`,
		a.ID, a.TenantID, a.TransformerID, a.GroupID, a.Channel, a.Recipient, a.MinLevel, a.Active, a.CreatedAt)
	return err
}

// GetRecipient retrieves a recipient of the request tenant.
func (r *AlertRepository) GetRecipient(ctx context.Context, id domain.UUID) (*domain.AlertRecipient, error) {
	q, args := scopedColumn(ctx, "r.tenant_id", recipientSelect+` WHERE r.id = $1 AND r.deleted_at IS NULL /*tenant*/`, id)
	return scanRecipient(r.db.Pool.QueryRow(ctx, q, args...))
}

// ListRecipients lists the recipients of a tenant, optionally only those
// configured for one transformer.
func (r *AlertRepository) ListRecipients(ctx context.Context, tenantID domain.UUID, transformerID *domain.UUID) ([]*domain.AlertRecipient, error) {
	if transformerID != nil {
		return collectRecipients(r.db.Pool.Query(ctx, recipientSelect+`
			WHERE r.tenant_id = $1 AND r.deleted_at IS NULL AND r.transformer_id = $2
			ORDER BY r.created_at`, tenantID, *transformerID))
	}
	return collectRecipients(r.db.Pool.Query(ctx, recipientSelect+`
		WHERE r.tenant_id = $1 AND r.deleted_at IS NULL ORDER BY r.created_at`, tenantID))
}

// RecipientsFor returns the active recipients that cover a transformer:
// tenant-wide, of the transformer itself, or of a group containing it.
func (r *AlertRepository) RecipientsFor(ctx context.Context, tenantID, transformerID domain.UUID) ([]*domain.AlertRecipient, error) {
	return collectRecipients(r.db.Pool.Query(ctx, recipientSelect+`
		WHERE r.tenant_id = $1 AND r.deleted_at IS NULL AND r.active
			AND (
				(r.transformer_id IS NULL AND r.group_id IS NULL)
				OR r.transformer_id = $2
				OR r.group_id IN (SELECT group_id FROM transformer_group_members WHERE transformer_id = $2)
			)`, tenantID, transformerID))
}

// UpdateRecipient saves channel, recipient, level, scope and active flag.
func (r *AlertRepository) UpdateRecipient(ctx context.Context, a *domain.AlertRecipient) error {
	q, args := scoped(ctx, `
		UPDATE alert_recipients SET transformer_id = $1, group_id = $2, channel = $3, recipient = $4,
			min_level = $5, active = $6, updated_at = NOW()
		WHERE id = $7 AND deleted_at IS NULL /*tenant*/`,
		a.TransformerID, a.GroupID, a.Channel, a.Recipient, a.MinLevel, a.Active, a.ID)
	tag, err := r.db.Pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteRecipient soft-deletes a recipient.
func (r *AlertRepository) DeleteRecipient(ctx context.Context, id domain.UUID) error {
	q, args := scoped(ctx, `UPDATE alert_recipients SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL /*tenant*/`, id)
	tag, err := r.db.Pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------- groups

// ListGroups lists the transformer groups of a tenant with their members.
func (r *AlertRepository) ListGroups(ctx context.Context, tenantID domain.UUID) ([]*domain.TransformerGroup, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT g.id, g.tenant_id, g.name, COALESCE(g.description, ''), g.created_at,
			COALESCE(array_agg(m.transformer_id::text) FILTER (WHERE m.transformer_id IS NOT NULL), '{}')
		FROM transformer_groups g
		LEFT JOIN transformer_group_members m ON m.group_id = g.id
		WHERE g.tenant_id = $1 AND g.deleted_at IS NULL
		GROUP BY g.id ORDER BY g.name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.TransformerGroup{}
	for rows.Next() {
		g := &domain.TransformerGroup{}
		var ids []string
		if err := rows.Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.CreatedAt, &ids); err != nil {
			return nil, err
		}
		g.TransformerIDs = make([]domain.UUID, len(ids))
		for i, id := range ids {
			g.TransformerIDs[i] = domain.UUID(id)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetGroup retrieves a group of the request tenant.
func (r *AlertRepository) GetGroup(ctx context.Context, id domain.UUID) (*domain.TransformerGroup, error) {
	g := &domain.TransformerGroup{}
	q, args := scoped(ctx, `SELECT id, tenant_id, name, COALESCE(description, ''), created_at
		FROM transformer_groups WHERE id = $1 AND deleted_at IS NULL /*tenant*/`, id)
	err := r.db.Pool.QueryRow(ctx, q, args...).Scan(&g.ID, &g.TenantID, &g.Name, &g.Description, &g.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return g, err
}

// SaveGroup creates or updates a group and replaces its members.
func (r *AlertRepository) SaveGroup(ctx context.Context, g *domain.TransformerGroup, create bool) error {
	return r.db.InTx(ctx, func(tx pgx.Tx) error {
		if create {
			if _, err := tx.Exec(ctx, `INSERT INTO transformer_groups (id, tenant_id, name, description, created_at, updated_at)
				VALUES ($1, $2, $3, NULLIF($4, ''), $5, $5)`, g.ID, g.TenantID, g.Name, g.Description, g.CreatedAt); err != nil {
				return err
			}
		} else {
			tag, err := tx.Exec(ctx, `UPDATE transformer_groups SET name = $1, description = NULLIF($2, ''), updated_at = NOW()
				WHERE id = $3 AND tenant_id = $4 AND deleted_at IS NULL`, g.Name, g.Description, g.ID, g.TenantID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return ErrNotFound
			}
			if _, err := tx.Exec(ctx, `DELETE FROM transformer_group_members WHERE group_id = $1`, g.ID); err != nil {
				return err
			}
		}
		for _, tid := range g.TransformerIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO transformer_group_members (group_id, transformer_id, tenant_id)
				SELECT $1, id, tenant_id FROM transformers WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
				ON CONFLICT DO NOTHING`, g.ID, tid, g.TenantID); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteGroup soft-deletes a group, removes its members and its recipients.
func (r *AlertRepository) DeleteGroup(ctx context.Context, tenantID, id domain.UUID) error {
	return r.db.InTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE transformer_groups SET deleted_at = NOW(), updated_at = NOW()
			WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`, id, tenantID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM transformer_group_members WHERE group_id = $1`, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE alert_recipients SET deleted_at = NOW() WHERE group_id = $1 AND deleted_at IS NULL`, id)
		return err
	})
}

// ---------------------------------------------------------------- deliveries

const deliverySelect = `
	SELECT a.id, a.tenant_id, a.transformer_id, COALESCE(t.code, ''), a.balance_id, a.recipient_id, a.type,
		a.channel, a.recipient, a.delivery_status, a.error_message, a.attempts, a.loss_pct::float8,
		a.limit_pct::float8, a.sent_at, a.created_at
	FROM alerts a
	LEFT JOIN transformers t ON t.id = a.transformer_id`

func scanDelivery(row pgx.Row) (*domain.AlertDelivery, error) {
	d := &domain.AlertDelivery{}
	err := row.Scan(&d.ID, &d.TenantID, &d.TransformerID, &d.TransformerCode, &d.BalanceID, &d.RecipientID, &d.Type,
		&d.Channel, &d.Recipient, &d.DeliveryStatus, &d.ErrorMessage, &d.Attempts, &d.LossPct,
		&d.LimitPct, &d.SentAt, &d.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return d, nil
}

// CreateDelivery records an alert to be sent. Returns false when the same
// balance was already alerted to the same recipient with the same type.
func (r *AlertRepository) CreateDelivery(ctx context.Context, d *domain.AlertDelivery) (bool, error) {
	defer invalidateFromContext(ctx)
	var exists bool
	if err := r.db.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM alerts WHERE balance_id = $1 AND recipient = $2 AND channel = $3
			AND type = $4 AND deleted_at IS NULL)`, d.BalanceID, d.Recipient, d.Channel, d.Type).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO alerts (id, tenant_id, transformer_id, balance_id, recipient_id, type, channel, recipient,
			delivery_status, attempts, loss_pct, limit_pct, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 0, $10, $11, $12, $12)`,
		d.ID, d.TenantID, d.TransformerID, d.BalanceID, d.RecipientID, d.Type, d.Channel, d.Recipient,
		d.DeliveryStatus, d.LossPct, d.LimitPct, d.CreatedAt)
	return err == nil, err
}

// MarkDelivery stores the result of a send attempt.
func (r *AlertRepository) MarkDelivery(ctx context.Context, id domain.UUID, status domain.AlertDeliveryStatus, errMsg *string) error {
	var sentAt *time.Time
	if status == domain.AlertDeliverySent {
		now := time.Now()
		sentAt = &now
	}
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE alerts SET delivery_status = $1, error_message = $2, sent_at = COALESCE($3, sent_at),
			attempts = attempts + 1, updated_at = NOW()
		WHERE id = $4`, status, errMsg, sentAt, id)
	return err
}

// DeliveryFilter filters the alert history.
type DeliveryFilter struct {
	TransformerID string
	Status        string
	Channel       string
	Type          string
	Since         *time.Time
	Limit         int
	Offset        int
}

// ListDeliveries lists the alert history of a tenant, newest first.
func (r *AlertRepository) ListDeliveries(ctx context.Context, tenantID domain.UUID, f DeliveryFilter) ([]*domain.AlertDelivery, int, error) {
	where := ` WHERE a.tenant_id = $1 AND a.deleted_at IS NULL
		AND ($2 = '' OR a.transformer_id::text = $2) AND ($3 = '' OR a.delivery_status = $3)
		AND ($4 = '' OR a.channel = $4) AND ($5 = '' OR a.type = $5)
		AND ($6::timestamptz IS NULL OR a.created_at >= $6)`
	args := []any{tenantID, f.TransformerID, f.Status, f.Channel, f.Type, f.Since}

	var total int
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM alerts a`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.db.Pool.Query(ctx, deliverySelect+where+` ORDER BY a.created_at DESC LIMIT $7 OFFSET $8`,
		append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*domain.AlertDelivery{}
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

// GetDelivery retrieves one alert of the request tenant.
func (r *AlertRepository) GetDelivery(ctx context.Context, id domain.UUID) (*domain.AlertDelivery, error) {
	q, args := scopedColumn(ctx, "a.tenant_id", deliverySelect+` WHERE a.id = $1 AND a.deleted_at IS NULL /*tenant*/`, id)
	return scanDelivery(r.db.Pool.QueryRow(ctx, q, args...))
}

// FailedForRetry returns failed or stuck alerts with fewer than maxAttempts.
func (r *AlertRepository) FailedForRetry(ctx context.Context, maxAttempts int, olderThan time.Time) ([]*domain.AlertDelivery, error) {
	rows, err := r.db.Pool.Query(ctx, deliverySelect+`
		WHERE a.deleted_at IS NULL AND a.attempts < $1 AND a.created_at < $2
			AND (a.delivery_status = 'ERROR' OR (a.delivery_status = 'PENDING' AND a.created_at < $2))
		ORDER BY a.created_at LIMIT 200`, maxAttempts, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*domain.AlertDelivery{}
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AlertStats counts the alerts of a tenant in a window.
type AlertStats struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Sent     int `json:"sent"`
	Failed   int `json:"failed"`
	Pending  int `json:"pending"`
}

// Stats returns alert counters since a time.
func (r *AlertRepository) Stats(ctx context.Context, tenantID domain.UUID, since time.Time) (*AlertStats, error) {
	s := &AlertStats{}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE type = 'CRITICAL'), COUNT(*) FILTER (WHERE type = 'WARNING'),
			COUNT(*) FILTER (WHERE delivery_status = 'SENT'), COUNT(*) FILTER (WHERE delivery_status = 'ERROR'),
			COUNT(*) FILTER (WHERE delivery_status = 'PENDING')
		FROM alerts WHERE tenant_id = $1 AND deleted_at IS NULL AND created_at >= $2`, tenantID, since,
	).Scan(&s.Total, &s.Critical, &s.Warning, &s.Sent, &s.Failed, &s.Pending)
	return s, err
}
