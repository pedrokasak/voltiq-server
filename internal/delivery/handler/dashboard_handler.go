package handler

import (
	"context"
	"net/http"
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
