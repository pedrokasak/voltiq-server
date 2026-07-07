// Package calc implements the technical loss calculation engine per
// PRODIST Module 7 (ANEEL) — Revision 13 (valid 2024).
// Section 6: Methodology for calculating technical losses in distribution.
package calc

import (
	"errors"
	"math"
	"time"

	"github.com/voltiq/server/internal/domain"
)

var (
	ErrEnergyInjectedZero   = errors.New("energy injected cannot be zero or negative")
	ErrPeriodInvalid        = errors.New("period in hours must be greater than zero")
	ErrNameplateDataMissing = errors.New("P0 and Pcc are required for PRODIST M7 calculation")
	ErrLimitInvalid         = errors.New("regulatory loss limit must be greater than zero")
)

// CalculateBalance is the entry point for the engine.
// Applies PRODIST M7 and returns the complete balance result.
//
// Example:
//
//	input := domain.CalculationInput{
//	    Transformer:     transformer,     // registration data + nameplate
//	    EnergyInjected:  15000.0,         // kWh injected in the period
//	    UCConsumptions:  []float64{...},  // consumption of each UC
//	    PeriodHours:     720,             // 30 days × 24h
//	    AverageCurrent:  45.2,            // Amperes measured (optional)
//	}
//	result, err := calc.CalculateBalance(input)
func CalculateBalance(input domain.CalculationInput) (domain.CalculationResult, error) {
	if err := validateInput(input); err != nil {
		return domain.CalculationResult{}, err
	}

	totalConsumption := sumConsumptions(input.UCConsumptions)
	lossKWh := calculateTotalLoss(input.EnergyInjected, totalConsumption)
	lossPct := calculateLossPct(lossKWh, input.EnergyInjected)
	technicalLoss := calculateTechnicalLossTransformer(input.Transformer, input.PeriodHours, input.AverageCurrent)
	nonTechnicalLoss := calculateNonTechnicalLoss(lossKWh, technicalLoss)

	limitPct := 10.0
	if input.Transformer.LossLimitPct != nil {
		limitPct = *input.Transformer.LossLimitPct
	}
	status := classifyStatus(lossPct, limitPct)

	return domain.CalculationResult{
		TransformerID: input.Transformer.ID,
		TenantID:      input.Transformer.TenantID,

		EnergyInjectedKWh:   input.EnergyInjected,
		TotalConsumptionKWh: totalConsumption,
		LossKWh:             lossKWh,
		LossPct:             lossPct,

		TechnicalLossTrafoKWh: technicalLoss,
		NonTechnicalLossKWh:   nonTechnicalLoss,

		Status:   status,
		LimitPct: limitPct,
		UCCount:  len(input.UCConsumptions),

		CalculatedAt: time.Now().UTC(),
	}, nil
}

// CalculateBalanceBatch processes multiple transformers in parallel via goroutines.
// Returns results in the same order as inputs.
// Individual errors do not cancel the batch.
func CalculateBalanceBatch(inputs []domain.CalculationInput) ([]domain.CalculationResult, []error) {
	results := make([]domain.CalculationResult, len(inputs))
	errors := make([]error, len(inputs))

	type item struct {
		idx int
		res domain.CalculationResult
		err error
	}

	ch := make(chan item, len(inputs))

	for i, input := range inputs {
		go func(idx int, inp domain.CalculationInput) {
			res, err := CalculateBalance(inp)
			ch <- item{idx, res, err}
		}(i, input)
	}

	for range inputs {
		it := <-ch
		results[it.idx] = it.res
		errors[it.idx] = it.err
	}

	return results, errors
}

// --- Internal functions ---

// sumConsumptions sums the consumptions of all linked UCs.
// PRODIST M7, Section 6.1: billed energy = Σ(UC_consumptions)
func sumConsumptions(consumptions []float64) float64 {
	var total float64
	for _, c := range consumptions {
		total += c
	}
	return total
}

// calculateTotalLoss calculates the total energy loss.
// PRODIST M7, Section 6.1:
//
//	Loss_Total (kWh) = Energy_Injected - Σ(UC_consumptions)
func calculateTotalLoss(injected, totalConsumption float64) float64 {
	return injected - totalConsumption
}

// calculateLossPct calculates the loss percentage over injected energy.
// PRODIST M7, Section 6.1:
//
//	% Loss = (Loss_Total / Energy_Injected) × 100
func calculateLossPct(lossKWh, injected float64) float64 {
	if injected == 0 {
		return 0
	}
	return (lossKWh / injected) * 100
}

// calculateTechnicalLossTransformer calculates technical losses in the transformer.
// PRODIST M7, Section 6.2 — Transformer Losses:
//
//	PT_trafo = P0 × T + Pcc × (Ic/In)² × T
//
//	P0  = no-load losses in core (kW) — nameplate data
//	Pcc = load losses in windings at nominal load (kW) — nameplate data
//	Ic  = average current of period (A)
//	In  = nominal current (A)
//	T   = period duration in hours
//
//	Result in kWh.
func calculateTechnicalLossTransformer(t domain.Transformer, periodHours, averageCurrent float64) float64 {
	if t.CoreLossKW == nil || t.WindingLossKW == nil {
		return 0
	}

	// Core losses: occur 24h/day regardless of load
	coreLoss := *t.CoreLossKW * periodHours

	// Winding losses: vary with (Ic/In)²
	var windingLoss float64
	nominalCurrent := t.NominalCurrent()

	if nominalCurrent > 0 && averageCurrent > 0 {
		loadFactor := averageCurrent / nominalCurrent
		windingLoss = *t.WindingLossKW * math.Pow(loadFactor, 2) * periodHours
	} else {
		// No measured current: conservative estimate of 50% loading
		windingLoss = *t.WindingLossKW * math.Pow(0.5, 2) * periodHours
	}

	return coreLoss + windingLoss
}

// calculateNonTechnicalLoss estimates non-technical losses (theft, fraud, metering error).
// PRODIST M7, Section 6.4:
//
//	PNT = Loss_Total - Technical_Loss
//
//	Negative value (inconsistent nameplate data) is normalized to zero.
func calculateNonTechnicalLoss(lossTotal, technicalLoss float64) float64 {
	pnt := lossTotal - technicalLoss
	if pnt < 0 {
		return 0
	}
	return pnt
}

// classifyStatus determines the regulatory status of the transformer.
// RF03.4:
//
//	NORMAL   → loss < 80% of ANEEL limit
//	WARNING  → loss between 80% and 100% of limit
//	CRITICAL → loss ≥ regulatory limit
func classifyStatus(lossPct, limitPct float64) domain.BalanceCalculationStatus {
	if limitPct <= 0 {
		return domain.BalanceCalculationStatusNormal
	}
	switch {
	case lossPct >= limitPct:
		return domain.BalanceCalculationStatusCritical
	case lossPct >= limitPct*0.80:
		return domain.BalanceCalculationStatusWarning
	default:
		return domain.BalanceCalculationStatusNormal
	}
}

func validateInput(input domain.CalculationInput) error {
	if input.EnergyInjected <= 0 {
		return ErrEnergyInjectedZero
	}
	if input.PeriodHours <= 0 {
		return ErrPeriodInvalid
	}
	if input.Transformer.CoreLossKW == nil || input.Transformer.WindingLossKW == nil {
		return ErrNameplateDataMissing
	}
	if input.Transformer.LossLimitPct == nil || *input.Transformer.LossLimitPct <= 0 {
		return ErrLimitInvalid
	}
	return nil
}

// --- Exported helpers ---

// PeriodInHours converts a time interval to hours.
func PeriodInHours(start, end time.Time) float64 {
	return end.Sub(start).Hours()
}

// LoadFactor calculates the transformer load factor (0 to 1).
func LoadFactor(averageCurrent, nominalCurrent float64) float64 {
	if nominalCurrent == 0 {
		return 0
	}
	return math.Min(averageCurrent/nominalCurrent, 1.0)
}

// EnergyFromDemand estimates energy (kWh) from average demand (kW) and period (h).
func EnergyFromDemand(averageDemandKW, periodHours float64) float64 {
	return averageDemandKW * periodHours
}

// CalculateTechnicalLoss calculates technical losses according to PRODIST M7.
// This is a wrapper that returns a pointer to the result for use in usecase layer.
func CalculateTechnicalLoss(input *domain.CalculationInput) *domain.CalculationResult {
	if input == nil {
		return nil
	}

	result, _ := CalculateBalance(*input)
	return &result
}
