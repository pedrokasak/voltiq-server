package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

var (
	ErrConsumingUnitNotFound = errors.New("consuming unit not found")
	ErrUCCodeExists          = errors.New("UC code already exists")
)

// ConsumingUnitUseCase handles consuming unit business logic
type ConsumingUnitUseCase struct {
	ucRepo *repository.ConsumingUnitRepository
}

// CreateConsumingUnitInput contains data to create a consuming unit
type CreateConsumingUnitInput struct {
	TenantID      domain.UUID
	TransformerID domain.UUID
	UCCode        string
	Name          string
	Class         domain.UCClass
	Active        bool
}

// UpdateConsumingUnitInput contains data to update a consuming unit
type UpdateConsumingUnitInput struct {
	ID            domain.UUID
	TransformerID domain.UUID
	UCCode        string
	Name          string
	Class         domain.UCClass
	Active        bool
}

// NewConsumingUnitUseCase creates a new ConsumingUnitUseCase
func NewConsumingUnitUseCase(ucRepo *repository.ConsumingUnitRepository) *ConsumingUnitUseCase {
	return &ConsumingUnitUseCase{
		ucRepo: ucRepo,
	}
}

// CreateConsumingUnit creates a new consuming unit
func (uc *ConsumingUnitUseCase) CreateConsumingUnit(ctx context.Context, input CreateConsumingUnitInput) (*domain.ConsumingUnit, error) {
	now := time.Now()

	consumingUnit := &domain.ConsumingUnit{
		ID:            domain.UUID(time.Now().Format("20060102150405") + input.UCCode),
		TenantID:      input.TenantID,
		TransformerID: input.TransformerID,
		UCCode:        input.UCCode,
		Name:          input.Name,
		Class:         &input.Class,
		Active:        input.Active,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := uc.ucRepo.Create(ctx, consumingUnit); err != nil {
		return nil, errors.New("failed to create consuming unit")
	}

	return consumingUnit, nil
}

// GetConsumingUnitByID returns a consuming unit by ID
func (uc *ConsumingUnitUseCase) GetConsumingUnitByID(ctx context.Context, id domain.UUID) (*domain.ConsumingUnit, error) {
	unit, err := uc.ucRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if unit == nil {
		return nil, ErrConsumingUnitNotFound
	}

	return unit, nil
}

// ListConsumingUnits returns all consuming units for a tenant
func (uc *ConsumingUnitUseCase) ListConsumingUnits(ctx context.Context, tenantID domain.UUID) ([]*domain.ConsumingUnit, error) {
	units, err := uc.ucRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, errors.New("failed to list consuming units")
	}

	return units, nil
}

// ListConsumingUnitsByTransformer returns consuming units linked to a transformer
func (uc *ConsumingUnitUseCase) ListConsumingUnitsByTransformer(ctx context.Context, transformerID domain.UUID) ([]*domain.ConsumingUnit, error) {
	units, err := uc.ucRepo.GetByTransformer(ctx, transformerID)
	if err != nil {
		return nil, errors.New("failed to list consuming units by transformer")
	}

	return units, nil
}

// UpdateConsumingUnit updates an existing consuming unit
func (uc *ConsumingUnitUseCase) UpdateConsumingUnit(ctx context.Context, input UpdateConsumingUnitInput) (*domain.ConsumingUnit, error) {
	unit, err := uc.ucRepo.GetByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	if unit == nil {
		return nil, ErrConsumingUnitNotFound
	}

	unit.TransformerID = input.TransformerID
	unit.UCCode = input.UCCode
	unit.Name = input.Name
	unit.Class = &input.Class
	unit.Active = input.Active
	unit.UpdatedAt = time.Now()

	if err := uc.ucRepo.Update(ctx, unit); err != nil {
		return nil, errors.New("failed to update consuming unit")
	}

	return unit, nil
}

// DeleteConsumingUnit performs a soft delete on a consuming unit
// NOTE: This updates deleted_at, it does NOT remove the record from the database
// Following SDD 04-DESIGN-DETALHADO/01-modelo-de-dados.md convention
func (uc *ConsumingUnitUseCase) DeleteConsumingUnit(ctx context.Context, id domain.UUID) error {
	return uc.ucRepo.Delete(ctx, id)
}
