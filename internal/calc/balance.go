// Package calc provides basic balance calculation functions.
package calc

import (
	"github.com/voltiq/server/internal/domain"
)

// CalculateBasicBalance calculates the energy balance of a transformer
// per PRODIST Module 7, Section 6.2.
// Returns the difference between injected energy and sum of linked UC consumptions.
func CalculateBasicBalance(injected float64, consumptions []float64) float64 {
	var totalConsumption float64
	for _, c := range consumptions {
		totalConsumption += c
	}
	return injected - totalConsumption
}

// CalculateLossPercentage calculates the loss percentage over injected energy.
func CalculateLossPercentage(loss float64, injected float64) float64 {
	if injected == 0 {
		return 0
	}
	return (loss / injected) * 100
}

// DetermineBalanceStatus determines the regulatory status of the transformer.
// RF03.4 (mesma regra de classifyStatus):
// NORMAL   → loss < 80% of ANEEL limit
// WARNING  → loss between 80% and 100% of limit
// CRITICAL → loss ≥ regulatory limit
func DetermineBalanceStatus(lossPct float64, limitPct *float64) domain.BalanceCalculationStatus {
	limit := 10.0
	if limitPct != nil {
		limit = *limitPct
	}
	return classifyStatus(lossPct, limit)
}

// CalculateTechnicalLossPRODIST calculates technical losses per PRODIST M7.
// PT_trafo = P0 × T + Pcc × (Ic/In)² × T
func CalculateTechnicalLossPRODIST(
	energyInjected float64,
	powerKVA float64,
	coreLossKW *float64,
	windingLossKW *float64,
	periodHours float64,
) float64 {
	if coreLossKW == nil || windingLossKW == nil {
		return 0
	}

	coreLossPeriod := *coreLossKW * periodHours
	loadFactor := 0.5
	windingLossPeriod := *windingLossKW * loadFactor * loadFactor * periodHours

	return coreLossPeriod + windingLossPeriod
}
