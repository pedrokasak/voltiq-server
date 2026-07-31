package usecase

import (
	"context"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// FinancialRow represents a row in the financial table
type FinancialRow struct {
	TransformerID         string  `json:"transformer_id"`
	Code                  string  `json:"code"`
	LossPct               float64 `json:"loss_pct"`
	LossKWh               float64 `json:"loss_kwh"`
	LossCostBRL           float64 `json:"loss_cost_brl"`
	EnergyInjectedKWh     float64 `json:"energy_injected_kwh"`
	EnergyBilledKWh       float64 `json:"energy_billed_kwh"`
	RevenueBRL            float64 `json:"revenue_brl"`
	PotentialSavingsBRL   float64 `json:"potential_savings_brl"`
}

// FinancialTableData represents the complete financial table response
type FinancialTableData struct {
	TariffPerMWh    float64      `json:"tariff_per_mwh"`
	TotalLossCostBRL float64     `json:"total_loss_cost_brl"`
	TotalRevenueBRL  float64      `json:"total_revenue_brl"`
	TotalSavingsBRL  float64      `json:"total_savings_brl"`
	Rows            []FinancialRow `json:"rows"`
}

// FinancialUseCase handles financial data queries
type FinancialUseCase struct {
	balanceRepo   *repository.BalanceRepository
	transformerRepo *repository.TransformerRepository
}

// NewFinancialUseCase creates a new FinancialUseCase
func NewFinancialUseCase(
	balanceRepo *repository.BalanceRepository,
	transformerRepo *repository.TransformerRepository,
) *FinancialUseCase {
	return &FinancialUseCase{
		balanceRepo:   balanceRepo,
		transformerRepo: transformerRepo,
	}
}

// GetFinancialTable returns the financial table for a tenant
func (uc *FinancialUseCase) GetFinancialTable(ctx context.Context, tenantID domain.UUID, tariffPerMWh float64) (*FinancialTableData, error) {
	// Get all transformers for the tenant
	transformers, err := uc.transformerRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Get latest balance for each transformer
	var rows []FinancialRow
	totalLossCost := 0.0
	totalRevenue := 0.0
	totalSavings := 0.0

	for _, t := range transformers {
		latestBalance, err := uc.balanceRepo.GetLatestByTransformer(ctx, t.ID)
		if err != nil {
			continue // Skip transformers without balance data
		}

		if latestBalance == nil {
			continue
		}

		lossKWh := latestBalance.LossKWh
		if lossKWh < 0 {
			lossKWh = 0
		}

		lossCost := (lossKWh / 1000.0) * tariffPerMWh

		revenue := (latestBalance.TotalConsumptionKWh / 1000.0) * tariffPerMWh

		// Potential savings: excess loss above ANEEL limit
		limitPct := 10.0 // Default ANEEL limit
		if latestBalance.LossLimitPct != nil {
			limitPct = *latestBalance.LossLimitPct
		}

		excessLossPct := 0.0
		if latestBalance.LossPct > limitPct {
			excessLossPct = latestBalance.LossPct - limitPct
		}

		excessLossKWh := (excessLossPct / 100.0) * latestBalance.EnergyInjectedKWh
		if excessLossKWh < 0 {
			excessLossKWh = 0
		}

		potentialSavings := (excessLossKWh / 1000.0) * tariffPerMWh

		rows = append(rows, FinancialRow{
			TransformerID:         string(latestBalance.TransformerID),
			Code:                  t.Code,
			LossPct:              latestBalance.LossPct,
			LossKWh:              latestBalance.LossKWh,
			LossCostBRL:          lossCost,
			EnergyInjectedKWh:    latestBalance.EnergyInjectedKWh,
			EnergyBilledKWh:      latestBalance.TotalConsumptionKWh,
			RevenueBRL:           revenue,
			PotentialSavingsBRL:  potentialSavings,
		})

		totalLossCost += lossCost
		totalRevenue += revenue
		totalSavings += potentialSavings
	}

	return &FinancialTableData{
		TariffPerMWh:    tariffPerMWh,
		TotalLossCostBRL: totalLossCost,
		TotalRevenueBRL:  totalRevenue,
		TotalSavingsBRL:  totalSavings,
		Rows:             rows,
	}, nil
}