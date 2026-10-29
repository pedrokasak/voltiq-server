package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/usecase"
)

// DashboardHandler handles dashboard HTTP requests. Every query runs with
// the request context so repositories keep the tenant scope.
type DashboardHandler struct {
	dashboard   *usecase.DashboardUseCase
	balanceRepo *repository.BalanceRepository
}

// TransformerCurrentStatus represents the current status of a transformer for quick dashboard
type TransformerCurrentStatus struct {
	TransformerID       string     `json:"transformer_id"`
	Code                string     `json:"code"`
	SubstationName      *string    `json:"substation_name"`
	Status              string     `json:"status"`
	LossPct             float64    `json:"loss_pct"`
	LossLimitPct        float64    `json:"loss_limit_pct"`
	EnergyInjectedKWh   float64    `json:"energy_injected_kwh"`
	TotalConsumptionKWh float64    `json:"total_consumption_kwh"`
	LossKWh             float64    `json:"loss_kwh"`
	UCCount             int        `json:"uc_count"`
	PeriodStart         *time.Time `json:"period_start"`
	PeriodEnd           *time.Time `json:"period_end"`
	CalculatedAt        *time.Time `json:"calculated_at"`
	LastReadingAt       *time.Time `json:"last_reading_at"`
	IsOverloaded        bool       `json:"is_overloaded"`
	OverloadPct         float64    `json:"overload_pct"`
	HasBalance          bool       `json:"has_balance"`
}

// DashboardKPIs contains dashboard key performance indicators
type DashboardKPIs struct {
	TotalTransformers    int     `json:"total_transformers"`
	TotalUCs             int     `json:"total_ucs"`
	TransformersNormal   int     `json:"transformers_normal"`
	TransformersWarning  int     `json:"transformers_warning"`
	TransformersCritical int     `json:"transformers_critical"`
	TransformersNoData   int     `json:"transformers_no_data"`
	AverageLossPct       float64 `json:"average_loss_pct"`
	WeightedLossPct      float64 `json:"weighted_loss_pct"`
	TotalEnergyKWh       float64 `json:"total_energy_kwh"`
	TotalBilledKWh       float64 `json:"total_billed_kwh"`
	TotalLossKWh         float64 `json:"total_loss_kwh"`
}

// NewDashboardHandler creates a new DashboardHandler
func NewDashboardHandler(dashboard *usecase.DashboardUseCase, balanceRepo *repository.BalanceRepository) *DashboardHandler {
	return &DashboardHandler{dashboard: dashboard, balanceRepo: balanceRepo}
}

func canViewFinancial(r *http.Request) bool {
	return middleware.CanViewFinancialData(domain.UserRole(middleware.GetUserRole(r.Context())))
}

func queryFloat(r *http.Request, name string) float64 {
	v, err := strconv.ParseFloat(strings.Replace(r.URL.Query().Get(name), ",", ".", 1), 64)
	if err != nil {
		return 0
	}
	return v
}

// GetKPIs returns dashboard KPIs computed from the latest balance of each
// transformer (single query).
func (h *DashboardHandler) GetKPIs(w http.ResponseWriter, r *http.Request) {
	op, err := h.dashboard.Map(r.Context(), middleware.GetTenantID(r.Context()), usecase.MapFilter{}, false)
	if err != nil {
		writeUseCaseError(w, err, "KPI_ERROR")
		return
	}
	summ, err := h.dashboard.Operational(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "KPI_ERROR")
		return
	}
	k := &DashboardKPIs{
		TotalTransformers:    summ.Network.Transformers,
		TotalUCs:             summ.Network.ConsumingUnits,
		TransformersNormal:   op.ByStatus["NORMAL"],
		TransformersWarning:  op.ByStatus["WARNING"],
		TransformersCritical: op.ByStatus["CRITICAL"],
		TransformersNoData:   op.ByStatus["NO_DATA"],
	}
	var sumPct float64
	var n int
	for _, p := range op.Transformers {
		if p.LossPct == nil {
			continue
		}
		sumPct += *p.LossPct
		n++
		k.TotalEnergyKWh += deref(p.EnergyInjectedKWh)
		k.TotalBilledKWh += deref(p.TotalConsumptionKWh)
		k.TotalLossKWh += deref(p.LossKWh)
	}
	if n > 0 {
		k.AverageLossPct = sumPct / float64(n)
	}
	if k.TotalEnergyKWh > 0 {
		k.WeightedLossPct = k.TotalLossKWh / k.TotalEnergyKWh * 100
	}
	request.WriteJSON(w, http.StatusOK, request.Success(k, ""))
}

func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

// GetTransformerCurrentStatus returns the current status of all transformers for quick dashboard
func (h *DashboardHandler) GetTransformerCurrentStatus(w http.ResponseWriter, r *http.Request) {
	m, err := h.dashboard.Map(r.Context(), middleware.GetTenantID(r.Context()), usecase.MapFilter{
		Status: strings.ToUpper(r.URL.Query().Get("status")),
	}, false)
	if err != nil {
		writeUseCaseError(w, err, "STATUS_ERROR")
		return
	}
	out := make([]TransformerCurrentStatus, 0, len(m.Transformers))
	for _, p := range m.Transformers {
		s := TransformerCurrentStatus{
			TransformerID: string(p.ID), Code: p.Code, SubstationName: p.SubstationName, Status: p.Status,
			LossPct: deref(p.LossPct), LossLimitPct: p.LossLimitPct, EnergyInjectedKWh: deref(p.EnergyInjectedKWh),
			TotalConsumptionKWh: deref(p.TotalConsumptionKWh), LossKWh: deref(p.LossKWh), UCCount: p.UCCount,
			PeriodStart: p.LastPeriodStart, PeriodEnd: p.LastPeriodEnd, LastReadingAt: p.LastReadingAt,
			HasBalance: p.LossPct != nil,
		}
		if p.LimitUsagePct != nil {
			s.OverloadPct = *p.LimitUsagePct
			s.IsOverloaded = deref(p.LossPct) >= p.LossLimitPct
		}
		out = append(out, s)
	}
	request.WriteJSON(w, http.StatusOK, request.Success(out, ""))
}

// GetMonthlyLossHistory returns the aggregated loss of the last 12 months
func (h *DashboardHandler) GetMonthlyLossHistory(w http.ResponseWriter, r *http.Request) {
	aggs, err := h.balanceRepo.GetMonthlyAggregatesByTenant(r.Context(), middleware.GetTenantID(r.Context()), 12)
	if err != nil {
		writeUseCaseError(w, err, "HISTORY_ERROR")
		return
	}
	var injected, billed, loss float64
	count := 0
	months := make([]map[string]any, 0, len(aggs))
	for _, a := range aggs {
		injected += a.TotalEnergyInjected
		billed += a.TotalConsumption
		loss += a.TotalLoss
		count += a.BalanceCount
		lossPct := 0.0
		if a.TotalEnergyInjected > 0 {
			lossPct = a.TotalLoss / a.TotalEnergyInjected * 100
		}
		months = append(months, map[string]any{
			"month": a.Month.Format("2006-01"), "energy_injected_kwh": a.TotalEnergyInjected,
			"total_consumption_kwh": a.TotalConsumption, "loss_kwh": a.TotalLoss, "loss_pct": lossPct,
		})
	}
	avg := 0.0
	if injected > 0 {
		avg = loss / injected * 100
	}
	request.WriteJSON(w, http.StatusOK, request.Success(map[string]any{
		"average_loss_pct": avg,
		"total_energy_kwh": injected,
		"total_billed_kwh": billed,
		"total_loss_kwh":   loss,
		"balance_count":    count,
		"months":           months,
	}, ""))
}

// GetKPISparklines returns sparkline data for each KPI card. Valores em R$
// sao omitidos para papeis sem financial:view.
func (h *DashboardHandler) GetKPISparklines(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := middleware.GetTenantID(ctx)
	aggs, err := h.balanceRepo.GetMonthlyAggregatesByTenant(ctx, tenantID, 12)
	if err != nil {
		writeUseCaseError(w, err, "SPARKLINE_ERROR")
		return
	}
	op, err := h.dashboard.Operational(ctx, tenantID)
	if err != nil {
		writeUseCaseError(w, err, "SPARKLINE_ERROR")
		return
	}
	limit := 10.0
	if op.LatestMonth != nil {
		limit = op.LatestMonth.LimitPct
	}
	tariff, _ := h.dashboard.Tariff(ctx, tenantID, 0)
	financial := canViewFinancial(r)

	energy := make([]SparklinePoint, 0, len(aggs))
	losses := make([]SparklinePointWithLimit, 0, len(aggs))
	cost := make([]SparklinePoint, 0, len(aggs))
	revenue := make([]SparklinePoint, 0, len(aggs))
	savings := make([]SparklinePoint, 0, len(aggs))
	for _, a := range aggs {
		m := a.Month.Format("2006-01")
		energy = append(energy, SparklinePoint{Date: m, Value: a.TotalEnergyInjected})
		lossPct := 0.0
		if a.TotalEnergyInjected > 0 {
			lossPct = a.TotalLoss / a.TotalEnergyInjected * 100
		}
		losses = append(losses, SparklinePointWithLimit{Date: m, Value: lossPct, Limit: limit})
		if financial {
			excess := a.TotalLoss - a.TotalEnergyInjected*limit/100
			if excess < 0 {
				excess = 0
			}
			cost = append(cost, SparklinePoint{Date: m, Value: a.TotalLoss / 1000 * tariff})
			revenue = append(revenue, SparklinePoint{Date: m, Value: a.TotalConsumption / 1000 * tariff})
			savings = append(savings, SparklinePoint{Date: m, Value: excess / 1000 * tariff})
		}
	}
	resp := map[string]any{"energy_injected": energy, "total_loss": losses}
	if financial {
		resp["loss_cost"], resp["billed_revenue"], resp["potential_savings"] = cost, revenue, savings
		resp["tariff_brl_mwh"] = tariff
	}
	request.WriteJSON(w, http.StatusOK, request.Success(resp, ""))
}

// GetLossEvolution returns monthly loss evolution with ANEEL limit
func (h *DashboardHandler) GetLossEvolution(w http.ResponseWriter, r *http.Request) {
	op, err := h.dashboard.Operational(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "EVOLUTION_ERROR")
		return
	}
	type EvolutionPoint struct {
		Month           string  `json:"month"`
		TotalLossPct    float64 `json:"total_loss_pct"`
		TechnicalPct    float64 `json:"technical_pct"`
		NonTechnicalPct float64 `json:"non_technical_pct"`
		AneelLimit      float64 `json:"aneel_limit"`
		InjectedMWh     float64 `json:"injected_mwh"`
		BilledMWh       float64 `json:"billed_mwh"`
	}
	points := make([]EvolutionPoint, 0, len(op.Evolution))
	for _, p := range op.Evolution {
		points = append(points, EvolutionPoint{
			Month: p.Month, TotalLossPct: p.LossPct, TechnicalPct: p.TechnicalPct,
			NonTechnicalPct: p.NonTechnicalPct, AneelLimit: p.LimitPct, InjectedMWh: p.InjectedMWh, BilledMWh: p.BilledMWh,
		})
	}
	request.WriteJSON(w, http.StatusOK, request.Success(points, ""))
}

// GetLossComposition returns technical vs non-technical loss donut data
func (h *DashboardHandler) GetLossComposition(w http.ResponseWriter, r *http.Request) {
	comp, err := h.balanceRepo.GetLossCompositionByTenant(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "COMPOSITION_ERROR")
		return
	}
	total := comp.TotalTechnicalLoss + comp.TotalNonTechnicalLoss
	techPct, nonTechPct := 0.0, 0.0
	if total > 0 {
		techPct = comp.TotalTechnicalLoss / total * 100
		nonTechPct = comp.TotalNonTechnicalLoss / total * 100
	}
	request.WriteJSON(w, http.StatusOK, request.Success(map[string]any{
		"technical":         comp.TotalTechnicalLoss,
		"non_technical":     comp.TotalNonTechnicalLoss,
		"technical_pct":     techPct,
		"non_technical_pct": nonTechPct,
	}, ""))
}

// GetTransformerStatusDistribution returns status donut data
func (h *DashboardHandler) GetTransformerStatusDistribution(w http.ResponseWriter, r *http.Request) {
	m, err := h.dashboard.Map(r.Context(), middleware.GetTenantID(r.Context()), usecase.MapFilter{}, false)
	if err != nil {
		writeUseCaseError(w, err, "STATUS_DIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(map[string]any{
		"normal":   m.ByStatus["NORMAL"],
		"warning":  m.ByStatus["WARNING"],
		"critical": m.ByStatus["CRITICAL"],
		"no_data":  m.ByStatus["NO_DATA"],
		"total":    m.Total,
	}, ""))
}

// GetQuarterlyInjectedBilled returns quarterly injected vs billed bars
func (h *DashboardHandler) GetQuarterlyInjectedBilled(w http.ResponseWriter, r *http.Request) {
	aggs, err := h.balanceRepo.GetQuarterlyAggregatesByTenant(r.Context(), middleware.GetTenantID(r.Context()), 4)
	if err != nil {
		writeUseCaseError(w, err, "QUARTERLY_ERROR")
		return
	}
	type QuarterlyPoint struct {
		Quarter     string  `json:"quarter"`
		InjectedMWh float64 `json:"injected_mwh"`
		BilledMWh   float64 `json:"billed_mwh"`
	}
	points := make([]QuarterlyPoint, 0, len(aggs))
	for _, a := range aggs {
		q := (int(a.Quarter.Month())-1)/3 + 1
		points = append(points, QuarterlyPoint{
			Quarter:     strconv.Itoa(a.Quarter.Year()) + "-T" + strconv.Itoa(q),
			InjectedMWh: a.TotalEnergyInjected / 1000,
			BilledMWh:   a.TotalConsumption / 1000,
		})
	}
	request.WriteJSON(w, http.StatusOK, request.Success(points, ""))
}

// GetAccumulatedLossCost returns accumulated loss cost area chart
// (tarifa do tenant; ?tariff= sobrescreve para simulacao).
func (h *DashboardHandler) GetAccumulatedLossCost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := middleware.GetTenantID(ctx)
	tariff, _ := h.dashboard.Tariff(ctx, tenantID, queryFloat(r, "tariff"))
	points, err := h.balanceRepo.GetAccumulatedLossCost(ctx, tenantID, tariff, 12)
	if err != nil {
		writeUseCaseError(w, err, "COST_ERROR")
		return
	}
	type CostPoint struct {
		Month              string  `json:"month"`
		MonthlyLossKWh     float64 `json:"monthly_loss_kwh"`
		AccumulatedLossKWh float64 `json:"accumulated_loss_kwh"`
		MonthlyCostBRL     float64 `json:"monthly_cost_brl"`
		AccumulatedCostBRL float64 `json:"accumulated_cost_brl"`
	}
	resp := make([]CostPoint, 0, len(points))
	for _, p := range points {
		resp = append(resp, CostPoint{
			Month: p.Month.Format("2006-01"), MonthlyLossKWh: p.MonthlyLossKWh, AccumulatedLossKWh: p.AccumulatedLossKWh,
			MonthlyCostBRL: p.MonthlyCostBRL, AccumulatedCostBRL: p.AccumulatedCostBRL,
		})
	}
	request.WriteJSON(w, http.StatusOK, request.Success(resp, ""))
}

// GetTopTransformersByLoss returns top N transformers by loss (max 50)
func (h *DashboardHandler) GetTopTransformersByLoss(w http.ResponseWriter, r *http.Request) {
	limit := min(max(queryInt(r, "limit", 5), 1), 50)
	items, err := h.balanceRepo.GetTopTransformersByLoss(r.Context(), middleware.GetTenantID(r.Context()), limit)
	if err != nil {
		writeUseCaseError(w, err, "TOP_ERROR")
		return
	}
	type TopTransformer struct {
		TransformerID       string  `json:"transformer_id"`
		Code                string  `json:"code"`
		LossPct             float64 `json:"loss_pct"`
		LossKWh             float64 `json:"loss_kwh"`
		EnergyInjectedKWh   float64 `json:"energy_injected_kwh"`
		TechnicalLossKWh    float64 `json:"technical_loss_kwh"`
		NonTechnicalLossKWh float64 `json:"non_technical_loss_kwh"`
	}
	resp := make([]TopTransformer, 0, len(items))
	for _, t := range items {
		resp = append(resp, TopTransformer{
			TransformerID: string(t.TransformerID), Code: t.Code, LossPct: t.LossPct, LossKWh: t.LossKWh,
			EnergyInjectedKWh: t.EnergyInjectedKWh, TechnicalLossKWh: t.TechnicalLossKWh, NonTechnicalLossKWh: t.NonTechnicalLossKWh,
		})
	}
	request.WriteJSON(w, http.StatusOK, request.Success(resp, ""))
}

// GetFinancialTable returns the financial table of the latest balance of
// each transformer (tarifa do tenant; ?tariff= sobrescreve).
func (h *DashboardHandler) GetFinancialTable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := middleware.GetTenantID(ctx)
	tariff, _ := h.dashboard.Tariff(ctx, tenantID, queryFloat(r, "tariff"))
	m, err := h.dashboard.Map(ctx, tenantID, usecase.MapFilter{}, false)
	if err != nil {
		writeUseCaseError(w, err, "FINANCIAL_ERROR")
		return
	}
	type FinancialRow struct {
		TransformerID       string  `json:"transformer_id"`
		Code                string  `json:"code"`
		LossPct             float64 `json:"loss_pct"`
		LossKWh             float64 `json:"loss_kwh"`
		LossCostBRL         float64 `json:"loss_cost_brl"`
		EnergyInjectedKWh   float64 `json:"energy_injected_kwh"`
		EnergyBilledKWh     float64 `json:"energy_billed_kwh"`
		RevenueBRL          float64 `json:"revenue_brl"`
		PotentialSavingsBRL float64 `json:"potential_savings_brl"`
	}
	rows := make([]FinancialRow, 0, len(m.Transformers))
	for _, p := range m.Transformers {
		if p.LossPct == nil {
			continue
		}
		loss, injected := deref(p.LossKWh), deref(p.EnergyInjectedKWh)
		excess := loss - injected*p.LossLimitPct/100
		if excess < 0 {
			excess = 0
		}
		rows = append(rows, FinancialRow{
			TransformerID: string(p.ID), Code: p.Code, LossPct: *p.LossPct, LossKWh: loss,
			LossCostBRL: loss / 1000 * tariff, EnergyInjectedKWh: injected, EnergyBilledKWh: deref(p.TotalConsumptionKWh),
			RevenueBRL: deref(p.TotalConsumptionKWh) / 1000 * tariff, PotentialSavingsBRL: excess / 1000 * tariff,
		})
	}
	request.WriteJSON(w, http.StatusOK, request.Success(rows, ""))
}

// GetMap returns the georeferenced transformers, substations, bounds and
// status counters (GET /dashboard/map?status=&substation_id=&group_id=&search=).
func (h *DashboardHandler) GetMap(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := usecase.MapFilter{Status: strings.ToUpper(q.Get("status")), Search: strings.TrimSpace(q.Get("search"))}
	if v := q.Get("substation_id"); isUUID(v) {
		f.SubstationID = v
	}
	if v := q.Get("group_id"); isUUID(v) {
		f.GroupID = v
	}
	m, err := h.dashboard.Map(r.Context(), middleware.GetTenantID(r.Context()), f, canViewFinancial(r))
	if err != nil {
		writeUseCaseError(w, err, "MAP_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(m, ""))
}

// GetOperational returns the operational dashboard (GET /dashboard/operational)
func (h *DashboardHandler) GetOperational(w http.ResponseWriter, r *http.Request) {
	d, err := h.dashboard.Operational(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "OPERATIONAL_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(d, ""))
}

// GetManagerial returns the managerial dashboard (GET /dashboard/managerial?tariff=)
func (h *DashboardHandler) GetManagerial(w http.ResponseWriter, r *http.Request) {
	d, err := h.dashboard.Managerial(r.Context(), middleware.GetTenantID(r.Context()), queryFloat(r, "tariff"))
	if err != nil {
		writeUseCaseError(w, err, "MANAGERIAL_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(d, ""))
}

// SparklinePoint represents a simple sparkline data point
type SparklinePoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// SparklinePointWithLimit represents a sparkline point with limit line
type SparklinePointWithLimit struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
	Limit float64 `json:"limit"`
}
