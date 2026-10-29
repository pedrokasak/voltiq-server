package usecase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

var (
	ErrConsumingUnitNotFound = errors.New("consuming unit not found")
	ErrUCCodeExists          = errors.New("UC code already exists")
)

// ConsumingUnitUseCase handles consuming unit business logic
type ConsumingUnitUseCase struct {
	ucRepo          *repository.ConsumingUnitRepository
	transformerRepo *repository.TransformerRepository
	readingRepo     *repository.UCReadingRepository
}

// CreateConsumingUnitInput contains data to create a consuming unit
type CreateConsumingUnitInput struct {
	TenantID      domain.UUID
	TransformerID domain.UUID
	UCCode        string
	Name          string
	Class         *domain.UCClass
	Active        *bool
}

// UpdateConsumingUnitInput is a partial update: nil fields keep their value.
type UpdateConsumingUnitInput struct {
	ID            domain.UUID
	TransformerID *domain.UUID
	UCCode        *string
	Name          *string
	Class         *domain.UCClass
	Active        *bool
}

// NewConsumingUnitUseCase creates a new ConsumingUnitUseCase
func NewConsumingUnitUseCase(
	ucRepo *repository.ConsumingUnitRepository,
	transformerRepo *repository.TransformerRepository,
	readingRepo *repository.UCReadingRepository,
) *ConsumingUnitUseCase {
	return &ConsumingUnitUseCase{ucRepo: ucRepo, transformerRepo: transformerRepo, readingRepo: readingRepo}
}

// ensureTransformer checks the transformer exists in the request tenant
// (RF01.2: vincular UC a um trafo do proprio tenant).
func (uc *ConsumingUnitUseCase) ensureTransformer(ctx context.Context, id domain.UUID) error {
	if uc.transformerRepo == nil {
		return nil
	}
	t, err := uc.transformerRepo.GetByID(ctx, id)
	if err != nil || t == nil {
		return &domain.ValidationError{Fields: []domain.FieldError{{Field: "transformer_id", Message: "transformador não encontrado"}}}
	}
	return nil
}

// CreateConsumingUnit creates a new consuming unit
func (uc *ConsumingUnitUseCase) CreateConsumingUnit(ctx context.Context, input CreateConsumingUnitInput) (*domain.ConsumingUnit, error) {
	now := time.Now()
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	unit := &domain.ConsumingUnit{
		ID:            domain.UUID(uuid.New().String()),
		TenantID:      input.TenantID,
		TransformerID: input.TransformerID,
		UCCode:        strings.TrimSpace(input.UCCode),
		Name:          strings.TrimSpace(input.Name),
		Class:         input.Class,
		Active:        active,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := domain.ValidateConsumingUnit(*unit); err != nil {
		return nil, err
	}
	if err := uc.ensureTransformer(ctx, unit.TransformerID); err != nil {
		return nil, err
	}
	if existing, err := uc.ucRepo.GetByCode(ctx, unit.TenantID, unit.UCCode); err == nil && existing != nil {
		return nil, ErrUCCodeExists
	}

	if err := uc.ucRepo.Create(ctx, unit); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUCCodeExists
		}
		return nil, fmt.Errorf("failed to create consuming unit: %w", err)
	}

	return unit, nil
}

// GetConsumingUnitByID returns a consuming unit by ID
func (uc *ConsumingUnitUseCase) GetConsumingUnitByID(ctx context.Context, id domain.UUID) (*domain.ConsumingUnit, error) {
	unit, err := uc.ucRepo.GetByID(ctx, id)
	if err != nil || unit == nil {
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
	if units == nil {
		units = []*domain.ConsumingUnit{}
	}
	return units, nil
}

// ListConsumingUnitsByTransformer returns consuming units linked to a transformer
func (uc *ConsumingUnitUseCase) ListConsumingUnitsByTransformer(ctx context.Context, transformerID domain.UUID) ([]*domain.ConsumingUnit, error) {
	units, err := uc.ucRepo.GetByTransformer(ctx, transformerID)
	if err != nil {
		return nil, errors.New("failed to list consuming units by transformer")
	}
	if units == nil {
		units = []*domain.ConsumingUnit{}
	}
	return units, nil
}

// UpdateConsumingUnit applies a partial update to a consuming unit
func (uc *ConsumingUnitUseCase) UpdateConsumingUnit(ctx context.Context, input UpdateConsumingUnitInput) (*domain.ConsumingUnit, error) {
	unit, err := uc.ucRepo.GetByID(ctx, input.ID)
	if err != nil || unit == nil {
		return nil, ErrConsumingUnitNotFound
	}

	if input.TransformerID != nil && *input.TransformerID != unit.TransformerID {
		if err := uc.ensureTransformer(ctx, *input.TransformerID); err != nil {
			return nil, err
		}
		unit.TransformerID = *input.TransformerID
	}
	if input.UCCode != nil {
		code := strings.TrimSpace(*input.UCCode)
		if code != unit.UCCode {
			if existing, err := uc.ucRepo.GetByCode(ctx, unit.TenantID, code); err == nil && existing != nil {
				return nil, ErrUCCodeExists
			}
		}
		unit.UCCode = code
	}
	if input.Name != nil {
		unit.Name = strings.TrimSpace(*input.Name)
	}
	if input.Class != nil {
		unit.Class = input.Class
	}
	if input.Active != nil {
		unit.Active = *input.Active
	}
	unit.UpdatedAt = time.Now()

	if err := domain.ValidateConsumingUnit(*unit); err != nil {
		return nil, err
	}
	if err := uc.ucRepo.Update(ctx, unit); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrUCCodeExists
		}
		return nil, fmt.Errorf("failed to update consuming unit: %w", err)
	}

	return unit, nil
}

// DeleteConsumingUnit performs a soft delete on a consuming unit
// NOTE: This updates deleted_at, it does NOT remove the record from the database
// Following SDD 04-DESIGN-DETALHADO/01-modelo-de-dados.md convention
func (uc *ConsumingUnitUseCase) DeleteConsumingUnit(ctx context.Context, id domain.UUID) error {
	err := uc.ucRepo.Delete(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrConsumingUnitNotFound
	}
	return err
}

// ConsumptionPoint is the consumption of a UC in a month.
type ConsumptionPoint struct {
	Month          string   `json:"month"` // YYYY-MM
	ConsumptionKWh float64  `json:"consumption_kwh"`
	Readings       int      `json:"readings"`
	VariationPct   *float64 `json:"variation_pct"` // vs previous month
}

// ConsumptionHistory summarizes the consumption of a UC.
type ConsumptionHistory struct {
	Unit            *domain.ConsumingUnit `json:"unit"`
	TransformerCode string                `json:"transformer_code"`
	Months          []ConsumptionPoint    `json:"months"`
	TotalKWh        float64               `json:"total_kwh"`
	AverageKWh      float64               `json:"average_kwh"`
	MaxKWh          float64               `json:"max_kwh"`
	MinKWh          float64               `json:"min_kwh"`
	// Anomalies lists months whose consumption dropped more than 50% versus
	// the average of the previous three (indicio de perda nao tecnica).
	Anomalies []string `json:"anomalies"`
}

// GetConsumptionHistory returns the monthly consumption of a UC.
func (uc *ConsumingUnitUseCase) GetConsumptionHistory(ctx context.Context, id domain.UUID, months int) (*ConsumptionHistory, error) {
	unit, err := uc.ucRepo.GetByID(ctx, id)
	if err != nil || unit == nil {
		return nil, ErrConsumingUnitNotFound
	}
	if months <= 0 || months > 60 {
		months = 12
	}
	h := &ConsumptionHistory{Unit: unit, Months: []ConsumptionPoint{}, Anomalies: []string{}}
	if t, err := uc.transformerRepo.GetByID(ctx, unit.TransformerID); err == nil && t != nil {
		h.TransformerCode = t.Code
	}
	if uc.readingRepo == nil {
		return h, nil
	}

	end := time.Now()
	start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location()).AddDate(0, -(months - 1), 0)
	readings, err := uc.readingRepo.GetByUCAndPeriod(ctx, id, start, end)
	if err != nil {
		return nil, fmt.Errorf("load readings: %w", err)
	}

	byMonth := map[string]*ConsumptionPoint{}
	order := []string{}
	for m := start; !m.After(end); m = m.AddDate(0, 1, 0) {
		key := m.Format("2006-01")
		byMonth[key] = &ConsumptionPoint{Month: key}
		order = append(order, key)
	}
	for _, r := range readings {
		if p, ok := byMonth[r.ReadingAt.Format("2006-01")]; ok {
			p.ConsumptionKWh += r.ConsumptionKWh
			p.Readings++
		}
	}

	var withData []float64
	for i, key := range order {
		p := byMonth[key]
		if i > 0 {
			prev := byMonth[order[i-1]]
			if prev.ConsumptionKWh > 0 && p.Readings > 0 {
				v := round((p.ConsumptionKWh-prev.ConsumptionKWh)/prev.ConsumptionKWh*100, 2)
				p.VariationPct = &v
			}
		}
		if p.Readings > 0 {
			if len(withData) >= 3 {
				avg := (withData[len(withData)-1] + withData[len(withData)-2] + withData[len(withData)-3]) / 3
				if avg > 0 && p.ConsumptionKWh < avg*0.5 {
					h.Anomalies = append(h.Anomalies, key)
				}
			}
			withData = append(withData, p.ConsumptionKWh)
			h.TotalKWh += p.ConsumptionKWh
		}
		h.Months = append(h.Months, *p)
	}
	if len(withData) > 0 {
		h.AverageKWh = round(h.TotalKWh/float64(len(withData)), 2)
		h.MaxKWh, h.MinKWh = withData[0], withData[0]
		for _, v := range withData {
			h.MaxKWh = math.Max(h.MaxKWh, v)
			h.MinKWh = math.Min(h.MinKWh, v)
		}
	}
	return h, nil
}
