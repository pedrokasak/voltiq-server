package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

var (
	ErrTransformerCodeExists = errors.New("transformer code already exists")
	ErrPlanLimitReached      = errors.New("plan limit reached")
)

// PlanLimitError tells which quota of the plan was exceeded.
type PlanLimitError struct {
	Resource string `json:"resource"`
	Plan     string `json:"plan"`
	Limit    int    `json:"limit"`
	Current  int    `json:"current"`
}

func (e *PlanLimitError) Error() string {
	return fmt.Sprintf("plan %s allows %d %s (current: %d)", e.Plan, e.Limit, e.Resource, e.Current)
}

// Unwrap lets callers match ErrPlanLimitReached with errors.Is.
func (e *PlanLimitError) Unwrap() error { return ErrPlanLimitReached }

// TransformerUseCase handles transformer business logic
type TransformerUseCase struct {
	transformerRepo *repository.TransformerRepository
	tenantRepo      *repository.TenantRepository
	balanceRepo     *repository.BalanceRepository
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

// UpdateTransformerInput is a partial update: nil fields keep their value.
type UpdateTransformerInput struct {
	ID                domain.UUID
	Code              *string
	PowerKVA          *float64
	PrimaryVoltageKV  *float64
	SecondaryVoltageV *float64
	Lat               *float64
	Lng               *float64
	ClearLocation     bool
	CoreLossKW        *float64
	WindingLossKW     *float64
	LossLimitPct      *float64
	SubstationID      *domain.UUID
	ClearSubstation   bool
	Active            *bool
}

// NewTransformerUseCase creates a new TransformerUseCase. tenantRepo and
// balanceRepo are optional (plan limits and stats are skipped without them).
func NewTransformerUseCase(
	transformerRepo *repository.TransformerRepository,
	tenantRepo *repository.TenantRepository,
	balanceRepo *repository.BalanceRepository,
) *TransformerUseCase {
	return &TransformerUseCase{
		transformerRepo: transformerRepo,
		tenantRepo:      tenantRepo,
		balanceRepo:     balanceRepo,
	}
}

// CheckTransformerQuota verifica se o plano do tenant comporta mais n
// transformadores (regra do CLAUDE.md: limite antes de criar).
func (uc *TransformerUseCase) CheckTransformerQuota(ctx context.Context, tenantID domain.UUID, n int) error {
	if uc.tenantRepo == nil {
		return nil
	}
	tenant, err := uc.tenantRepo.GetByID(ctx, tenantID)
	if err != nil || tenant == nil {
		return fmt.Errorf("failed to load tenant: %w", err)
	}
	limits := domain.LimitsForPlan(tenant.Plan)
	if limits.MaxTransformers <= 0 {
		return nil
	}
	current, err := uc.transformerRepo.CountByTenant(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to count transformers: %w", err)
	}
	if !domain.AllowsMore(limits.MaxTransformers, current, n) {
		return &PlanLimitError{Resource: "transformers", Plan: string(tenant.Plan), Limit: limits.MaxTransformers, Current: current}
	}
	return nil
}

// CreateTransformer validates and creates a transformer
func (uc *TransformerUseCase) CreateTransformer(ctx context.Context, input CreateTransformerInput) (*domain.Transformer, error) {
	now := time.Now()
	transformer := &domain.Transformer{
		ID:                domain.UUID(uuid.New().String()),
		TenantID:          input.TenantID,
		Code:              strings.TrimSpace(input.Code),
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
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := domain.ValidateTransformer(*transformer); err != nil {
		return nil, err
	}

	if err := uc.CheckTransformerQuota(ctx, input.TenantID, 1); err != nil {
		return nil, err
	}

	if existing, err := uc.transformerRepo.GetByCode(ctx, input.TenantID, transformer.Code); err == nil && existing != nil {
		return nil, ErrTransformerCodeExists
	}

	if err := uc.transformerRepo.Create(ctx, transformer); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrTransformerCodeExists
		}
		return nil, fmt.Errorf("failed to create transformer: %w", err)
	}

	return transformer, nil
}

// GetTransformerByID returns a transformer by ID
func (uc *TransformerUseCase) GetTransformerByID(ctx context.Context, id domain.UUID) (*domain.Transformer, error) {
	transformer, err := uc.transformerRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrTransformerNotFound
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

// ListSummaries returns transformers with their latest balance, filtered and
// paginated.
func (uc *TransformerUseCase) ListSummaries(ctx context.Context, tenantID domain.UUID, f repository.TransformerFilter) ([]*repository.TransformerSummary, int, error) {
	items, total, err := uc.transformerRepo.ListSummaries(ctx, tenantID, f)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list transformers: %w", err)
	}
	if items == nil {
		items = []*repository.TransformerSummary{}
	}
	return items, total, nil
}

// ListBySubstation returns the transformers fed by a substation.
func (uc *TransformerUseCase) ListBySubstation(ctx context.Context, tenantID, substationID domain.UUID) ([]*domain.Transformer, error) {
	items, err := uc.transformerRepo.ListBySubstation(ctx, tenantID, substationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list transformers by substation: %w", err)
	}
	if items == nil {
		items = []*domain.Transformer{}
	}
	return items, nil
}

// CountTransformers returns how many transformers the tenant has.
func (uc *TransformerUseCase) CountTransformers(ctx context.Context, tenantID domain.UUID) (int, error) {
	return uc.transformerRepo.CountByTenant(ctx, tenantID)
}

// UpdateTransformer applies a partial update to a transformer
func (uc *TransformerUseCase) UpdateTransformer(ctx context.Context, input UpdateTransformerInput) (*domain.Transformer, error) {
	transformer, err := uc.transformerRepo.GetByID(ctx, input.ID)
	if err != nil || transformer == nil {
		return nil, ErrTransformerNotFound
	}

	if input.Code != nil {
		code := strings.TrimSpace(*input.Code)
		if code != transformer.Code {
			if existing, err := uc.transformerRepo.GetByCode(ctx, transformer.TenantID, code); err == nil && existing != nil {
				return nil, ErrTransformerCodeExists
			}
		}
		transformer.Code = code
	}
	if input.PowerKVA != nil {
		transformer.PowerKVA = *input.PowerKVA
	}
	if input.PrimaryVoltageKV != nil {
		transformer.PrimaryVoltageKV = *input.PrimaryVoltageKV
	}
	if input.SecondaryVoltageV != nil {
		transformer.SecondaryVoltageV = *input.SecondaryVoltageV
	}
	if input.ClearLocation {
		transformer.Lat, transformer.Lng = nil, nil
	} else {
		if input.Lat != nil {
			transformer.Lat = input.Lat
		}
		if input.Lng != nil {
			transformer.Lng = input.Lng
		}
	}
	if input.CoreLossKW != nil {
		transformer.CoreLossKW = input.CoreLossKW
	}
	if input.WindingLossKW != nil {
		transformer.WindingLossKW = input.WindingLossKW
	}
	if input.LossLimitPct != nil {
		transformer.LossLimitPct = input.LossLimitPct
	}
	if input.ClearSubstation {
		transformer.SubstationID = nil
	} else if input.SubstationID != nil {
		transformer.SubstationID = input.SubstationID
	}
	if input.Active != nil {
		transformer.Active = *input.Active
	}
	transformer.UpdatedAt = time.Now()

	if err := domain.ValidateTransformer(*transformer); err != nil {
		return nil, err
	}

	if err := uc.transformerRepo.Update(ctx, transformer); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrTransformerCodeExists
		}
		return nil, fmt.Errorf("failed to update transformer: %w", err)
	}

	return transformer, nil
}

// DeleteTransformer performs a soft delete on a transformer
// NOTE: This updates deleted_at, it does NOT remove the record from the database
// Following SDD 04-DESIGN-DETALHADO/01-modelo-de-dados.md convention
func (uc *TransformerUseCase) DeleteTransformer(ctx context.Context, id domain.UUID) error {
	err := uc.transformerRepo.Delete(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrTransformerNotFound
	}
	return err
}

// GetTransformerByCode returns a transformer by code
func (uc *TransformerUseCase) GetTransformerByCode(ctx context.Context, tenantID domain.UUID, code string) (*domain.Transformer, error) {
	t, err := uc.transformerRepo.GetByCode(ctx, tenantID, code)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTransformerNotFound
	}
	return t, nil
}

// TransformerExistsByCode checks if a transformer code exists for a tenant
func (uc *TransformerUseCase) TransformerExistsByCode(ctx context.Context, tenantID domain.UUID, code string) (bool, error) {
	t, err := uc.transformerRepo.GetByCode(ctx, tenantID, code)
	if err != nil {
		return false, err
	}
	return t != nil, nil
}

// TransformerStats contains transformer statistics
type TransformerStats struct {
	ID                    string   `json:"transformer_id"`
	Code                  string   `json:"code"`
	TotalEnergyKWh        float64  `json:"total_energy_kwh"`
	TotalConsumptionKWh   float64  `json:"total_consumption_kwh"`
	TotalLossKWh          float64  `json:"total_loss_kwh"`
	TotalTechnicalLossKWh float64  `json:"total_technical_loss_kwh"`
	AverageLossPct        float64  `json:"average_loss_pct"`
	MaxLossPct            float64  `json:"max_loss_pct"`
	MinLossPct            float64  `json:"min_loss_pct"`
	BalanceCount          int      `json:"balance_count"`
	CriticalCount         int      `json:"critical_count"`
	WarningCount          int      `json:"warning_count"`
	Status                string   `json:"status"`
	StatusUpdatedAt       string   `json:"status_updated_at"`
	LossLimitPct          float64  `json:"loss_limit_pct"`
	Trend                 string   `json:"trend"` // UP, DOWN, STABLE, UNKNOWN
	LastLossPct           *float64 `json:"last_loss_pct"`
}

// GetTransformerStats summarizes the balances of a transformer over the
// last 12 months.
func (uc *TransformerUseCase) GetTransformerStats(ctx context.Context, id domain.UUID) (*TransformerStats, error) {
	transformer, err := uc.transformerRepo.GetByID(ctx, id)
	if err != nil || transformer == nil {
		return nil, ErrTransformerNotFound
	}

	stats := &TransformerStats{
		ID:           string(transformer.ID),
		Code:         transformer.Code,
		Status:       "NO_DATA",
		Trend:        "UNKNOWN",
		LossLimitPct: transformer.LossLimitPctValue(),
	}
	if uc.balanceRepo == nil {
		return stats, nil
	}

	end := time.Now()
	balances, err := uc.balanceRepo.GetByTransformerAndPeriod(ctx, id, end.AddDate(-1, 0, 0), end)
	if err != nil {
		return nil, fmt.Errorf("failed to load balances: %w", err)
	}
	if len(balances) == 0 {
		return stats, nil
	}

	// balances vem do mais recente para o mais antigo
	stats.BalanceCount = len(balances)
	stats.MinLossPct = balances[0].LossPct
	var sumPct float64
	for _, b := range balances {
		stats.TotalEnergyKWh += b.EnergyInjectedKWh
		stats.TotalConsumptionKWh += b.TotalConsumptionKWh
		stats.TotalLossKWh += b.LossKWh
		if b.TechnicalLossKWh != nil {
			stats.TotalTechnicalLossKWh += *b.TechnicalLossKWh
		}
		sumPct += b.LossPct
		if b.LossPct > stats.MaxLossPct {
			stats.MaxLossPct = b.LossPct
		}
		if b.LossPct < stats.MinLossPct {
			stats.MinLossPct = b.LossPct
		}
		switch b.Status {
		case domain.BalanceStatusCritical:
			stats.CriticalCount++
		case domain.BalanceStatusWarning:
			stats.WarningCount++
		}
	}
	stats.AverageLossPct = round(sumPct/float64(len(balances)), 2)

	latest := balances[0]
	stats.Status = string(latest.Status)
	stats.StatusUpdatedAt = latest.CalculatedAt.Format(time.RFC3339)
	last := latest.LossPct
	stats.LastLossPct = &last

	if len(balances) >= 2 {
		diff := latest.LossPct - balances[1].LossPct
		switch {
		case diff > 0.5:
			stats.Trend = "UP"
		case diff < -0.5:
			stats.Trend = "DOWN"
		default:
			stats.Trend = "STABLE"
		}
	}

	return stats, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
