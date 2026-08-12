package usecase

import (
	"context"
	"time"

	"github.com/voltiq/server/internal/calc"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// RiskUseCase handles risk score and anomaly detection business logic
type RiskUseCase struct {
	balanceRepo      *repository.BalanceRepository
	transformerRepo  *repository.TransformerRepository
	ucRepo           *repository.ConsumingUnitRepository
}

// RiskScoreOutput contains the calculated risk score
type RiskScoreOutput struct {
	RiskScore calc.RiskScore
}

// AnomalyOutput contains detected anomalies
type AnomalyOutput struct {
	Anomalies []calc.Anomaly
}

// NewRiskUseCase creates a new RiskUseCase
func NewRiskUseCase(
	balanceRepo *repository.BalanceRepository,
	transformerRepo *repository.TransformerRepository,
	ucRepo *repository.ConsumingUnitRepository,
) *RiskUseCase {
	return &RiskUseCase{
		balanceRepo:      balanceRepo,
		transformerRepo:  transformerRepo,
		ucRepo:           ucRepo,
	}
}

// GetRiskScore calculates and returns the risk score for a transformer
func (uc *RiskUseCase) GetRiskScore(ctx context.Context, transformerID domain.UUID) (*RiskScoreOutput, error) {
	transformer, err := uc.transformerRepo.GetByID(ctx, transformerID)
	if err != nil {
		return nil, err
	}
	if transformer == nil {
		return nil, ErrTransformerNotFound
	}

	// Get latest balance
	latestBalance, err := uc.balanceRepo.GetLatestByTransformer(ctx, transformerID)
	if err != nil {
		return nil, err
	}
	if latestBalance == nil {
		return nil, ErrBalanceNotFound
	}

	// Get historical balances (last 6 periods)
	sixMonthsAgo := time.Now().AddDate(0, -6, 0)
	historicalBalances, err := uc.balanceRepo.GetByTransformerAndPeriod(ctx, transformerID, sixMonthsAgo, time.Now())
	if err != nil {
		return nil, err
	}

	// Get UC count
	ucs, err := uc.ucRepo.GetByTransformer(ctx, transformerID)
	if err != nil {
		return nil, err
	}

	// Calculate risk score
	riskScore := calc.CalculateRiskScore(latestBalance, historicalBalances, transformer, len(ucs))

	return &RiskScoreOutput{
		RiskScore: riskScore,
	}, nil
}

// GetAnomalies detects and returns anomalies for a transformer
func (uc *RiskUseCase) GetAnomalies(ctx context.Context, transformerID domain.UUID, monthsBack int) (*AnomalyOutput, error) {
	transformer, err := uc.transformerRepo.GetByID(ctx, transformerID)
	if err != nil {
		return nil, err
	}
	if transformer == nil {
		return nil, ErrTransformerNotFound
	}

	// Get historical balances
	startDate := time.Now().AddDate(0, -monthsBack, 0)
	balances, err := uc.balanceRepo.GetByTransformerAndPeriod(ctx, transformerID, startDate, time.Now())
	if err != nil {
		return nil, err
	}

	if len(balances) == 0 {
		return &AnomalyOutput{Anomalies: []calc.Anomaly{}}, nil
	}

	// Detect anomalies
	anomalies := calc.DetectAnomalies(transformerID, balances, transformer)

	return &AnomalyOutput{
		Anomalies: anomalies,
	}, nil
}

// GetAllRiskScores returns risk scores for all transformers of a tenant
func (uc *RiskUseCase) GetAllRiskScores(ctx context.Context, tenantID domain.UUID) ([]*calc.RiskScore, error) {
	transformers, err := uc.transformerRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	riskScores := make([]*calc.RiskScore, 0, len(transformers))

	for _, t := range transformers {
		latestBalance, err := uc.balanceRepo.GetLatestByTransformer(ctx, t.ID)
		if err != nil || latestBalance == nil {
			// Skip transformers without balance data
			continue
		}

		// Get historical balances (last 6 periods)
		sixMonthsAgo := time.Now().AddDate(0, -6, 0)
		historicalBalances, err := uc.balanceRepo.GetByTransformerAndPeriod(ctx, t.ID, sixMonthsAgo, time.Now())
		if err != nil {
			continue
		}

		// Get UC count
		ucs, err := uc.ucRepo.GetByTransformer(ctx, t.ID)
		if err != nil {
			continue
		}

		riskScore := calc.CalculateRiskScore(latestBalance, historicalBalances, t, len(ucs))
		riskScores = append(riskScores, &riskScore)
	}

	return riskScores, nil
}

// GetAllAnomalies returns anomalies for all transformers of a tenant
func (uc *RiskUseCase) GetAllAnomalies(ctx context.Context, tenantID domain.UUID, monthsBack int) (map[domain.UUID][]calc.Anomaly, error) {
	transformers, err := uc.transformerRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	allAnomalies := make(map[domain.UUID][]calc.Anomaly)
	startDate := time.Now().AddDate(0, -monthsBack, 0)

	for _, t := range transformers {
		balances, err := uc.balanceRepo.GetByTransformerAndPeriod(ctx, t.ID, startDate, time.Now())
		if err != nil || len(balances) == 0 {
			continue
		}

		anomalies := calc.DetectAnomalies(t.ID, balances, t)
		if len(anomalies) > 0 {
			allAnomalies[t.ID] = anomalies
		}
	}

	return allAnomalies, nil
}