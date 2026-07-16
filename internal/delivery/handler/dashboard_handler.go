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
