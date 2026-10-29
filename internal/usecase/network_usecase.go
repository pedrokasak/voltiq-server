package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// NetworkUseCase handles substations and manual readings (RF01/RF02.5).
type NetworkUseCase struct {
	substationRepo  *repository.SubstationRepository
	transformerRepo *repository.TransformerRepository
	ucRepo          *repository.ConsumingUnitRepository
	trafoReadings   *repository.TransformerReadingRepository
	ucReadings      *repository.UCReadingRepository
	balance         *BalanceUseCase
}

// NewNetworkUseCase creates a NetworkUseCase
func NewNetworkUseCase(
	substationRepo *repository.SubstationRepository,
	transformerRepo *repository.TransformerRepository,
	ucRepo *repository.ConsumingUnitRepository,
	trafoReadings *repository.TransformerReadingRepository,
	ucReadings *repository.UCReadingRepository,
	balance *BalanceUseCase,
) *NetworkUseCase {
	return &NetworkUseCase{substationRepo: substationRepo, transformerRepo: transformerRepo, ucRepo: ucRepo,
		trafoReadings: trafoReadings, ucReadings: ucReadings, balance: balance}
}

// SubstationInput creates or updates a substation.
type SubstationInput struct {
	Code string
	Name string
	Lat  *float64
	Lng  *float64
}

func (in *SubstationInput) validate() error {
	in.Code, in.Name = strings.TrimSpace(in.Code), strings.TrimSpace(in.Name)
	v := &domain.ValidationError{}
	if in.Code == "" || len(in.Code) > 50 {
		v.Fields = append(v.Fields, domain.FieldError{Field: "code", Message: "código é obrigatório (até 50 caracteres)"})
	}
	if in.Name == "" {
		in.Name = in.Code
	}
	if in.Lat != nil && (*in.Lat < -90 || *in.Lat > 90) {
		v.Fields = append(v.Fields, domain.FieldError{Field: "lat", Message: "latitude deve estar entre -90 e 90"})
	}
	if in.Lng != nil && (*in.Lng < -180 || *in.Lng > 180) {
		v.Fields = append(v.Fields, domain.FieldError{Field: "lng", Message: "longitude deve estar entre -180 e 180"})
	}
	if (in.Lat == nil) != (in.Lng == nil) {
		v.Fields = append(v.Fields, domain.FieldError{Field: "lat", Message: "informe latitude e longitude juntas"})
	}
	if len(v.Fields) > 0 {
		return v
	}
	return nil
}

// ListSubstations lists substations with transformer totals.
func (uc *NetworkUseCase) ListSubstations(ctx context.Context, tenantID domain.UUID) ([]*repository.SubstationSummary, error) {
	items, err := uc.substationRepo.List(ctx, tenantID)
	if items == nil && err == nil {
		items = []*repository.SubstationSummary{}
	}
	return items, err
}

// GetSubstation returns one substation.
func (uc *NetworkUseCase) GetSubstation(ctx context.Context, id domain.UUID) (*domain.Substation, error) {
	s, err := uc.substationRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, ErrNotFound
	}
	return s, nil
}

// CreateSubstation creates a substation.
func (uc *NetworkUseCase) CreateSubstation(ctx context.Context, tenantID domain.UUID, in SubstationInput) (*domain.Substation, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	now := time.Now()
	s := &domain.Substation{ID: domain.UUID(uuid.New().String()), TenantID: tenantID, Code: in.Code, Name: in.Name,
		Lat: in.Lat, Lng: in.Lng, CreatedAt: now, UpdatedAt: now}
	if err := uc.substationRepo.Create(ctx, s); err != nil {
		if isUniqueViolation(err) {
			return nil, conflict("já existe uma subestação com este código")
		}
		return nil, err
	}
	return s, nil
}

// UpdateSubstation changes a substation.
func (uc *NetworkUseCase) UpdateSubstation(ctx context.Context, id domain.UUID, in SubstationInput) (*domain.Substation, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	s, err := uc.GetSubstation(ctx, id)
	if err != nil {
		return nil, err
	}
	s.Code, s.Name, s.Lat, s.Lng = in.Code, in.Name, in.Lat, in.Lng
	if err := uc.substationRepo.Update(ctx, s); err != nil {
		if isUniqueViolation(err) {
			return nil, conflict("já existe uma subestação com este código")
		}
		return nil, err
	}
	s.UpdatedAt = time.Now()
	return s, nil
}

// DeleteSubstation soft-deletes a substation (transformers are unlinked).
func (uc *NetworkUseCase) DeleteSubstation(ctx context.Context, id domain.UUID) error {
	return uc.substationRepo.Delete(ctx, id)
}

// ManualReadingInput is a reading typed by the user (RF02.5).
type ManualReadingInput struct {
	ReadingAt   time.Time
	EnergyKWh   float64
	DemandKW    *float64
	PowerFactor *float64
	Recalculate bool
}

// ManualReadingResult is the stored reading and the optional recalculation.
type ManualReadingResult struct {
	Reading any                        `json:"reading"`
	Balance *domain.TransformerBalance `json:"balance,omitempty"`
	Message string                     `json:"message,omitempty"`
}

func validateManual(in ManualReadingInput, energyField string) error {
	v := &domain.ValidationError{}
	if in.ReadingAt.IsZero() {
		v.Fields = append(v.Fields, domain.FieldError{Field: "reading_at", Message: "data da leitura é obrigatória"})
	} else if in.ReadingAt.After(time.Now().Add(time.Hour)) {
		v.Fields = append(v.Fields, domain.FieldError{Field: "reading_at", Message: "a leitura não pode estar no futuro"})
	}
	if in.EnergyKWh < 0 || in.EnergyKWh > 1e9 {
		v.Fields = append(v.Fields, domain.FieldError{Field: energyField, Message: "energia deve ser zero ou positiva"})
	}
	if in.DemandKW != nil && *in.DemandKW < 0 {
		v.Fields = append(v.Fields, domain.FieldError{Field: "demand_kw", Message: "demanda deve ser zero ou positiva"})
	}
	if in.PowerFactor != nil && (*in.PowerFactor <= 0 || *in.PowerFactor > 1) {
		v.Fields = append(v.Fields, domain.FieldError{Field: "power_factor", Message: "fator de potência deve estar entre 0 e 1"})
	}
	if len(v.Fields) > 0 {
		return v
	}
	return nil
}

func (uc *NetworkUseCase) recalcMonth(ctx context.Context, transformerID domain.UUID, at time.Time, res *ManualReadingResult) {
	if uc.balance == nil {
		return
	}
	start := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, at.Location())
	out, err := uc.balance.CalculateBalance(ctx, CalculateBalanceInput{
		TransformerID: transformerID, PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0).Add(-time.Nanosecond),
	})
	if err != nil {
		res.Message = "leitura registrada; o balanço do mês ainda não pôde ser calculado (faltam leituras)"
		return
	}
	res.Balance = out.Balance
	res.Message = "leitura registrada e balanço do mês recalculado"
}

// AddTransformerReading stores a manual transformer reading.
func (uc *NetworkUseCase) AddTransformerReading(ctx context.Context, tenantID, transformerID domain.UUID, in ManualReadingInput) (*ManualReadingResult, error) {
	t, err := uc.transformerRepo.GetByID(ctx, transformerID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTransformerNotFound
	}
	if err := validateManual(in, "energy_kwh"); err != nil {
		return nil, err
	}
	keys, err := uc.trafoReadings.ExistingKeys(ctx, tenantID, []domain.UUID{transformerID}, in.ReadingAt, in.ReadingAt)
	if err != nil {
		return nil, err
	}
	if len(keys) > 0 {
		return nil, conflict("já existe uma leitura deste transformador neste horário")
	}
	r := &domain.TransformerReading{
		ID: domain.UUID(uuid.New().String()), TenantID: tenantID, TransformerID: transformerID, ReadingAt: in.ReadingAt,
		EnergyKWh: in.EnergyKWh, DemandKW: in.DemandKW, PowerFactor: in.PowerFactor, CreatedAt: time.Now(),
	}
	if err := uc.trafoReadings.Create(ctx, r); err != nil {
		return nil, err
	}
	res := &ManualReadingResult{Reading: r, Message: "leitura registrada"}
	if in.Recalculate {
		uc.recalcMonth(ctx, transformerID, in.ReadingAt, res)
	}
	return res, nil
}

// AddUCReading stores a manual UC consumption reading.
func (uc *NetworkUseCase) AddUCReading(ctx context.Context, tenantID, ucID domain.UUID, in ManualReadingInput) (*ManualReadingResult, error) {
	unit, err := uc.ucRepo.GetByID(ctx, ucID)
	if err != nil {
		return nil, err
	}
	if unit == nil {
		return nil, ErrConsumingUnitNotFound
	}
	if err := validateManual(in, "consumption_kwh"); err != nil {
		return nil, err
	}
	keys, err := uc.ucReadings.ExistingKeys(ctx, tenantID, []domain.UUID{ucID}, in.ReadingAt, in.ReadingAt)
	if err != nil {
		return nil, err
	}
	if len(keys) > 0 {
		return nil, conflict("já existe uma leitura desta unidade neste horário")
	}
	r := &domain.UCReading{
		ID: domain.UUID(uuid.New().String()), TenantID: tenantID, UCID: ucID, TransformerID: unit.TransformerID,
		ReadingAt: in.ReadingAt, ConsumptionKWh: in.EnergyKWh, CreatedAt: time.Now(),
	}
	if err := uc.ucReadings.Create(ctx, r); err != nil {
		return nil, err
	}
	res := &ManualReadingResult{Reading: r, Message: "leitura registrada"}
	if in.Recalculate {
		uc.recalcMonth(ctx, unit.TransformerID, in.ReadingAt, res)
	}
	return res, nil
}
