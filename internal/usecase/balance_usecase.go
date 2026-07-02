package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/energybalance/server/internal/calc"
	"github.com/energybalance/server/internal/domain"
	"github.com/energybalance/server/internal/repository"
)

var (
	ErrBalanceNotFound       = errors.New("balance not found")
	ErrInsufficientData      = errors.New("insufficient data for balance calculation")
	ErrTransformerNotFound   = errors.New("transformer not found")
)

// BalanceUseCase handles energy balance calculation business logic
type BalanceUseCase struct {
	balanceRepo      *repository.BalanceRepository
	transformerRepo  *repository.TransformerRepository
	trafoReadingRepo *repository.TransformerReadingRepository
	ucReadingRepo    *repository.UCReadingRepository
	ucRepo           *repository.ConsumingUnitRepository
}

// CalculateBalanceInput contains data for balance calculation
type CalculateBalanceInput struct {
	TransformerID domain.UUID
	PeriodStart   time.Time
	PeriodEnd     time.Time
}

// CalculateBalanceOutput contains the calculation result
type CalculateBalanceOutput struct {
	Balance *domain.TransformerBalance
}

// NewBalanceUseCase creates a new BalanceUseCase
func NewBalanceUseCase(
	balanceRepo *repository.BalanceRepository,
	transformerRepo *repository.TransformerRepository,
	trafoReadingRepo *repository.TransformerReadingRepository,
	ucReadingRepo *repository.UCReadingRepository,
	ucRepo *repository.ConsumingUnitRepository,
) *BalanceUseCase {
	return &BalanceUseCase{
		balanceRepo:      balanceRepo,
		transformerRepo:  transformerRepo,
		trafoReadingRepo: trafoReadingRepo,
		ucReadingRepo:    ucReadingRepo,
		ucRepo:           ucRepo,
	}
}

// CalculateBalance calculates the energy balance for a transformer in a period
func (uc *BalanceUseCase) CalculateBalance(ctx context.Context, input CalculateBalanceInput) (*CalculateBalanceOutput, error) {
	transformer, err := uc.transformerRepo.GetByID(ctx, input.TransformerID)
	if err != nil {
		return nil, ErrTransformerNotFound
	}

	if transformer == nil {
		return nil, ErrTransformerNotFound
	}

	trafoReadings, err := uc.trafoReadingRepo.GetByTransformerAndPeriod(ctx, input.TransformerID, input.PeriodStart, input.PeriodEnd)
	if err != nil {
		return nil, errors.New("failed to get transformer readings")
	}

	if len(trafoReadings) == 0 {
		return nil, ErrInsufficientData
	}

	ucs, err := uc.ucRepo.GetByTransformer(ctx, input.TransformerID)
	if err != nil {
		return nil, errors.New("failed to get consuming units")
	}

	var totalConsumption float64
	for _, u := range ucs {
		ucReadings, err := uc.ucReadingRepo.GetByUCAndPeriod(ctx, u.ID, input.PeriodStart, input.PeriodEnd)
		if err != nil {
			continue
		}
		for _, r := range ucReadings {
			totalConsumption += r.ConsumptionKWh
		}
	}

	var totalInjected float64
	for _, r := range trafoReadings {
		totalInjected += r.EnergyKWh
	}

	lossKWh := totalInjected - totalConsumption
	lossPct := 0.0
	if totalInjected > 0 {
		lossPct = (lossKWh / totalInjected) * 100
	}

	status := domain.BalanceStatusNormal
	limitPct := 10.0
	if transformer.LossLimitPct != nil {
		limitPct = *transformer.LossLimitPct
	}

	if lossPct > limitPct {
		status = domain.BalanceStatusCritical
	} else if lossPct > limitPct*0.7 {
		status = domain.BalanceStatusWarning
	}

	balance := &domain.TransformerBalance{
		ID:                  domain.UUID(time.Now().Format("20060102150405")),
		TenantID:            transformer.TenantID,
		TransformerID:       transformer.ID,
		PeriodStart:         input.PeriodStart,
		PeriodEnd:           input.PeriodEnd,
		EnergyInjectedKWh:   totalInjected,
		TotalConsumptionKWh: totalConsumption,
		LossKWh:             lossKWh,
		LossPct:             lossPct,
		Status:              status,
		UCCount:             len(ucs),
		CalculatedAt:        time.Now(),
	}

	if err := uc.balanceRepo.Create(ctx, balance); err != nil {
		return nil, errors.New("failed to save balance")
	}

	return &CalculateBalanceOutput{
		Balance: balance,
	}, nil
}

// ListBalanceByTransformer returns balance records for a transformer in a period
func (uc *BalanceUseCase) ListBalanceByTransformer(ctx context.Context, transformerID domain.UUID, start, end time.Time) ([]*domain.TransformerBalance, error) {
	balances, err := uc.balanceRepo.GetByTransformerAndPeriod(ctx, transformerID, start, end)
	if err != nil {
		return nil, errors.New("failed to list balances")
	}

	return balances, nil
}

// GetLatestBalance returns the most recent balance for a transformer
func (uc *BalanceUseCase) GetLatestBalance(ctx context.Context, transformerID domain.UUID) (*domain.TransformerBalance, error) {
	balance, err := uc.balanceRepo.GetLatestByTransformer(ctx, transformerID)
	if err != nil {
		return nil, err
	}

	if balance == nil {
		return nil, ErrBalanceNotFound
	}

	return balance, nil
}

// CalculateTechnicalLoss calculates technical losses according to PRODIST Module 7
// PT_trafo = P0×T + Pcc×(Ic/In)²×T
func (uc *BalanceUseCase) CalculateTechnicalLoss(ctx context.Context, input CalculateBalanceInput) (*domain.CalculationResult, error) {
	transformer, err := uc.transformerRepo.GetByID(ctx, input.TransformerID)
	if err != nil {
		return nil, ErrTransformerNotFound
	}

	if transformer == nil {
		return nil, ErrTransformerNotFound
	}

	trafoReadings, err := uc.trafoReadingRepo.GetByTransformerAndPeriod(ctx, input.TransformerID, input.PeriodStart, input.PeriodEnd)
	if err != nil {
		return nil, errors.New("failed to get transformer readings")
	}

	ucs, err := uc.ucRepo.GetByTransformer(ctx, input.TransformerID)
	if err != nil {
		return nil, errors.New("failed to get consuming units")
	}

	var totalConsumption float64
	var consumptions []float64
	for _, u := range ucs {
		ucReadings, err := uc.ucReadingRepo.GetByUCAndPeriod(ctx, u.ID, input.PeriodStart, input.PeriodEnd)
		if err != nil {
			continue
		}
		for _, r := range ucReadings {
			consumptions = append(consumptions, r.ConsumptionKWh)
			totalConsumption += r.ConsumptionKWh
		}
	}

	var totalInjected float64
	for _, r := range trafoReadings {
		totalInjected += r.EnergyKWh
	}

	calcInput := &domain.CalculationInput{
		Transformer:    *transformer,
		EnergyInjected: totalInjected,
		UCConsumptions: consumptions,
		PeriodHours:    input.PeriodEnd.Sub(input.PeriodStart).Hours(),
	}

	result := calc.CalculateTechnicalLoss(calcInput)
	result.TransformerID = transformer.ID
	result.TenantID = transformer.TenantID

	return result, nil
}
