package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/ingestion"
	"github.com/voltiq/server/internal/repository"
)

// RegistryKind selects which registry a file feeds.
type RegistryKind string

const (
	RegistryTransformers   RegistryKind = "transformers"
	RegistryConsumingUnits RegistryKind = "consuming_units"
)

// RegistryImportUseCase importa o cadastro em massa de transformadores e
// UCs (Sprint 3: importacao de base de transformadores e validacao).
type RegistryImportUseCase struct {
	db              *repository.Database
	transformerRepo *repository.TransformerRepository
	ucRepo          *repository.ConsumingUnitRepository
	substationRepo  *repository.SubstationRepository
	transformers    *TransformerUseCase
}

// NewRegistryImportUseCase creates a RegistryImportUseCase
func NewRegistryImportUseCase(
	db *repository.Database,
	transformerRepo *repository.TransformerRepository,
	ucRepo *repository.ConsumingUnitRepository,
	substationRepo *repository.SubstationRepository,
	transformers *TransformerUseCase,
) *RegistryImportUseCase {
	return &RegistryImportUseCase{
		db:              db,
		transformerRepo: transformerRepo,
		ucRepo:          ucRepo,
		substationRepo:  substationRepo,
		transformers:    transformers,
	}
}

// RegistryImportInput is a registry file upload.
type RegistryImportInput struct {
	TenantID domain.UUID
	UserID   domain.UUID
	Kind     RegistryKind
	Content  []byte
	FileName string
	Mapping  map[string]string
	// DryRun only validates and reports what would change.
	DryRun bool
}

// RegistryImportOutput summarizes a registry import.
type RegistryImportOutput struct {
	Kind              RegistryKind            `json:"kind"`
	DryRun            bool                    `json:"dry_run"`
	TotalRows         int                     `json:"total_rows"`
	ValidRows         int                     `json:"valid_rows"`
	Created           int                     `json:"created"`
	Updated           int                     `json:"updated"`
	SubstationsNew    int                     `json:"substations_created"`
	RowsError         int                     `json:"rows_error"`
	Errors            []ingestion.ParseError  `json:"errors"`
	Mapping           ingestion.MappingReport `json:"mapping"`
	Detected          DetectedFormat          `json:"detected"`
	PlanLimit         *PlanLimitError         `json:"plan_limit,omitempty"`
	Message           string                  `json:"message"`
	DurationMS        int64                   `json:"duration_ms"`
	UnknownReferences []string                `json:"unknown_references,omitempty"`
}

// Import validates the whole file and, unless DryRun, applies it in a single
// transaction: either every valid row is written or none is.
func (uc *RegistryImportUseCase) Import(ctx context.Context, in RegistryImportInput) (*RegistryImportOutput, error) {
	began := time.Now()
	table, err := ingestion.ReadTable(in.Content, in.FileName)
	if err != nil {
		return nil, badRequest(err.Error())
	}
	out := &RegistryImportOutput{
		Kind:     in.Kind,
		DryRun:   in.DryRun,
		Detected: DetectedFormat{Format: table.Format, Delimiter: table.Delimiter, Headers: table.Headers, Rows: len(table.Rows)},
	}

	switch in.Kind {
	case RegistryConsumingUnits:
		err = uc.importUCs(ctx, in, table, out)
	default:
		out.Kind = RegistryTransformers
		err = uc.importTransformers(ctx, in, table, out)
	}
	if err != nil {
		return nil, err
	}
	if out.Errors == nil {
		out.Errors = []ingestion.ParseError{}
	}
	out.RowsError = len(out.Errors)
	out.DurationMS = time.Since(began).Milliseconds()
	verb := "importadas"
	if in.DryRun {
		verb = "validadas"
	}
	out.Message = fmt.Sprintf("%d linhas %s: %d novas, %d atualizadas, %d com erro", out.ValidRows, verb, out.Created, out.Updated, out.RowsError)
	slog.Info("registry imported", "tenant_id", in.TenantID, "kind", out.Kind, "dry_run", in.DryRun,
		"created", out.Created, "updated", out.Updated, "errors", out.RowsError)
	return out, nil
}

func (uc *RegistryImportUseCase) importTransformers(ctx context.Context, in RegistryImportInput, table *ingestion.Table, out *RegistryImportOutput) error {
	rows, errs, report, err := ingestion.ParseTransformerRegistry(table, in.Mapping)
	out.Mapping = report
	if err != nil {
		return badRequest(err.Error())
	}
	out.Errors = errs
	out.TotalRows = len(rows) + len(errs)
	out.ValidRows = len(rows)

	refs, err := uc.transformerRepo.RefsByTenant(ctx, in.TenantID)
	if err != nil {
		return fmt.Errorf("load transformers: %w", err)
	}
	newOnes := 0
	for _, r := range rows {
		if _, exists := refs[strings.ToUpper(r.Transformer.Code)]; !exists {
			newOnes++
		}
	}

	if err := uc.transformers.CheckTransformerQuota(ctx, in.TenantID, newOnes); err != nil {
		if pl, ok := err.(*PlanLimitError); ok {
			out.PlanLimit = pl
			if !in.DryRun {
				return pl
			}
		} else {
			return err
		}
	}

	if in.DryRun {
		out.Created = newOnes
		out.Updated = len(rows) - newOnes
		return nil
	}

	return uc.db.InTx(ctx, func(tx pgx.Tx) error {
		subs := map[string]domain.UUID{}
		for _, r := range rows {
			t := r.Transformer
			t.ID = domain.UUID(uuid.New().String())
			t.TenantID = in.TenantID
			if r.SubstationCode != "" {
				key := strings.ToUpper(r.SubstationCode)
				sid, ok := subs[key]
				if !ok {
					var created bool
					sid, created, err = uc.substationRepo.GetOrCreateByCode(ctx, tx, in.TenantID, r.SubstationCode)
					if err != nil {
						return fmt.Errorf("line %d: substation: %w", r.Line, err)
					}
					if created {
						out.SubstationsNew++
					}
					subs[key] = sid
				}
				t.SubstationID = &sid
			}
			inserted, err := uc.transformerRepo.UpsertByCode(ctx, tx, &t)
			if err != nil {
				return fmt.Errorf("line %d: %w", r.Line, err)
			}
			if inserted {
				out.Created++
			} else {
				out.Updated++
			}
		}
		return nil
	})
}

func (uc *RegistryImportUseCase) importUCs(ctx context.Context, in RegistryImportInput, table *ingestion.Table, out *RegistryImportOutput) error {
	rows, errs, report, err := ingestion.ParseUCRegistry(table, in.Mapping)
	out.Mapping = report
	if err != nil {
		return badRequest(err.Error())
	}
	out.TotalRows = len(rows) + len(errs)

	trafos, err := uc.transformerRepo.RefsByTenant(ctx, in.TenantID)
	if err != nil {
		return fmt.Errorf("load transformers: %w", err)
	}
	existing, err := uc.ucRepo.RefsByTenant(ctx, in.TenantID)
	if err != nil {
		return fmt.Errorf("load consuming units: %w", err)
	}

	type resolved struct {
		row ingestion.UCRow
		tID domain.UUID
	}
	var ok []resolved
	unknown := map[string]bool{}
	for _, r := range rows {
		ref, found := lookupTransformer(trafos, r.TransformerRef)
		if !found {
			unknown[r.TransformerRef] = true
			errs = append(errs, ingestion.ParseError{Line: r.Line, Column: "transformer", Value: r.TransformerRef, Message: "transformador não cadastrado"})
			continue
		}
		ok = append(ok, resolved{row: r, tID: ref.ID})
	}
	for ref := range unknown {
		out.UnknownReferences = append(out.UnknownReferences, ref)
	}
	out.Errors = errs
	out.ValidRows = len(ok)

	if in.DryRun {
		for _, r := range ok {
			if _, exists := existing[strings.ToUpper(r.row.UCCode)]; exists {
				out.Updated++
			} else {
				out.Created++
			}
		}
		return nil
	}

	return uc.db.InTx(ctx, func(tx pgx.Tx) error {
		for _, r := range ok {
			u := &domain.ConsumingUnit{
				ID:            domain.UUID(uuid.New().String()),
				TenantID:      in.TenantID,
				TransformerID: r.tID,
				UCCode:        r.row.UCCode,
				Name:          r.row.Name,
				Class:         r.row.Class,
				Active:        true,
			}
			inserted, err := uc.ucRepo.UpsertByCode(ctx, tx, u)
			if err != nil {
				return fmt.Errorf("line %d: %w", r.row.Line, err)
			}
			if inserted {
				out.Created++
			} else {
				out.Updated++
			}
		}
		return nil
	})
}

// CreateTransformers creates several transformers atomically after
// validating all of them and the plan quota (POST /transformers/batch).
func (uc *RegistryImportUseCase) CreateTransformers(ctx context.Context, tenantID domain.UUID, inputs []CreateTransformerInput) ([]*domain.Transformer, error) {
	v := &domain.ValidationError{}
	seen := map[string]bool{}
	items := make([]*domain.Transformer, 0, len(inputs))
	now := time.Now()
	for i, in := range inputs {
		t := &domain.Transformer{
			ID: domain.UUID(uuid.New().String()), TenantID: tenantID, Code: strings.TrimSpace(in.Code),
			PowerKVA: in.PowerKVA, PrimaryVoltageKV: in.PrimaryVoltageKV, SecondaryVoltageV: in.SecondaryVoltageV,
			Lat: in.Lat, Lng: in.Lng, CoreLossKW: in.CoreLossKW, WindingLossKW: in.WindingLossKW,
			LossLimitPct: in.LossLimitPct, SubstationID: in.SubstationID, Active: true, CreatedAt: now, UpdatedAt: now,
		}
		if err := domain.ValidateTransformer(*t); err != nil {
			if ve, ok := err.(*domain.ValidationError); ok {
				for _, f := range ve.Fields {
					v.Fields = append(v.Fields, domain.FieldError{Field: fmt.Sprintf("[%d].%s", i, f.Field), Message: f.Message})
				}
			}
			continue
		}
		key := strings.ToUpper(t.Code)
		if seen[key] {
			v.Fields = append(v.Fields, domain.FieldError{Field: fmt.Sprintf("[%d].code", i), Message: "código repetido na requisição"})
			continue
		}
		seen[key] = true
		items = append(items, t)
	}
	if err := v.OrNil(); err != nil {
		return nil, err
	}
	if err := uc.transformers.CheckTransformerQuota(ctx, tenantID, len(items)); err != nil {
		return nil, err
	}

	err := uc.db.InTx(ctx, func(tx pgx.Tx) error {
		for _, t := range items {
			if _, err := tx.Exec(ctx, `
				INSERT INTO transformers (id, tenant_id, substation_id, code, power_kva, primary_voltage_kv,
					secondary_voltage_v, lat, lng, core_loss_kw, winding_loss_kw, loss_limit_pct, active, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, true, $13, $13)`,
				t.ID, t.TenantID, t.SubstationID, t.Code, t.PowerKVA, t.PrimaryVoltageKV, t.SecondaryVoltageV,
				t.Lat, t.Lng, t.CoreLossKW, t.WindingLossKW, t.LossLimitPct, now); err != nil {
				if isUniqueViolation(err) {
					return fmt.Errorf("%w: %s", ErrTransformerCodeExists, t.Code)
				}
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}
