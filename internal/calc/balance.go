// Package calc provides basic balance calculation functions.
package calc

import (
	"time"

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
// NORMAL → loss < 80% of ANEEL limit
// WARNING → loss between 80% and 100% of limit
// CRITICAL → loss ≥ regulatory limit
func DetermineBalanceStatus(lossPct float64, limitPct *float64) domain.BalanceCalculationStatus {
	if limitPct == nil {
		defaultLimit := 10.0
		limitPct = &defaultLimit
	}

	if lossPct <= *limitPct*0.8 {
		return domain.BalanceCalculationStatusNormal
	} else if lossPct <= *limitPct {
		return domain.BalanceCalculationStatusWarning
	}
	return domain.BalanceCalculationStatusCritical
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

// BalanceService provides business logic for balance calculations.
type BalanceService struct {
	balanceRepo            BalanceRepository
	transformerReadingRepo TransformerReadingRepository
	ucReadingRepo          UCReadingRepository
	ucRepo                 ConsumingUnitRepository
	transformerRepo        TransformerRepository
}

// NewBalanceService creates a new BalanceService.
func NewBalanceService(
	balanceRepo BalanceRepository,
	transformerReadingRepo TransformerReadingRepository,
	ucReadingRepo UCReadingRepository,
	ucRepo ConsumingUnitRepository,
	transformerRepo TransformerRepository,
) *BalanceService {
	return &BalanceService{
		balanceRepo:            balanceRepo,
		transformerReadingRepo: transformerReadingRepo,
		ucReadingRepo:          ucReadingRepo,
		ucRepo:                 ucRepo,
		transformerRepo:        transformerRepo,
	}
}

// CalculateBalancePeriod calculates the balance for a transformer in a given period.
func (s *BalanceService) CalculateBalancePeriod(
	transformerID domain.UUID,
	tenantID domain.UUID,
	periodStart time.Time,
	periodEnd time.Time,
) (*domain.TransformerBalance, error) {
	return nil, nil
}

// RecalculateAllTransformers recalculates balance for all transformers of a tenant.
func (s *BalanceService) RecalculateAllTransformers(tenantID domain.UUID) error {
	return nil
}

// Repository interfaces for dependency injection
type BalanceRepository interface {
	Create(balance *domain.TransformerBalance) error
	GetByTransformerAndPeriod(transformerID domain.UUID, start, end time.Time) ([]*domain.TransformerBalance, error)
	GetLatestByTransformer(transformerID domain.UUID) (*domain.TransformerBalance, error)
}

type TransformerRepository interface {
	GetByID(id domain.UUID) (*domain.Transformer, error)
	GetByTenant(tenantID domain.UUID) ([]*domain.Transformer, error)
}

type TransformerReadingRepository interface {
	GetTotalByTransformerAndPeriod(transformerID domain.UUID, start, end time.Time) (float64, error)
}

type UCReadingRepository interface {
	GetTotalByTransformerAndPeriod(transformerID domain.UUID, start, end time.Time) (float64, error)
}

type ConsumingUnitRepository interface {
	GetByTransformer(transformerID domain.UUID) ([]*domain.ConsumingUnit, error)
}
