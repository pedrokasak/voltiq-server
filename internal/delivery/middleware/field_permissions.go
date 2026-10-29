package middleware

import (
	"github.com/voltiq/server/internal/domain"
)

// CanViewFinancialData verifica se role tem acesso a dados financeiros
func CanViewFinancialData(role domain.UserRole) bool {
	return domain.HasPermission(role, domain.PermissionFinancialView)
}

// BalanceResponseDTO is the balance returned to every role
type BalanceResponseDTO struct {
	ID                  domain.UUID `json:"id"`
	TransformerID       domain.UUID `json:"transformer_id"`
	PeriodStart         string      `json:"period_start"`
	PeriodEnd           string      `json:"period_end"`
	EnergyInjectedKWh   float64     `json:"energy_injected_kwh"`
	TotalConsumptionKWh float64     `json:"total_consumption_kwh"`
	LossKWh             float64     `json:"loss_kwh"`
	LossPct             float64     `json:"loss_pct"`
	TechnicalLossKWh    *float64    `json:"technical_loss_kwh"`
	NonTechnicalLossKWh *float64    `json:"non_technical_loss_kwh"`
	LossLimitPct        *float64    `json:"loss_limit_pct"`
	Status              string      `json:"status"`
	UCCount             int         `json:"uc_count"`
	CalculatedAt        string      `json:"calculated_at"`
}

// GetBalanceDTO retorna o balanco para a API. O balanco so tem grandezas
// operacionais (kWh, %, status), visiveis a todos os papeis; valores em R$
// ficam nos endpoints financeiros (financial:view).
func GetBalanceDTO(balance *domain.TransformerBalance, _ domain.UserRole) any {
	return &BalanceResponseDTO{
		ID:                  balance.ID,
		TransformerID:       balance.TransformerID,
		PeriodStart:         balance.PeriodStart.Format("2006-01-02"),
		PeriodEnd:           balance.PeriodEnd.Format("2006-01-02"),
		EnergyInjectedKWh:   balance.EnergyInjectedKWh,
		TotalConsumptionKWh: balance.TotalConsumptionKWh,
		LossKWh:             balance.LossKWh,
		LossPct:             balance.LossPct,
		TechnicalLossKWh:    balance.TechnicalLossKWh,
		NonTechnicalLossKWh: balance.NonTechnicalLossKWh,
		LossLimitPct:        balance.LossLimitPct,
		Status:              string(balance.Status),
		UCCount:             balance.UCCount,
		CalculatedAt:        balance.CalculatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
