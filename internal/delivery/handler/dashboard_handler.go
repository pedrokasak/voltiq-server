package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// DashboardHandler handles dashboard HTTP requests
type DashboardHandler struct {
	transformerRepo *repository.TransformerRepository
	balanceRepo     *repository.BalanceRepository
	ucRepo          *repository.ConsumingUnitRepository
}

// TransformerCurrentStatus represents the current status of a transformer for quick dashboard
type TransformerCurrentStatus struct {
	TransformerID   string             `json:"transformer_id"`
	Code            string             `json:"code"`
	Status          domain.BalanceStatus `json:"status"`
	LossPct         float64            `json:"loss_pct"`
	LossLimitPct    float64            `json:"loss_limit_pct"`
	EnergyInjectedKWh float64          `json:"energy_injected_kwh"`
	TotalConsumptionKWh float64        `json:"total_consumption_kwh"`
	LossKWh         float64            `json:"loss_kwh"`
	UCCount         int                `json:"uc_count"`
	PeriodStart     time.Time          `json:"period_start"`
	PeriodEnd       time.Time          `json:"period_end"`
	CalculatedAt    time.Time          `json:"calculated_at"`
	IsOverloaded    bool               `json:"is_overloaded"`
	OverloadPct     float64            `json:"overload_pct"`
}

// DashboardKPIs contains dashboard key performance indicators
type DashboardKPIs struct {
	TotalTransformers    int     `json:"total_transformers"`
	TotalUCs             int     `json:"total_ucs"`
	TransformersNormal   int     `json:"transformers_normal"`
	TransformersWarning  int     `json:"transformers_warning"`
	TransformersCritical int     `json:"transformers_critical"`
	AverageLossPct       float64 `json:"average_loss_pct"`
	TotalEnergyKWh       float64 `json:"total_energy_kwh"`
	TotalBilledKWh       float64 `json:"total_billed_kwh"`
}

// NewDashboardHandler creates a new DashboardHandler
func NewDashboardHandler(
	transformerRepo *repository.TransformerRepository,
	balanceRepo *repository.BalanceRepository,
	ucRepo *repository.ConsumingUnitRepository,
) *DashboardHandler {
	return &DashboardHandler{
		transformerRepo: transformerRepo,
		balanceRepo:     balanceRepo,
		ucRepo:          ucRepo,
	}
}

// GetKPIs returns dashboard KPIs
func (h *DashboardHandler) GetKPIs(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	ctx := context.Background()

	// Get all transformers for tenant
	transformers, err := h.transformerRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", "failed to get transformers", nil))
		return
	}

	// Get all UCs for tenant
	ucs, err := h.ucRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", "failed to get consuming units", nil))
		return
	}

	// Calculate status for each transformer based on latest balance
	transformersNormal := 0
	transformersWarning := 0
	transformersCritical := 0
	totalLossPct := 0.0
	balanceCount := 0
	totalEnergyKWh := 0.0
	totalBilledKWh := 0.0

	for _, t := range transformers {
		latestBalance, err := h.balanceRepo.GetLatestByTransformer(ctx, t.ID)
		if err != nil {
			continue
		}

		if latestBalance != nil {
			switch latestBalance.Status {
			case domain.BalanceStatusNormal:
				transformersNormal++
			case domain.BalanceStatusWarning:
				transformersWarning++
			case domain.BalanceStatusCritical:
				transformersCritical++
			}

			totalLossPct += latestBalance.LossPct
			balanceCount++
			totalEnergyKWh += latestBalance.EnergyInjectedKWh
			totalBilledKWh += latestBalance.TotalConsumptionKWh
		}
	}

	averageLossPct := 0.0
	if balanceCount > 0 {
		averageLossPct = totalLossPct / float64(balanceCount)
	}

	kpis := &DashboardKPIs{
		TotalTransformers:    len(transformers),
		TotalUCs:             len(ucs),
		TransformersNormal:   transformersNormal,
		TransformersWarning:  transformersWarning,
		TransformersCritical: transformersCritical,
		AverageLossPct:       averageLossPct,
		TotalEnergyKWh:       totalEnergyKWh,
		TotalBilledKWh:       totalBilledKWh,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(kpis, ""))
}

// GetTransformerCurrentStatus returns the current status of all transformers for quick dashboard
func (h *DashboardHandler) GetTransformerCurrentStatus(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	ctx := context.Background()

	// Get all transformers for tenant
	transformers, err := h.transformerRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", "failed to get transformers", nil))
		return
	}

	var statuses []TransformerCurrentStatus

	for _, t := range transformers {
		latestBalance, err := h.balanceRepo.GetLatestByTransformer(ctx, t.ID)
		if err != nil {
			continue
		}

		if latestBalance == nil {
			// No balance calculated yet
			lossLimit := 10.0
			if t.LossLimitPct != nil {
				lossLimit = *t.LossLimitPct
			}
			status := TransformerCurrentStatus{
				TransformerID: string(t.ID),
				Code:          t.Code,
				Status:        domain.BalanceStatusNormal,
				LossPct:       0,
				LossLimitPct:  lossLimit,
				UCCount:       0,
			}
			statuses = append(statuses, status)
			continue
		}

		// Get loss limit from transformer
		lossLimit := 10.0
		if t.LossLimitPct != nil {
			lossLimit = *t.LossLimitPct
		}

		// Check if overloaded (loss exceeds limit)
		limitPct := latestBalance.LossPct
		if limitPct == 0 {
			limitPct = lossLimit
		}

		overloadPct := 0.0
		if limitPct > 0 {
			overloadPct = (latestBalance.LossPct / limitPct) * 100
		}

		status := TransformerCurrentStatus{
			TransformerID:       string(t.ID),
			Code:                t.Code,
			Status:              latestBalance.Status,
			LossPct:             latestBalance.LossPct,
			LossLimitPct:        lossLimit,
			EnergyInjectedKWh:   latestBalance.EnergyInjectedKWh,
			TotalConsumptionKWh: latestBalance.TotalConsumptionKWh,
			LossKWh:             latestBalance.LossKWh,
			UCCount:             latestBalance.UCCount,
			PeriodStart:         latestBalance.PeriodStart,
			PeriodEnd:           latestBalance.PeriodEnd,
			CalculatedAt:        latestBalance.CalculatedAt,
			IsOverloaded:        latestBalance.LossPct >= lossLimit,
			OverloadPct:         overloadPct,
		}
		statuses = append(statuses, status)
	}

	request.WriteJSON(w, http.StatusOK, request.Success(statuses, ""))
}

// GetMonthlyLossHistory returns monthly loss history for dashboard charts
func (h *DashboardHandler) GetMonthlyLossHistory(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	ctx := context.Background()

	// Get all transformers for tenant
	transformers, err := h.transformerRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", "failed to get transformers", nil))
		return
	}

	// Simplified: just return aggregated data for all transformers
	// In production, you'd query by month and aggregate
	totalLossPct := 0.0
	totalEnergyKWh := 0.0
	totalBilledKWh := 0.0
	balanceCount := 0

	for _, t := range transformers {
		balances, err := h.balanceRepo.GetByTransformerAndPeriod(ctx, t.ID, time.Now().AddDate(0, -12, 0), time.Now())
		if err != nil {
			continue
		}

		for _, b := range balances {
			totalLossPct += b.LossPct
			totalEnergyKWh += b.EnergyInjectedKWh
			totalBilledKWh += b.TotalConsumptionKWh
			balanceCount++
		}
	}

	avgLossPct := 0.0
	if balanceCount > 0 {
		avgLossPct = totalLossPct / float64(balanceCount)
	}

	response := map[string]any{
		"average_loss_pct": avgLossPct,
		"total_energy_kwh": totalEnergyKWh,
		"total_billed_kwh": totalBilledKWh,
		"balance_count":    balanceCount,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// GetKPISparklines returns sparkline data for each KPI card
func (h *DashboardHandler) GetKPISparklines(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	ctx := context.Background()

	// Get last 12 months of data for sparklines
	monthlyAggregates, err := h.balanceRepo.GetMonthlyAggregatesByTenant(ctx, tenantID, 12)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("SPARKLINE_ERROR", err.Error(), nil))
		return
	}

	// Energy injected sparkline
	energyInjectedPoints := make([]SparklinePoint, len(monthlyAggregates))
	for i, agg := range monthlyAggregates {
		energyInjectedPoints[i] = SparklinePoint{
			Date:  agg.Month.Format("2006-01"),
			Value: agg.TotalEnergyInjected,
		}
	}

	// Total loss sparkline with ANEEL limit
	lossPoints := make([]SparklinePointWithLimit, len(monthlyAggregates))
	// Get average ANEEL limit across transformers
	var avgAneelLimit float64
	transformers, _ := h.transformerRepo.GetByTenant(ctx, tenantID)
	if len(transformers) > 0 {
		var sumLimit float64
		for _, t := range transformers {
			if t.LossLimitPct != nil {
				sumLimit += *t.LossLimitPct
			} else {
				sumLimit += 10.0
			}
		}
		avgAneelLimit = sumLimit / float64(len(transformers))
	}
	for i, agg := range monthlyAggregates {
		lossPoints[i] = SparklinePointWithLimit{
			Date:      agg.Month.Format("2006-01"),
			Value:     agg.AvgLossPct,
			Limit:     avgAneelLimit,
		}
	}

	// Loss cost sparkline (using average tariff)
	// In production, this would come from tenant settings
	tariffPerMWh := 450.0 // R$/MWh default
	costPoints := make([]SparklinePoint, len(monthlyAggregates))
	for i, agg := range monthlyAggregates {
		costBRL := (agg.TotalLoss / 1000.0) * tariffPerMWh
		costPoints[i] = SparklinePoint{
			Date:  agg.Month.Format("2006-01"),
			Value: costBRL,
		}
	}

	// Billed revenue sparkline
	revenuePoints := make([]SparklinePoint, len(monthlyAggregates))
	for i, agg := range monthlyAggregates {
		revenueBRL := (agg.TotalConsumption / 1000.0) * tariffPerMWh
		revenuePoints[i] = SparklinePoint{
			Date:  agg.Month.Format("2006-01"),
			Value: revenueBRL,
		}
	}

	// Potential savings (difference between current loss and ANEEL limit)
	savingsPoints := make([]SparklinePoint, len(monthlyAggregates))
	for i, agg := range monthlyAggregates {
		allowedLossKWh := agg.TotalEnergyInjected * (avgAneelLimit / 100.0)
		excessLossKWh := agg.TotalLoss - allowedLossKWh
		if excessLossKWh < 0 {
			excessLossKWh = 0
		}
		savingsBRL := (excessLossKWh / 1000.0) * tariffPerMWh
		savingsPoints[i] = SparklinePoint{
			Date:  agg.Month.Format("2006-01"),
			Value: savingsBRL,
		}
	}

	response := map[string]any{
		"energy_injected":  energyInjectedPoints,
		"total_loss":       lossPoints,
		"loss_cost":        costPoints,
		"billed_revenue":   revenuePoints,
		"potential_savings": savingsPoints,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// GetLossEvolution returns monthly loss evolution with ANEEL limit
func (h *DashboardHandler) GetLossEvolution(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	ctx := context.Background()

	monthlyAggregates, err := h.balanceRepo.GetMonthlyAggregatesByTenant(ctx, tenantID, 12)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("EVOLUTION_ERROR", err.Error(), nil))
		return
	}

	// Get average ANEEL limit
	var avgAneelLimit float64
	transformers, _ := h.transformerRepo.GetByTenant(ctx, tenantID)
	if len(transformers) > 0 {
		var sumLimit float64
		for _, t := range transformers {
			if t.LossLimitPct != nil {
				sumLimit += *t.LossLimitPct
			} else {
				sumLimit += 10.0
			}
		}
		avgAneelLimit = sumLimit / float64(len(transformers))
	}

	type EvolutionPoint struct {
		Month         string  `json:"month"`
		TotalLossPct  float64 `json:"total_loss_pct"`
		TechnicalPct  float64 `json:"technical_pct"`
		NonTechnicalPct float64 `json:"non_technical_pct"`
		AneelLimit    float64 `json:"aneel_limit"`
	}

	points := make([]EvolutionPoint, len(monthlyAggregates))
	for i, agg := range monthlyAggregates {
		techPct := 0.0
		nonTechPct := 0.0
		if agg.TotalEnergyInjected > 0 {
			techPct = (agg.TotalTechnicalLoss / agg.TotalEnergyInjected) * 100
			nonTechPct = (agg.TotalNonTechnicalLoss / agg.TotalEnergyInjected) * 100
		}
		points[i] = EvolutionPoint{
			Month:            agg.Month.Format("2006-01"),
			TotalLossPct:     agg.AvgLossPct,
			TechnicalPct:     techPct,
			NonTechnicalPct:  nonTechPct,
			AneelLimit:       avgAneelLimit,
		}
	}

	request.WriteJSON(w, http.StatusOK, request.Success(points, ""))
}

// GetLossComposition returns technical vs non-technical loss donut data
func (h *DashboardHandler) GetLossComposition(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	ctx := context.Background()

	comp, err := h.balanceRepo.GetLossCompositionByTenant(ctx, tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("COMPOSITION_ERROR", err.Error(), nil))
		return
	}

	total := comp.TotalTechnicalLoss + comp.TotalNonTechnicalLoss
	techPct := 0.0
	nonTechPct := 0.0
	if total > 0 {
		techPct = (comp.TotalTechnicalLoss / total) * 100
		nonTechPct = (comp.TotalNonTechnicalLoss / total) * 100
	}

	response := map[string]any{
		"technical":      comp.TotalTechnicalLoss,
		"non_technical":  comp.TotalNonTechnicalLoss,
		"technical_pct":  techPct,
		"non_technical_pct": nonTechPct,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// GetTransformerStatusDistribution returns status donut data
func (h *DashboardHandler) GetTransformerStatusDistribution(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	ctx := context.Background()

	counts, err := h.balanceRepo.GetTransformerStatusCounts(ctx, tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("STATUS_DIST_ERROR", err.Error(), nil))
		return
	}

	total := counts.Normal + counts.Warning + counts.Critical
	response := map[string]any{
		"normal":   counts.Normal,
		"warning":  counts.Warning,
		"critical": counts.Critical,
		"total":    total,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// GetQuarterlyInjectedBilled returns quarterly injected vs billed bars
func (h *DashboardHandler) GetQuarterlyInjectedBilled(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	ctx := context.Background()

	aggregates, err := h.balanceRepo.GetQuarterlyAggregatesByTenant(ctx, tenantID, 4)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("QUARTERLY_ERROR", err.Error(), nil))
		return
	}

	type QuarterlyPoint struct {
		Quarter      string  `json:"quarter"`
		InjectedMWh  float64 `json:"injected_mwh"`
		BilledMWh    float64 `json:"billed_mwh"`
	}

	points := make([]QuarterlyPoint, len(aggregates))
	for i, agg := range aggregates {
		points[i] = QuarterlyPoint{
			Quarter:     agg.Quarter.Format("2006-Q"),
			InjectedMWh: agg.TotalEnergyInjected / 1000.0,
			BilledMWh:   agg.TotalConsumption / 1000.0,
		}
	}

	request.WriteJSON(w, http.StatusOK, request.Success(points, ""))
}

// GetAccumulatedLossCost returns accumulated loss cost area chart
func (h *DashboardHandler) GetAccumulatedLossCost(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	ctx := context.Background()

	// Get tariff from query param or use default
	tariffPerMWh := 450.0 // default R$/MWh
	if tariffStr := r.URL.Query().Get("tariff"); tariffStr != "" {
		if t, err := strconv.ParseFloat(tariffStr, 64); err == nil {
			tariffPerMWh = t
		}
	}

	points, err := h.balanceRepo.GetAccumulatedLossCost(ctx, tenantID, tariffPerMWh, 12)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("COST_ERROR", err.Error(), nil))
		return
	}

	type CostPoint struct {
		Month              string  `json:"month"`
		MonthlyLossKWh     float64 `json:"monthly_loss_kwh"`
		AccumulatedLossKWh float64 `json:"accumulated_loss_kwh"`
		MonthlyCostBRL     float64 `json:"monthly_cost_brl"`
		AccumulatedCostBRL float64 `json:"accumulated_cost_brl"`
	}

	response := make([]CostPoint, len(points))
	for i, p := range points {
		response[i] = CostPoint{
			Month:              p.Month.Format("2006-01"),
			MonthlyLossKWh:     p.MonthlyLossKWh,
			AccumulatedLossKWh: p.AccumulatedLossKWh,
			MonthlyCostBRL:     p.MonthlyCostBRL,
			AccumulatedCostBRL: p.AccumulatedCostBRL,
		}
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// GetTopTransformersByLoss returns top 5 transformers by loss
func (h *DashboardHandler) GetTopTransformersByLoss(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	ctx := context.Background()

	limit := 5
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	transformers, err := h.balanceRepo.GetTopTransformersByLoss(ctx, tenantID, limit)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("TOP_ERROR", err.Error(), nil))
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

	response := make([]TopTransformer, len(transformers))
	for i, t := range transformers {
		response[i] = TopTransformer{
			TransformerID:       string(t.TransformerID),
			Code:                t.Code,
			LossPct:             t.LossPct,
			LossKWh:             t.LossKWh,
			EnergyInjectedKWh:   t.EnergyInjectedKWh,
			TechnicalLossKWh:    t.TechnicalLossKWh,
			NonTechnicalLossKWh: t.NonTechnicalLossKWh,
		}
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// GetFinancialTable returns financial table with configurable tariff
func (h *DashboardHandler) GetFinancialTable(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	ctx := context.Background()

	tariffPerMWh := 450.0
	if tariffStr := r.URL.Query().Get("tariff"); tariffStr != "" {
		if t, err := strconv.ParseFloat(tariffStr, 64); err == nil {
			tariffPerMWh = t
		}
	}

	// Get latest balances for all transformers
	transformers, _ := h.transformerRepo.GetByTenant(ctx, tenantID)

	type FinancialRow struct {
		TransformerID     string  `json:"transformer_id"`
		Code              string  `json:"code"`
		LossPct           float64 `json:"loss_pct"`
		LossKWh           float64 `json:"loss_kwh"`
		LossCostBRL       float64 `json:"loss_cost_brl"`
		EnergyInjectedKWh float64 `json:"energy_injected_kwh"`
		EnergyBilledKWh   float64 `json:"energy_billed_kwh"`
		RevenueBRL        float64 `json:"revenue_brl"`
		PotentialSavingsBRL float64 `json:"potential_savings_brl"`
	}

	rows := make([]FinancialRow, 0, len(transformers))
	for _, t := range transformers {
		latest, _ := h.balanceRepo.GetLatestByTransformer(ctx, t.ID)
		if latest == nil {
			continue
		}

		lossCostBRL := (latest.LossKWh / 1000.0) * tariffPerMWh
		revenueBRL := (latest.TotalConsumptionKWh / 1000.0) * tariffPerMWh

		// Potential savings: excess loss above ANEEL limit
		limit := 10.0
		if t.LossLimitPct != nil {
			limit = *t.LossLimitPct
		}
		allowedLossKWh := latest.EnergyInjectedKWh * (limit / 100.0)
		excessLossKWh := latest.LossKWh - allowedLossKWh
		if excessLossKWh < 0 {
			excessLossKWh = 0
		}
		potentialSavingsBRL := (excessLossKWh / 1000.0) * tariffPerMWh

		rows = append(rows, FinancialRow{
			TransformerID:       string(t.ID),
			Code:                t.Code,
			LossPct:             latest.LossPct,
			LossKWh:             latest.LossKWh,
			LossCostBRL:         lossCostBRL,
			EnergyInjectedKWh:   latest.EnergyInjectedKWh,
			EnergyBilledKWh:     latest.TotalConsumptionKWh,
			RevenueBRL:          revenueBRL,
			PotentialSavingsBRL: potentialSavingsBRL,
		})
	}

	request.WriteJSON(w, http.StatusOK, request.Success(rows, ""))
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
