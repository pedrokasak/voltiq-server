package repository

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/voltiq/server/internal/domain"
)

// PlatformRepository answers cross-tenant questions for the super admin
// panel (SaaS metrics, tenant overview). It never filters by tenant.
type PlatformRepository struct {
	db *Database
}

// NewPlatformRepository creates a PlatformRepository
func NewPlatformRepository(db *Database) *PlatformRepository {
	return &PlatformRepository{db: db}
}

// PlanCount is the number of tenants of a plan per status.
type PlanCount struct {
	Plan   string `json:"plan"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// LifecycleCounts summarises tenant lifecycle events in a window.
type LifecycleCounts struct {
	NewTenants       int   `json:"new_tenants"`
	Activated        int   `json:"activated"`
	Cancelled        int   `json:"cancelled"`
	Suspended        int   `json:"suspended"`
	TrialsEnded      int   `json:"trials_ended"`
	TrialsConverted  int   `json:"trials_converted"`
	ExpansionCents   int64 `json:"expansion_cents"`
	ContractionCents int64 `json:"contraction_cents"`
}

// CountByPlanStatus returns tenant counts grouped by plan and status.
func (r *PlatformRepository) CountByPlanStatus(ctx context.Context) ([]PlanCount, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT plan, status, COUNT(*) FROM tenants
		GROUP BY plan, status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanCount
	for rows.Next() {
		var c PlanCount
		if err := rows.Scan(&c.Plan, &c.Status, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Lifecycle counts lifecycle events between from and to.
func (r *PlatformRepository) Lifecycle(ctx context.Context, from, to time.Time) (*LifecycleCounts, error) {
	var c LifecycleCounts
	err := r.db.Pool.QueryRow(ctx, `SELECT
			COUNT(*) FILTER (WHERE created_at >= $1 AND created_at < $2),
			COUNT(*) FILTER (WHERE activated_at >= $1 AND activated_at < $2),
			COUNT(*) FILTER (WHERE cancelled_at >= $1 AND cancelled_at < $2),
			COUNT(*) FILTER (WHERE suspended_at >= $1 AND suspended_at < $2),
			COUNT(*) FILTER (WHERE COALESCE(trial_expires_at, trial_until) >= $1 AND COALESCE(trial_expires_at, trial_until) < $2),
			COUNT(*) FILTER (WHERE COALESCE(trial_expires_at, trial_until) >= $1 AND COALESCE(trial_expires_at, trial_until) < $2
				AND activated_at IS NOT NULL)
		FROM tenants`, from, to).
		Scan(&c.NewTenants, &c.Activated, &c.Cancelled, &c.Suspended, &c.TrialsEnded, &c.TrialsConverted)
	if err != nil {
		return nil, err
	}
	// Creditos de proration: negativo = debito de upgrade (expansao),
	// positivo = credito de downgrade (contracao).
	err = r.db.Pool.QueryRow(ctx, `SELECT
			COALESCE(SUM(-amount_cents) FILTER (WHERE reason = 'upgrade'), 0),
			COALESCE(SUM(amount_cents) FILTER (WHERE reason = 'downgrade'), 0)
		FROM billing_proration_credits WHERE created_at >= $1 AND created_at < $2`, from, to).
		Scan(&c.ExpansionCents, &c.ContractionCents)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// TenantOverview is one row of the super admin tenant table.
type TenantOverview struct {
	ID              domain.UUID `json:"id"`
	Name            string      `json:"name"`
	Document        string      `json:"document"`
	Plan            string      `json:"plan"`
	Status          string      `json:"status"`
	TrialExpiresAt  *time.Time  `json:"trial_expires_at"`
	ActivatedAt     *time.Time  `json:"activated_at"`
	CreatedAt       time.Time   `json:"created_at"`
	Users           int         `json:"users"`
	MaxUsers        int         `json:"max_users"`
	Transformers    int         `json:"transformers"`
	ConsumingUnits  int         `json:"consuming_units"`
	LastImportAt    *time.Time  `json:"last_import_at"`
	LastLoginAt     *time.Time  `json:"last_login_at"`
	HasSubscription bool        `json:"has_subscription"`
	CriticalTrafos  int         `json:"critical_transformers"`
	OwnerEmail      *string     `json:"owner_email"`
	SubscriptionID  *string     `json:"-"`
}

// ListOverview returns tenants with usage counters (filters as the tenant
// list endpoint). The total is the filtered count.
func (r *PlatformRepository) ListOverview(ctx context.Context, f domain.ListTenantsFilter) ([]*TenantOverview, int, error) {
	where := "TRUE"
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where += " AND " + strings.ReplaceAll(cond, "$?", "$"+strconv.Itoa(len(args)))
	}
	if f.Status != "" {
		add("t.status = $?", f.Status)
	}
	if f.Plan != "" {
		add("t.plan = $?", f.Plan)
	}
	if f.Search != "" {
		add("(t.name ILIKE $? OR t.document ILIKE $?)", "%"+f.Search+"%")
	}
	var total int
	if err := r.db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM tenants t WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit, offset := f.Limit, (f.Page-1)*f.Limit
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	q := `SELECT t.id, t.name, COALESCE(t.document, ''), t.plan, t.status, COALESCE(t.trial_expires_at, t.trial_until),
			t.activated_at, t.created_at, t.max_users, t.payment_subscription_id,
			(SELECT COUNT(*) FROM users u WHERE u.tenant_id = t.id AND u.deleted_at IS NULL),
			(SELECT COUNT(*) FROM transformers x WHERE x.tenant_id = t.id AND x.deleted_at IS NULL),
			(SELECT COUNT(*) FROM consuming_units c WHERE c.tenant_id = t.id AND c.deleted_at IS NULL),
			(SELECT MAX(i.created_at) FROM imports i WHERE i.tenant_id = t.id),
			(SELECT MAX(u.last_login) FROM users u WHERE u.tenant_id = t.id),
			(SELECT COUNT(DISTINCT b.transformer_id) FROM transformer_balance b
				WHERE b.tenant_id = t.id AND b.status = 'CRITICAL' AND b.period_end >= NOW() - INTERVAL '45 days'),
			(SELECT u.email FROM users u WHERE u.tenant_id = t.id AND u.role = 'OWNER' AND u.deleted_at IS NULL
				ORDER BY u.created_at LIMIT 1)
		FROM tenants t WHERE ` + where + `
		ORDER BY t.created_at DESC LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))
	rows, err := r.db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*TenantOverview
	for rows.Next() {
		var o TenantOverview
		if err := rows.Scan(&o.ID, &o.Name, &o.Document, &o.Plan, &o.Status, &o.TrialExpiresAt,
			&o.ActivatedAt, &o.CreatedAt, &o.MaxUsers, &o.SubscriptionID,
			&o.Users, &o.Transformers, &o.ConsumingUnits, &o.LastImportAt, &o.LastLoginAt,
			&o.CriticalTrafos, &o.OwnerEmail); err != nil {
			return nil, 0, err
		}
		o.HasSubscription = o.SubscriptionID != nil && *o.SubscriptionID != ""
		out = append(out, &o)
	}
	return out, total, rows.Err()
}

// ListPlatformAudit returns the latest super admin actions across tenants.
func (r *PlatformRepository) ListPlatformAudit(ctx context.Context, tenantID *domain.UUID, limit int) ([]*AuditEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Pool.Query(ctx, `SELECT id, table_name, operation, user_email, old_data, new_data, created_at
		FROM audit_log WHERE table_name = 'app' AND ($1::uuid IS NULL OR tenant_id = $1)
		ORDER BY created_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditRows(rows)
}
