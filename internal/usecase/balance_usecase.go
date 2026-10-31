package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/voltiq/server/internal/calc"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

var (
	ErrBalanceNotFound     = errors.New("balance not found")
	ErrInsufficientData    = errors.New("insufficient data for balance calculation")
	ErrTransformerNotFound = errors.New("transformer not found")
	ErrInvalidPeriod       = errors.New("period_end must be after period_start")
)

// BalanceNotifier is notified after a balance is stored, so alerts can be
// dispatched (RF05). It is optional.
type BalanceNotifier interface {
	BalanceCalculated(ctx context.Context, transformer *domain.Transformer, balance *domain.TransformerBalance)
}

// BalanceUseCase handles energy balance calculation business logic
type BalanceUseCase struct {
	balanceRepo      *repository.BalanceRepository
	transformerRepo  *repository.TransformerRepository
	trafoReadingRepo *repository.TransformerReadingRepository
	ucReadingRepo    *repository.UCReadingRepository
	ucRepo           *repository.ConsumingUnitRepository
	notifier         BalanceNotifier
}

// CalculateBalanceInput contains data for balance calculation
type CalculateBalanceInput struct {
	TransformerID domain.UUID
	PeriodStart   time.Time
	PeriodEnd     time.Time

	// Valores informados manualmente (RF02.5). Quando presentes substituem a
	// soma das leituras do periodo.
	ManualInjectedKWh    *float64
	ManualConsumptionKWh *float64
}

// CalculateBalanceOutput contains the calculation result
type CalculateBalanceOutput struct {
	Balance *domain.TransformerBalance
	// Method is "PRODIST_M7" when nameplate data allowed the full technical
	// loss calculation, "BASIC" otherwise.
	Method string
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

// SetNotifier registers the component notified after each calculation.
func (uc *BalanceUseCase) SetNotifier(n BalanceNotifier) {
	uc.notifier = n
}

// NormalizePeriod torna o fim do periodo inclusivo: uma data sem horario
// (00:00) passa a valer ate 23:59:59 daquele dia, de modo que "2025-01-01 a
// 2025-01-31" cobre os 31 dias inteiros.
func NormalizePeriod(start, end time.Time) (time.Time, time.Time, error) {
	if end.Hour() == 0 && end.Minute() == 0 && end.Second() == 0 && end.Nanosecond() == 0 {
		end = end.Add(24*time.Hour - time.Nanosecond)
	}
	if !end.After(start) {
		return start, end, ErrInvalidPeriod
	}
	return start, end, nil
}

// CalculateBalance calcula o balanco de energia de um transformador no
// periodo, conforme PRODIST Modulo 7 (Secoes 6.1, 6.2 e 6.4), e grava o
// resultado. Recalcular o mesmo periodo substitui o resultado anterior.
func (uc *BalanceUseCase) CalculateBalance(ctx context.Context, input CalculateBalanceInput) (*CalculateBalanceOutput, error) {
	start, end, err := NormalizePeriod(input.PeriodStart, input.PeriodEnd)
	if err != nil {
		return nil, err
	}

	transformer, err := uc.transformerRepo.GetByID(ctx, input.TransformerID)
	if err != nil || transformer == nil {
		return nil, ErrTransformerNotFound
	}

	agg, err := uc.trafoReadingRepo.AggregateByTransformerAndPeriod(ctx, transformer.ID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate transformer readings: %w", err)
	}

	injected := agg.EnergyKWh
	if input.ManualInjectedKWh != nil {
		injected = *input.ManualInjectedKWh
	} else if agg.Count == 0 {
		return nil, ErrInsufficientData
	}

	ucs, err := uc.ucRepo.GetByTransformer(ctx, transformer.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get consuming units: %w", err)
	}

	consumption := 0.0
	if input.ManualConsumptionKWh != nil {
		consumption = *input.ManualConsumptionKWh
	} else {
		// Sem leitura de UC no periodo o balanco daria 100% de perda e
		// alertas falsos (ex.: leituras do trafo importadas antes das UCs).
		n, err := uc.ucReadingRepo.CountByTransformerAndPeriod(ctx, transformer.ID, start, end)
		if err != nil {
			return nil, fmt.Errorf("failed to count UC readings: %w", err)
		}
		if n == 0 {
			return nil, ErrInsufficientData
		}
		consumption, err = uc.ucReadingRepo.GetTotalByTransformerAndPeriod(ctx, transformer.ID, start, end)
		if err != nil {
			return nil, fmt.Errorf("failed to sum UC readings: %w", err)
		}
	}

	balance, method := buildBalance(*transformer, injected, consumption, len(ucs), agg, start, end)

	if err := uc.balanceRepo.UpsertForPeriod(ctx, balance); err != nil {
		return nil, fmt.Errorf("failed to save balance: %w", err)
	}

	if uc.notifier != nil {
		uc.notifier.BalanceCalculated(ctx, transformer, balance)
	}

	return &CalculateBalanceOutput{Balance: balance, Method: method}, nil
}

// buildBalance aplica o motor PRODIST M7 quando o trafo tem dados de placa
// (P0, Pcc e limite); sem eles calcula apenas o balanco e o percentual de
// perda (RF03.1 a RF03.4).
func buildBalance(
	t domain.Transformer,
	injected, consumption float64,
	ucCount int,
	agg repository.ReadingAggregate,
	start, end time.Time,
) (*domain.TransformerBalance, string) {
	limit := t.LossLimitPctValue()
	balance := &domain.TransformerBalance{
		ID:                  domain.UUID(uuid.New().String()),
		TenantID:            t.TenantID,
		TransformerID:       t.ID,
		PeriodStart:         start,
		PeriodEnd:           end,
		EnergyInjectedKWh:   injected,
		TotalConsumptionKWh: consumption,
		UCCount:             ucCount,
		CalculatedAt:        time.Now().UTC(),
		LossLimitPct:        &limit,
	}

	if t.LossLimitPct == nil {
		t.LossLimitPct = &limit
	}

	result, err := calc.CalculateBalance(domain.CalculationInput{
		Transformer:    t,
		EnergyInjected: injected,
		UCConsumptions: []float64{consumption},
		PeriodHours:    calc.PeriodInHours(start, end),
		AverageCurrent: averageCurrent(t, agg),
	})
	if err == nil {
		balance.LossKWh = result.LossKWh
		balance.LossPct = round(result.LossPct, 4)
		technical := round(result.TechnicalLossTrafoKWh, 4)
		nonTechnical := round(result.NonTechnicalLossKWh, 4)
		balance.TechnicalLossKWh = &technical
		balance.NonTechnicalLossKWh = &nonTechnical
		balance.Status = domain.BalanceStatus(result.Status)
		return balance, "PRODIST_M7"
	}

	loss := calc.CalculateBasicBalance(injected, []float64{consumption})
	pct := calc.CalculateLossPercentage(loss, injected)
	balance.LossKWh = loss
	balance.LossPct = round(pct, 4)
	balance.Status = domain.BalanceStatus(calc.DetermineBalanceStatus(pct, &limit))
	return balance, "BASIC"
}

// averageCurrent estima a corrente media (Ic) a partir da demanda media e do
// fator de potencia medidos: S = P / fp e Ic/In = S / Snominal. Sem demanda
// medida retorna 0 e o motor usa a estimativa conservadora de 50% de carga.
func averageCurrent(t domain.Transformer, agg repository.ReadingAggregate) float64 {
	if agg.AvgDemandKW == nil || *agg.AvgDemandKW <= 0 || t.PowerKVA <= 0 {
		return 0
	}
	pf := 0.92
	if agg.AvgPowerFactor != nil && *agg.AvgPowerFactor > 0 && *agg.AvgPowerFactor <= 1 {
		pf = *agg.AvgPowerFactor
	}
	apparentKVA := *agg.AvgDemandKW / pf
	return (apparentKVA / t.PowerKVA) * t.NominalCurrent()
}

func round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

// CalculateAllResult summarizes a tenant-wide calculation.
type CalculateAllResult struct {
	PeriodStart time.Time                    `json:"period_start"`
	PeriodEnd   time.Time                    `json:"period_end"`
	Total       int                          `json:"total"`
	Calculated  int                          `json:"calculated"`
	Skipped     int                          `json:"skipped"`
	Failed      int                          `json:"failed"`
	Normal      int                          `json:"normal"`
	Warning     int                          `json:"warning"`
	Critical    int                          `json:"critical"`
	Errors      []CalculateAllTransformerErr `json:"errors,omitempty"`
	DurationMS  int64                        `json:"duration_ms"`
}

// CalculateAllTransformerErr reports a transformer that could not be calculated.
type CalculateAllTransformerErr struct {
	TransformerID domain.UUID `json:"transformer_id"`
	Code          string      `json:"code"`
	Error         string      `json:"error"`
}

// CalculateAll recalcula o balanco de todos os trafos ativos do tenant no
// periodo, em paralelo com concorrencia limitada (RNF01.3: 500 trafos < 3 s).
// Trafos sem leitura no periodo sao contados como ignorados.
func (uc *BalanceUseCase) CalculateAll(ctx context.Context, tenantID domain.UUID, start, end time.Time) (*CalculateAllResult, error) {
	began := time.Now()
	transformers, err := uc.transformerRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list transformers: %w", err)
	}
	nStart, nEnd, err := NormalizePeriod(start, end)
	if err != nil {
		return nil, err
	}

	result := &CalculateAllResult{PeriodStart: nStart, PeriodEnd: nEnd}
	var mu sync.Mutex
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	for _, t := range transformers {
		if !t.Active {
			continue
		}
		result.Total++
		wg.Add(1)
		sem <- struct{}{}
		go func(t *domain.Transformer) {
			defer wg.Done()
			defer func() { <-sem }()
			out, err := uc.CalculateBalance(ctx, CalculateBalanceInput{
				TransformerID: t.ID,
				PeriodStart:   start,
				PeriodEnd:     end,
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case errors.Is(err, ErrInsufficientData):
				result.Skipped++
			case err != nil:
				result.Failed++
				result.Errors = append(result.Errors, CalculateAllTransformerErr{TransformerID: t.ID, Code: t.Code, Error: err.Error()})
			default:
				result.Calculated++
				switch out.Balance.Status {
				case domain.BalanceStatusCritical:
					result.Critical++
				case domain.BalanceStatusWarning:
					result.Warning++
				default:
					result.Normal++
				}
			}
		}(t)
	}
	wg.Wait()
	result.DurationMS = time.Since(began).Milliseconds()

	slog.Info("tenant balance recalculated",
		"tenant_id", tenantID, "total", result.Total, "calculated", result.Calculated,
		"skipped", result.Skipped, "failed", result.Failed, "duration_ms", result.DurationMS)
	return result, nil
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

// CalculateTechnicalLoss calcula as perdas tecnicas do trafo no periodo
// conforme PRODIST M7, sem gravar resultado:
// PT_trafo = P0 x T + Pcc x (Ic/In)^2 x T
func (uc *BalanceUseCase) CalculateTechnicalLoss(ctx context.Context, input CalculateBalanceInput) (*domain.CalculationResult, error) {
	start, end, err := NormalizePeriod(input.PeriodStart, input.PeriodEnd)
	if err != nil {
		return nil, err
	}

	transformer, err := uc.transformerRepo.GetByID(ctx, input.TransformerID)
	if err != nil || transformer == nil {
		return nil, ErrTransformerNotFound
	}

	agg, err := uc.trafoReadingRepo.AggregateByTransformerAndPeriod(ctx, transformer.ID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate transformer readings: %w", err)
	}

	consumption, err := uc.ucReadingRepo.GetTotalByTransformerAndPeriod(ctx, transformer.ID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to sum UC readings: %w", err)
	}

	t := *transformer
	if t.LossLimitPct == nil {
		limit := t.LossLimitPctValue()
		t.LossLimitPct = &limit
	}

	result, err := calc.CalculateBalance(domain.CalculationInput{
		Transformer:    t,
		EnergyInjected: agg.EnergyKWh,
		UCConsumptions: []float64{consumption},
		PeriodHours:    calc.PeriodInHours(start, end),
		AverageCurrent: averageCurrent(t, agg),
	})
	if err != nil {
		return nil, err
	}
	result.TransformerID = transformer.ID
	result.TenantID = transformer.TenantID

	return &result, nil
}
