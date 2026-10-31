package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
)

// AccountRepository reads tenant-level settings, usage counters and the
// audit trail.
type AccountRepository struct {
	db *Database
}

// NewAccountRepository creates an AccountRepository
func NewAccountRepository(db *Database) *AccountRepository {
	return &AccountRepository{db: db}
}

// TenantSettings are the editable settings of a tenant.
type TenantSettings struct {
	TenantID              domain.UUID `json:"tenant_id"`
	Name                  string      `json:"name"`
	Document              string      `json:"document"`
	TariffBRLMWh          *float64    `json:"tariff_brl_mwh"`
	OnboardingCompletedAt *time.Time  `json:"onboarding_completed_at"`
	Address               string      `json:"address"`
	AddressNumber         string      `json:"address_number"`
	Province              string      `json:"province"`
	PostalCode            string      `json:"postal_code"`
}

// GetSettings returns the settings of a tenant.
func (r *AccountRepository) GetSettings(ctx context.Context, tenantID domain.UUID) (*TenantSettings, error) {
	s := &TenantSettings{TenantID: tenantID}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT name, COALESCE(document, ''), tariff_brl_mwh::float8, onboarding_completed_at,
			COALESCE(address, ''), COALESCE(address_number, ''), COALESCE(province, ''), COALESCE(postal_code, '')
		FROM tenants WHERE id = $1`, tenantID,
	).Scan(&s.Name, &s.Document, &s.TariffBRLMWh, &s.OnboardingCompletedAt, &s.Address, &s.AddressNumber, &s.Province, &s.PostalCode)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateSettings saves the editable settings of a tenant.
func (r *AccountRepository) UpdateSettings(ctx context.Context, s *TenantSettings) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE tenants SET name = $1, tariff_brl_mwh = $2, address = $3, address_number = $4,
			province = $5, postal_code = $6, updated_at = NOW()
		WHERE id = $7`,
		s.Name, s.TariffBRLMWh, s.Address, s.AddressNumber, s.Province, s.PostalCode, s.TenantID)
	return err
}

// SetOnboardingCompleted marks (or clears) the onboarding as completed.
func (r *AccountRepository) SetOnboardingCompleted(ctx context.Context, tenantID domain.UUID, at *time.Time) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE tenants SET onboarding_completed_at = $1, updated_at = NOW() WHERE id = $2`, at, tenantID)
	return err
}

// Usage counts what a tenant has stored.
type Usage struct {
	Users           int        `json:"users"`
	PendingInvites  int        `json:"pending_invites"`
	Transformers    int        `json:"transformers"`
	ConsumingUnits  int        `json:"consuming_units"`
	Substations     int        `json:"substations"`
	Imports         int        `json:"imports"`
	CompletedImport int        `json:"completed_imports"`
	Balances        int        `json:"balances"`
	Recipients      int        `json:"alert_recipients"`
	Readings        int64      `json:"readings"`
	LastImportAt    *time.Time `json:"last_import_at"`
	LastBalanceAt   *time.Time `json:"last_balance_at"`
}

// GetUsage returns the usage counters of a tenant.
func (r *AccountRepository) GetUsage(ctx context.Context, tenantID domain.UUID) (*Usage, error) {
	u := &Usage{}
	err := r.db.Pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users WHERE tenant_id = $1 AND deleted_at IS NULL AND role <> 'SUPER_ADMIN'),
			(SELECT COUNT(*) FROM invites WHERE tenant_id = $1 AND status = 'PENDING' AND deleted_at IS NULL AND expires_at > NOW()),
			(SELECT COUNT(*) FROM transformers WHERE tenant_id = $1 AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM consuming_units WHERE tenant_id = $1 AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM substations WHERE tenant_id = $1 AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM imports WHERE tenant_id = $1 AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM imports WHERE tenant_id = $1 AND deleted_at IS NULL AND status = 'COMPLETED'),
			(SELECT COUNT(*) FROM transformer_balance WHERE tenant_id = $1),
			(SELECT COUNT(*) FROM alert_recipients WHERE tenant_id = $1 AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM transformer_readings WHERE tenant_id = $1) + (SELECT COUNT(*) FROM consuming_unit_readings WHERE tenant_id = $1),
			(SELECT MAX(created_at) FROM imports WHERE tenant_id = $1 AND deleted_at IS NULL),
			(SELECT MAX(calculated_at) FROM transformer_balance WHERE tenant_id = $1)`, tenantID,
	).Scan(&u.Users, &u.PendingInvites, &u.Transformers, &u.ConsumingUnits, &u.Substations, &u.Imports,
		&u.CompletedImport, &u.Balances, &u.Recipients, &u.Readings, &u.LastImportAt, &u.LastBalanceAt)
	return u, err
}

// AuditEntry is one row of the audit trail (RF07.4).
type AuditEntry struct {
	ID        domain.UUID     `json:"id"`
	TableName string          `json:"table_name"`
	Operation string          `json:"operation"`
	UserEmail *string         `json:"user_email"`
	OldData   json.RawMessage `json:"old_data,omitempty"`
	NewData   json.RawMessage `json:"new_data,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// AuditFilter filters the audit trail.
type AuditFilter struct {
	Table     string
	Operation string
	UserEmail string
	Since     *time.Time
	Until     *time.Time
	Limit     int
	Offset    int
}

// ListAudit lists the audit trail of a tenant, newest first.
func (r *AccountRepository) ListAudit(ctx context.Context, tenantID domain.UUID, f AuditFilter) ([]*AuditEntry, int, error) {
	where := ` WHERE tenant_id = $1 AND ($2 = '' OR table_name = $2) AND ($3 = '' OR operation = $3)
		AND ($4 = '' OR user_email ILIKE '%' || $4 || '%')
		AND ($5::timestamptz IS NULL OR created_at >= $5) AND ($6::timestamptz IS NULL OR created_at <= $6)`
	args := []any{tenantID, f.Table, f.Operation, f.UserEmail, f.Since, f.Until}
	var total int
	if err := r.db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_log`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.db.Pool.Query(ctx, `SELECT id, table_name, operation, user_email, old_data, new_data, created_at
		FROM audit_log`+where+` ORDER BY created_at DESC LIMIT $7 OFFSET $8`, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out, err := scanAuditRows(rows)
	return out, total, err
}

// scanAuditRows scans id, table_name, operation, user_email, old_data,
// new_data, created_at rows.
func scanAuditRows(rows pgx.Rows) ([]*AuditEntry, error) {
	out := []*AuditEntry{}
	for rows.Next() {
		e := &AuditEntry{}
		var oldData, newData []byte
		if err := rows.Scan(&e.ID, &e.TableName, &e.Operation, &e.UserEmail, &oldData, &newData, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.OldData, e.NewData = oldData, newData
		out = append(out, e)
	}
	return out, rows.Err()
}

// LogAction records an application-level action (super admin operations,
// logins) that is not a row change captured by the triggers.
func (r *AccountRepository) LogAction(ctx context.Context, tenantID *domain.UUID, actorEmail, action string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = r.db.Pool.Exec(ctx, `INSERT INTO audit_log (table_name, operation, new_data, user_email, tenant_id, created_at)
		VALUES ('app', $1, $2, NULLIF($3, ''), $4, NOW())`, action, payload, actorEmail, tenantID)
	return err
}
