package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/payment"
	"github.com/voltiq/server/internal/repository"
)

// SuperAdminUseCase handles cross-tenant administrative operations.
// Access is gated by the SUPER_ADMIN role at the router level.
type SuperAdminUseCase struct {
	tenantRepo   *repository.TenantRepository
	userRepo     *repository.UserRepository
	platformRepo *repository.PlatformRepository
	accountRepo  *repository.AccountRepository
	metricsRepo  *repository.MetricsRepository
	billing      *BillingUseCase
}

// NewSuperAdminUseCase creates a new SuperAdminUseCase
func NewSuperAdminUseCase(
	tenantRepo *repository.TenantRepository,
	userRepo *repository.UserRepository,
	platformRepo *repository.PlatformRepository,
	accountRepo *repository.AccountRepository,
	metricsRepo *repository.MetricsRepository,
	billing *BillingUseCase,
) *SuperAdminUseCase {
	return &SuperAdminUseCase{
		tenantRepo:   tenantRepo,
		userRepo:     userRepo,
		platformRepo: platformRepo,
		accountRepo:  accountRepo,
		metricsRepo:  metricsRepo,
		billing:      billing,
	}
}

// ListTenantsFilter holds filtering and pagination options for listing tenants
type ListTenantsFilter = domain.ListTenantsFilter

// TenantListResult holds the paginated result
type TenantListResult = domain.TenantListResult

func normalizeFilter(filter ListTenantsFilter) ListTenantsFilter {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	return filter
}

// ListTenants lists tenants with filtering and pagination
func (uc *SuperAdminUseCase) ListTenants(ctx context.Context, filter ListTenantsFilter) (*TenantListResult, error) {
	filter = normalizeFilter(filter)
	tenants, total, err := uc.tenantRepo.ListWithFilters(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &TenantListResult{
		Data:       tenants,
		Total:      total,
		Page:       filter.Page,
		Limit:      filter.Limit,
		TotalPages: (total + filter.Limit - 1) / filter.Limit,
	}, nil
}

// TenantOverviewPage is the paginated tenant table of the admin panel.
type TenantOverviewPage struct {
	Data       []*repository.TenantOverview `json:"data"`
	Total      int                          `json:"total"`
	Page       int                          `json:"page"`
	Limit      int                          `json:"limit"`
	TotalPages int                          `json:"total_pages"`
}

// ListOverview lists tenants with usage counters.
func (uc *SuperAdminUseCase) ListOverview(ctx context.Context, filter ListTenantsFilter) (*TenantOverviewPage, error) {
	filter = normalizeFilter(filter)
	items, total, err := uc.platformRepo.ListOverview(ctx, filter)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []*repository.TenantOverview{}
	}
	return &TenantOverviewPage{Data: items, Total: total, Page: filter.Page, Limit: filter.Limit,
		TotalPages: (total + filter.Limit - 1) / filter.Limit}, nil
}

// GetTenantByID retrieves a single tenant by ID across the platform
func (uc *SuperAdminUseCase) GetTenantByID(ctx context.Context, id domain.UUID) (*domain.Tenant, error) {
	t, err := uc.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTenantNotFound
	}
	return t, nil
}

// TenantDetail is the full view of a tenant for subscription management.
type TenantDetail struct {
	Tenant      *domain.Tenant             `json:"tenant"`
	Users       []*domain.User             `json:"users"`
	Usage       *repository.Usage          `json:"usage"`
	Limits      domain.PlanLimits          `json:"limits"`
	Payments    []*payment.Payment         `json:"payments"`
	PaymentsErr string                     `json:"payments_error,omitempty"`
	Dunning     []*repository.DunningEvent `json:"dunning"`
	Audit       []*repository.AuditEntry   `json:"audit"`
	MRRCents    int64                      `json:"mrr_cents"`
	TrialDays   int                        `json:"trial_days_left"`
}

// GetTenantDetail returns tenant, users, usage, payments and admin actions.
func (uc *SuperAdminUseCase) GetTenantDetail(ctx context.Context, id domain.UUID) (*TenantDetail, error) {
	t, err := uc.GetTenantByID(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &TenantDetail{Tenant: t, Limits: domain.LimitsForPlan(t.Plan), Payments: []*payment.Payment{}}
	if d.Users, err = uc.userRepo.GetByTenant(ctx, id); err != nil {
		return nil, err
	}
	if d.Usage, err = uc.accountRepo.GetUsage(ctx, id); err != nil {
		return nil, err
	}
	if uc.billing != nil {
		// Falha do gateway nao impede a tela de administracao
		if p, err := uc.billing.GetPaymentHistory(ctx, id, 20, 0); err != nil {
			slog.Warn("admin tenant payments unavailable", "tenant_id", id, "error", err)
			d.PaymentsErr = "não foi possível consultar o gateway de pagamento"
		} else if p != nil {
			d.Payments = p
		}
		if d.Dunning, err = uc.billing.GetDunningHistory(ctx, id); err != nil {
			return nil, err
		}
	}
	if d.Audit, err = uc.platformRepo.ListPlatformAudit(ctx, &id, 30); err != nil {
		return nil, err
	}
	if t.Status == domain.TenantStatusActive {
		d.MRRCents = repository.PlanPriceCents(string(t.Plan))
	}
	if t.Status == domain.TenantStatusTrial {
		end := t.TrialUntil
		if t.TrialExpiresAt != nil {
			end = *t.TrialExpiresAt
		}
		d.TrialDays = max(0, int(math.Ceil(time.Until(end).Hours()/24)))
	}
	return d, nil
}

// ListTenantUsers lists all users belonging to a tenant
func (uc *SuperAdminUseCase) ListTenantUsers(ctx context.Context, tenantID domain.UUID) ([]*domain.User, error) {
	if _, err := uc.GetTenantByID(ctx, tenantID); err != nil {
		return nil, err
	}
	return uc.userRepo.GetByTenant(ctx, tenantID)
}

// audit records a super admin action; failures are logged, not returned,
// because the action itself already succeeded.
func (uc *SuperAdminUseCase) audit(ctx context.Context, tenantID domain.UUID, actor, action string, data map[string]any) {
	if uc.accountRepo == nil {
		return
	}
	if err := uc.accountRepo.LogAction(ctx, &tenantID, actor, action, data); err != nil {
		slog.Error("admin audit failed", "tenant_id", tenantID, "action", action, "error", err)
	}
}

// ActivateTenant activates a tenant (TRIAL -> ACTIVE or SUSPENDED -> ACTIVE)
func (uc *SuperAdminUseCase) ActivateTenant(ctx context.Context, id domain.UUID, plan domain.TenantPlan, actor string) (*domain.Tenant, error) {
	tenant, err := uc.GetTenantByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tenant.Status == domain.TenantStatusActive {
		return nil, ErrTenantAlreadyActive
	}
	before := tenant.Status
	if plan != "" {
		if !validPaidPlan(plan) {
			return nil, ErrInvalidPlan
		}
		tenant.Plan = plan
		tenant.MaxUsers = getMaxUsersForPlan(plan)
	}
	if tenant.Plan == domain.TenantPlanTrial {
		return nil, ErrInvalidPlan
	}
	now := time.Now()
	tenant.Status = domain.TenantStatusActive
	tenant.Active = true
	tenant.ActivatedAt = &now
	tenant.SuspendedAt = nil
	tenant.UpdatedAt = now
	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}
	uc.audit(ctx, id, actor, "TENANT_ACTIVATED", map[string]any{"from_status": before, "plan": tenant.Plan})
	return tenant, nil
}

func validPaidPlan(p domain.TenantPlan) bool {
	return p == domain.TenantPlanStarter || p == domain.TenantPlanPro || p == domain.TenantPlanEnterprise
}

// UpdateTenantPlan updates the tenant's plan
func (uc *SuperAdminUseCase) UpdateTenantPlan(ctx context.Context, id domain.UUID, plan domain.TenantPlan, actor string) (*domain.Tenant, error) {
	if !validPaidPlan(plan) && plan != domain.TenantPlanTrial {
		return nil, ErrInvalidPlan
	}
	tenant, err := uc.GetTenantByID(ctx, id)
	if err != nil {
		return nil, err
	}
	newMaxUsers := getMaxUsersForPlan(plan)
	if tenant.SeatCount > newMaxUsers {
		return nil, ErrSeatLimitExceeded
	}
	before := tenant.Plan
	tenant.Plan = plan
	tenant.MaxUsers = newMaxUsers
	tenant.UpdatedAt = time.Now()
	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}
	uc.audit(ctx, id, actor, "TENANT_PLAN_CHANGED", map[string]any{"from": before, "to": plan})
	return tenant, nil
}

// validStatuses are the statuses a super admin may set manually.
var validStatuses = map[domain.TenantStatus]bool{
	domain.TenantStatusTrial: true, domain.TenantStatusActive: true, domain.TenantStatusSuspended: true,
	domain.TenantStatusPendingPayment: true, domain.TenantStatusCancelled: true,
}

// UpdateTenantStatus updates the tenant's status manually, recording the reason
func (uc *SuperAdminUseCase) UpdateTenantStatus(ctx context.Context, id domain.UUID, status string, reason string, actor string) (*domain.Tenant, error) {
	newStatus := domain.TenantStatus(status)
	if !validStatuses[newStatus] {
		return nil, badRequest("status inválido")
	}
	tenant, err := uc.GetTenantByID(ctx, id)
	if err != nil {
		return nil, err
	}
	before := tenant.Status
	now := time.Now()
	tenant.Status = newStatus
	tenant.UpdatedAt = now
	switch newStatus {
	case domain.TenantStatusSuspended:
		tenant.SuspendedAt = &now
	case domain.TenantStatusCancelled:
		tenant.CancelledAt = &now
		tenant.Active = false
	case domain.TenantStatusActive:
		tenant.Active = true
		tenant.SuspendedAt = nil
		if tenant.ActivatedAt == nil {
			tenant.ActivatedAt = &now
		}
	}
	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}
	uc.audit(ctx, id, actor, "TENANT_STATUS_CHANGED", map[string]any{"from": before, "to": newStatus, "reason": reason})
	return tenant, nil
}

// ExtendTrial adds days to the tenant trial (1-90) and reopens it when the
// trial had already expired.
func (uc *SuperAdminUseCase) ExtendTrial(ctx context.Context, id domain.UUID, days int, reason, actor string) (*domain.Tenant, error) {
	if days < 1 || days > 90 {
		return nil, badRequest("a extensão deve ser de 1 a 90 dias")
	}
	tenant, err := uc.GetTenantByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if tenant.Status == domain.TenantStatusActive {
		return nil, badRequest("tenant já possui assinatura ativa")
	}
	base := tenant.TrialUntil
	if tenant.TrialExpiresAt != nil {
		base = *tenant.TrialExpiresAt
	}
	if base.Before(time.Now()) {
		base = time.Now()
	}
	end := base.AddDate(0, 0, days)
	tenant.TrialUntil = end
	tenant.TrialExpiresAt = &end
	tenant.Status = domain.TenantStatusTrial
	tenant.Active = true
	tenant.SuspendedAt = nil
	tenant.UpdatedAt = time.Now()
	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}
	uc.audit(ctx, id, actor, "TENANT_TRIAL_EXTENDED", map[string]any{"days": days, "until": end, "reason": reason})
	return tenant, nil
}

// PlanMRR is the subscription count and MRR (R$) of one plan.
type PlanMRR struct {
	Count int     `json:"count"`
	MRR   float64 `json:"mrr"`
}

// MRRPoint is one point of the MRR series.
type MRRPoint struct {
	Date string  `json:"date"`
	MRR  float64 `json:"mrr"`
}

// SaaSMetrics are the platform metrics of the admin billing dashboard.
// Valores monetarios em R$; taxas em fracao (0.05 = 5%).
type SaaSMetrics struct {
	PeriodDays          int                         `json:"period_days"`
	MRR                 float64                     `json:"mrr"`
	ARR                 float64                     `json:"arr"`
	ARPA                float64                     `json:"arpa"`
	ChurnRate           float64                     `json:"churn_rate"`
	LTV                 float64                     `json:"ltv"`
	TrialConversion     float64                     `json:"trial_conversion"`
	ExpansionRevenue    float64                     `json:"expansion_revenue"`
	ContractionRevenue  float64                     `json:"contraction_revenue"`
	NetRevenueRetention float64                     `json:"net_revenue_retention"`
	ActiveSubscriptions int                         `json:"active_subscriptions"`
	TrialTenants        int                         `json:"trial_tenants"`
	SuspendedTenants    int                         `json:"suspended_tenants"`
	PendingPayment      int                         `json:"pending_payment_tenants"`
	CancelledTenants    int                         `json:"cancelled_tenants"`
	TotalTenants        int                         `json:"total_tenants"`
	Lifecycle           *repository.LifecycleCounts `json:"lifecycle"`
	ByPlan              map[string]PlanMRR          `json:"by_plan"`
	ByStatus            map[string]int              `json:"by_status"`
	Series              []MRRPoint                  `json:"series"`
}

// ParsePeriodDays converts "30d", "90d", "12m" or "1y" into days (default 30).
func ParsePeriodDays(p string) int {
	var n int
	var unit string
	if _, err := fmt.Sscanf(p, "%d%s", &n, &unit); err != nil || n <= 0 {
		return 30
	}
	switch unit {
	case "m":
		n *= 30
	case "y":
		n *= 365
	}
	return min(n, 730)
}

// SaaSMetrics computes the platform metrics for the last periodDays.
func (uc *SuperAdminUseCase) SaaSMetrics(ctx context.Context, periodDays int) (*SaaSMetrics, error) {
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -periodDays)
	counts, err := uc.platformRepo.CountByPlanStatus(ctx)
	if err != nil {
		return nil, err
	}
	m := &SaaSMetrics{
		PeriodDays: periodDays,
		ByPlan:     map[string]PlanMRR{"starter": {}, "pro": {}, "enterprise": {}},
		ByStatus:   map[string]int{},
		Series:     []MRRPoint{},
	}
	for _, c := range counts {
		m.TotalTenants += c.Count
		m.ByStatus[c.Status] += c.Count
		switch domain.TenantStatus(c.Status) {
		case domain.TenantStatusActive:
			price := float64(repository.PlanPriceCents(c.Plan)) / 100
			p := m.ByPlan[c.Plan]
			p.Count += c.Count
			p.MRR += price * float64(c.Count)
			m.ByPlan[c.Plan] = p
			m.MRR += price * float64(c.Count)
			m.ActiveSubscriptions += c.Count
		case domain.TenantStatusTrial:
			m.TrialTenants += c.Count
		case domain.TenantStatusSuspended:
			m.SuspendedTenants += c.Count
		case domain.TenantStatusPendingPayment:
			m.PendingPayment += c.Count
		case domain.TenantStatusCancelled:
			m.CancelledTenants += c.Count
		}
	}
	m.ARR = m.MRR * 12
	if m.ActiveSubscriptions > 0 {
		m.ARPA = m.MRR / float64(m.ActiveSubscriptions)
	}

	lc, err := uc.platformRepo.Lifecycle(ctx, from, now)
	if err != nil {
		return nil, err
	}
	m.Lifecycle = lc
	m.ExpansionRevenue = float64(lc.ExpansionCents) / 100
	m.ContractionRevenue = float64(lc.ContractionCents) / 100

	// Churn mensal: cancelamentos do periodo sobre a base no inicio do
	// periodo (ativos de hoje + cancelados - ativados no periodo).
	base := m.ActiveSubscriptions + lc.Cancelled - lc.Activated
	if base > 0 {
		months := math.Max(float64(periodDays)/30, 1)
		m.ChurnRate = round4(float64(lc.Cancelled) / float64(base) / months)
	}
	if m.ChurnRate > 0 {
		m.LTV = round2(m.ARPA / m.ChurnRate)
	} else {
		// Sem churn observado o LTV e limitado a 36 meses de receita
		m.LTV = round2(m.ARPA * 36)
	}
	if lc.TrialsEnded > 0 {
		m.TrialConversion = round4(float64(lc.TrialsConverted) / float64(lc.TrialsEnded))
	}

	if uc.metricsRepo != nil {
		daily, err := uc.metricsRepo.GetMetricsRange(ctx, from, now)
		if err != nil {
			return nil, err
		}
		for _, d := range daily {
			m.Series = append(m.Series, MRRPoint{Date: d.Date.Format("2006-01-02"), MRR: float64(d.MRRCents) / 100})
		}
	}
	// NRR: (MRR inicial + expansao - contracao - MRR perdido) / MRR inicial.
	startMRR := m.MRR
	if len(m.Series) > 0 {
		startMRR = m.Series[0].MRR
	}
	if startMRR > 0 {
		lost := float64(lc.Cancelled) * m.ARPA
		m.NetRevenueRetention = round4((startMRR + m.ExpansionRevenue - m.ContractionRevenue - lost) / startMRR)
	} else {
		m.NetRevenueRetention = 1
	}
	m.Series = append(m.Series, MRRPoint{Date: now.Format("2006-01-02"), MRR: m.MRR})
	return m, nil
}

// PlatformAudit lists the latest super admin actions.
func (uc *SuperAdminUseCase) PlatformAudit(ctx context.Context, limit int) ([]*repository.AuditEntry, error) {
	return uc.platformRepo.ListPlatformAudit(ctx, nil, limit)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// getMaxUsersForPlan returns the max_users for a given plan
// (0 em PlanLimits significa ilimitado; a coluna max_users usa 999999).
func getMaxUsersForPlan(plan domain.TenantPlan) int {
	if n := domain.LimitsForPlan(plan).MaxUsers; n > 0 {
		return n
	}
	return 999999
}
