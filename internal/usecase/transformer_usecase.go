package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

var (
	ErrTransformerCodeExists = errors.New("transformer code already exists")
)

// TransformerUseCase handles transformer business logic
type TransformerUseCase struct {
	transformerRepo *repository.TransformerRepository
}

// CreateTransformerInput contains data to create a transformer
type CreateTransformerInput struct {
	TenantID          domain.UUID
	Code              string
	PowerKVA          float64
	PrimaryVoltageKV  float64
	SecondaryVoltageV float64
	Lat               *float64
	Lng               *float64
	CoreLossKW        *float64
	WindingLossKW     *float64
	LossLimitPct      *float64
	SubstationID      *domain.UUID
}

// UpdateTransformerInput contains data to update a transformer
type UpdateTransformerInput struct {
	ID                domain.UUID
	Code              string
	PowerKVA          float64
	PrimaryVoltageKV  float64
	SecondaryVoltageV float64
	Lat               *float64
	Lng               *float64
	CoreLossKW        *float64
	WindingLossKW     *float64
	LossLimitPct      *float64
	SubstationID      *domain.UUID
}

// NewTransformerUseCase creates a new TransformerUseCase
func NewTransformerUseCase(transformerRepo *repository.TransformerRepository) *TransformerUseCase {
	return &TransformerUseCase{
		transformerRepo: transformerRepo,
	}
}

// CreateTransformer creates a new transformer
func (uc *TransformerUseCase) CreateTransformer(ctx context.Context, input CreateTransformerInput) (*domain.Transformer, error) {
	transformer := &domain.Transformer{
		ID:                domain.UUID(time.Now().Format("20060102150405") + input.Code),
		TenantID:          input.TenantID,
		Code:              input.Code,
		PowerKVA:          input.PowerKVA,
		PrimaryVoltageKV:  input.PrimaryVoltageKV,
		SecondaryVoltageV: input.SecondaryVoltageV,
		Lat:               input.Lat,
		Lng:               input.Lng,
		CoreLossKW:        input.CoreLossKW,
		WindingLossKW:     input.WindingLossKW,
		LossLimitPct:      input.LossLimitPct,
		SubstationID:      input.SubstationID,
		Active:            true,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := uc.transformerRepo.Create(ctx, transformer); err != nil {
		return nil, errors.New("failed to create transformer")
	}

	return transformer, nil
}

// GetTransformerByID returns a transformer by ID
func (uc *TransformerUseCase) GetTransformerByID(ctx context.Context, id domain.UUID) (*domain.Transformer, error) {
	transformer, err := uc.transformerRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if transformer == nil {
		return nil, ErrTransformerNotFound
	}

	return transformer, nil
}

// ListTransformers returns all transformers for a tenant
func (uc *TransformerUseCase) ListTransformers(ctx context.Context, tenantID domain.UUID) ([]*domain.Transformer, error) {
	transformers, err := uc.transformerRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, errors.New("failed to list transformers")
	}

	return transformers, nil
}

// UpdateTransformer updates an existing transformer
func (uc *TransformerUseCase) UpdateTransformer(ctx context.Context, input UpdateTransformerInput) (*domain.Transformer, error) {
	transformer, err := uc.transformerRepo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	if transformer == nil {
		return nil, ErrTransformerNotFound
	}

	transformer.Code = input.Code
	transformer.PowerKVA = input.PowerKVA
	transformer.PrimaryVoltageKV = input.PrimaryVoltageKV
	transformer.SecondaryVoltageV = input.SecondaryVoltageV
	transformer.Lat = input.Lat
	transformer.Lng = input.Lng
	transformer.CoreLossKW = input.CoreLossKW
	transformer.WindingLossKW = input.WindingLossKW
	transformer.LossLimitPct = input.LossLimitPct
	transformer.SubstationID = input.SubstationID
	transformer.UpdatedAt = time.Now()

	if err := uc.transformerRepo.Update(ctx, transformer); err != nil {
		return nil, errors.New("failed to update transformer")
	}

	return transformer, nil
}

// DeleteTransformer performs a soft delete on a transformer
// NOTE: This updates deleted_at, it does NOT remove the record from the database
// Following SDD 04-DESIGN-DETALHADO/01-modelo-de-dados.md convention
func (uc *TransformerUseCase) DeleteTransformer(ctx context.Context, id domain.UUID) error {
	return uc.transformerRepo.Delete(ctx, id)
}

// GetTransformerByCode returns a transformer by code
func (uc *TransformerUseCase) GetTransformerByCode(ctx context.Context, tenantID domain.UUID, code string) (*domain.Transformer, error) {
	transformers, err := uc.transformerRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	for _, t := range transformers {
		if t.Code == code {
			return t, nil
		}
	}

	return nil, ErrTransformerNotFound
}

// TransformerExistsByCode checks if a transformer code exists for a tenant
func (uc *TransformerUseCase) TransformerExistsByCode(ctx context.Context, tenantID domain.UUID, code string) (bool, error) {
	transformers, err := uc.transformerRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return false, err
	}

	for _, t := range transformers {
		if t.Code == code {
			return true, nil
		}
	}

	return false, nil
}

// TransformerStats contains transformer statistics
type TransformerStats struct {
	ID              string  `json:"transformer_id"`
	Code            string  `json:"code"`
	TotalEnergyKWh  float64 `json:"total_energy_kwh"`
	AverageLossPct  float64 `json:"average_loss_pct"`
	MaxLossPct      float64 `json:"max_loss_pct"`
	BalanceCount    int     `json:"balance_count"`
	Status          string  `json:"status"`
	StatusUpdatedAt string  `json:"status_updated_at"`
}

// GetTransformerStats returns statistics for a transformer
func (uc *TransformerUseCase) GetTransformerStats(ctx context.Context, id domain.UUID) (*TransformerStats, error) {
	transformer, err := uc.transformerRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if transformer == nil {
		return nil, ErrTransformerNotFound
	}

	// Get latest balance to determine status
	balanceRepo := repository.NewBalanceRepository(nil)
	latestBalance, _ := balanceRepo.GetLatestByTransformer(ctx, id)

	status := "NORMAL"
	statusUpdatedAt := ""
	if latestBalance != nil {
		switch latestBalance.Status {
		case domain.BalanceStatusWarning:
			status = "ATENCAO"
		case domain.BalanceStatusCritical:
			status = "CRITICO"
		default:
			status = "NORMAL"
		}
		statusUpdatedAt = latestBalance.CalculatedAt.Format(time.RFC3339)
	}

	return &TransformerStats{
		ID:              string(transformer.ID),
		Code:            transformer.Code,
		TotalEnergyKWh:  0,
		AverageLossPct:  0,
		MaxLossPct:      0,
		BalanceCount:    0,
		Status:          status,
		StatusUpdatedAt: statusUpdatedAt,
	}, nil
}
