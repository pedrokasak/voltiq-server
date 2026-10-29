package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/ingestion"
	"github.com/voltiq/server/internal/repository"
)

var (
	ErrImportNotFound   = errors.New("import not found")
	ErrInvalidFile      = errors.New("invalid file format")
	ErrImportNotRetried = errors.New("import has no stored file to retry")
)

// maxStoredErrors limits how many row errors are kept in errors_json.
const maxStoredErrors = 1000

// maxAutoRecalc limits the transformer-months recalculated right after an
// import; bigger imports ask the user to run the tenant recalculation.
const maxAutoRecalc = 500

// ImportUseCase handles CSV/XLSX reading imports (RF02)
type ImportUseCase struct {
	db              *repository.Database
	parser          *ingestion.CSVParser
	importRepo      *repository.ImportRepository
	transformerRepo *repository.TransformerRepository
	trafoReadings   *repository.TransformerReadingRepository
	ucRepo          *repository.ConsumingUnitRepository
	ucReadings      *repository.UCReadingRepository
	balance         *BalanceUseCase
}

// ImportInput contains import request data
type ImportInput struct {
	TenantID    domain.UUID
	UserID      domain.UUID
	File        io.Reader // used when Content is nil
	Content     []byte
	FileName    string
	ReadingType ingestion.ReadingType
	// Mapping maps schema fields to file headers chosen by the user
	// (importacao dinamica). Unmapped fields are matched automatically.
	Mapping map[string]string
	// SkipRecalc disables the automatic balance recalculation.
	SkipRecalc bool
}

// ImportOutput contains the import or validation result
type ImportOutput struct {
	ImportID          domain.UUID             `json:"import_id,omitempty"`
	Status            domain.ImportStatus     `json:"status"`
	FileName          string                  `json:"file_name"`
	ReadingType       string                  `json:"reading_type"`
	TotalRows         int                     `json:"total_rows"`
	RowsOK            int                     `json:"rows_ok"`
	RowsError         int                     `json:"rows_error"`
	DuplicatesSkipped int                     `json:"duplicates_skipped"`
	Message           string                  `json:"message"`
	Errors            []ingestion.ParseError  `json:"errors"`
	ErrorsTruncated   bool                    `json:"errors_truncated"`
	Mapping           ingestion.MappingReport `json:"mapping"`
	Detected          DetectedFormat          `json:"detected"`
	Preview           []map[string]any        `json:"preview,omitempty"`
	UnknownRefs       []string                `json:"unknown_references,omitempty"`
	PeriodStart       *time.Time              `json:"period_start,omitempty"`
	PeriodEnd         *time.Time              `json:"period_end,omitempty"`
	MetersAffected    int                     `json:"meters_affected"`
	Recalculated      *RecalcSummary          `json:"recalculated,omitempty"`
	DurationMS        int64                   `json:"duration_ms"`
}

// DetectedFormat describes how the file was read.
type DetectedFormat struct {
	Format    string   `json:"format"`
	Delimiter string   `json:"delimiter,omitempty"`
	Headers   []string `json:"headers"`
	Rows      int      `json:"rows"`
}

// RecalcSummary reports the automatic balance recalculation of an import.
type RecalcSummary struct {
	Transformers int    `json:"transformers"`
	Months       int    `json:"months"`
	Calculated   int    `json:"calculated"`
	Skipped      bool   `json:"skipped"`
	Reason       string `json:"reason,omitempty"`
}

// NewImportUseCase creates a new ImportUseCase
func NewImportUseCase(
	db *repository.Database,
	parser *ingestion.CSVParser,
	importRepo *repository.ImportRepository,
	transformerRepo *repository.TransformerRepository,
	trafoReadings *repository.TransformerReadingRepository,
	ucRepo *repository.ConsumingUnitRepository,
	ucReadings *repository.UCReadingRepository,
	balance *BalanceUseCase,
) *ImportUseCase {
	return &ImportUseCase{
		db:              db,
		parser:          parser,
		importRepo:      importRepo,
		transformerRepo: transformerRepo,
		trafoReadings:   trafoReadings,
		ucRepo:          ucRepo,
		ucReadings:      ucReadings,
		balance:         balance,
	}
}

// resolvedRow is a parsed reading with database IDs.
type resolvedRow struct {
	reading       *ingestion.ParsedReading
	meterID       domain.UUID // transformer or UC
	transformerID domain.UUID
}

type analysis struct {
	parse       *ingestion.ParseResult
	rows        []resolvedRow
	errors      []ingestion.ParseError
	unknownRefs []string
	start, end  time.Time
}

// analyze parses the file and resolves references within the tenant.
func (uc *ImportUseCase) analyze(ctx context.Context, in ImportInput) (*analysis, error) {
	parsed, err := uc.parser.ParseFile(in.Content, in.FileName, in.ReadingType, in.Mapping)
	if err != nil {
		if parsed != nil {
			return &analysis{parse: parsed}, badRequest(err.Error())
		}
		return nil, badRequest(err.Error())
	}

	a := &analysis{parse: parsed, errors: append([]ingestion.ParseError{}, parsed.Errors...)}
	unknown := map[string]bool{}

	if in.ReadingType == ingestion.ReadingTypeUC {
		refs, err := uc.ucRepo.RefsByTenant(ctx, in.TenantID)
		if err != nil {
			return nil, fmt.Errorf("load consuming units: %w", err)
		}
		for _, p := range parsed.Success {
			ref, ok := lookupUC(refs, p.UCID)
			if !ok {
				unknown[p.UCID] = true
				a.errors = append(a.errors, ingestion.ParseError{Line: p.Line, Column: "uc", Value: p.UCID,
					Message: "unidade consumidora não cadastrada"})
				continue
			}
			a.rows = append(a.rows, resolvedRow{reading: p, meterID: ref.ID, transformerID: ref.TransformerID})
		}
	} else {
		refs, err := uc.transformerRepo.RefsByTenant(ctx, in.TenantID)
		if err != nil {
			return nil, fmt.Errorf("load transformers: %w", err)
		}
		for _, p := range parsed.Success {
			ref, ok := lookupTransformer(refs, p.TransformerID)
			if !ok {
				unknown[p.TransformerID] = true
				a.errors = append(a.errors, ingestion.ParseError{Line: p.Line, Column: "transformer", Value: p.TransformerID,
					Message: "transformador não cadastrado"})
				continue
			}
			a.rows = append(a.rows, resolvedRow{reading: p, meterID: ref.ID, transformerID: ref.ID})
		}
	}

	for ref := range unknown {
		a.unknownRefs = append(a.unknownRefs, ref)
	}
	sort.Strings(a.unknownRefs)
	sort.Slice(a.errors, func(i, j int) bool { return a.errors[i].Line < a.errors[j].Line })

	for i, r := range a.rows {
		at := r.reading.ReadingAt
		if i == 0 || at.Before(a.start) {
			a.start = at
		}
		if i == 0 || at.After(a.end) {
			a.end = at
		}
	}
	return a, nil
}

func lookupTransformer(refs map[string]repository.TransformerRef, raw string) (repository.TransformerRef, bool) {
	if ref, ok := refs[strings.ToUpper(strings.TrimSpace(raw))]; ok {
		return ref, true
	}
	ref, ok := refs[strings.ToLower(strings.TrimSpace(raw))]
	return ref, ok
}

func lookupUC(refs map[string]repository.UCRef, raw string) (repository.UCRef, bool) {
	if ref, ok := refs[strings.ToUpper(strings.TrimSpace(raw))]; ok {
		return ref, true
	}
	ref, ok := refs[strings.ToLower(strings.TrimSpace(raw))]
	return ref, ok
}

// dedupe drops rows already stored and rows repeated inside the file.
func (uc *ImportUseCase) dedupe(ctx context.Context, tenantID domain.UUID, rt ingestion.ReadingType, a *analysis) (int, error) {
	if len(a.rows) == 0 {
		return 0, nil
	}
	idSet := map[domain.UUID]bool{}
	for _, r := range a.rows {
		idSet[r.meterID] = true
	}
	ids := make([]domain.UUID, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}

	var existing map[repository.ReadingKey]bool
	var err error
	if rt == ingestion.ReadingTypeUC {
		existing, err = uc.ucReadings.ExistingKeys(ctx, tenantID, ids, a.start, a.end)
	} else {
		existing, err = uc.trafoReadings.ExistingKeys(ctx, tenantID, ids, a.start, a.end)
	}
	if err != nil {
		return 0, fmt.Errorf("check duplicates: %w", err)
	}

	kept := a.rows[:0]
	dups := 0
	for _, r := range a.rows {
		key := repository.ReadingKey{MeterID: r.meterID, At: r.reading.ReadingAt.UTC()}
		if existing[key] {
			dups++
			continue
		}
		existing[key] = true
		kept = append(kept, r)
	}
	a.rows = kept
	return dups, nil
}

func (uc *ImportUseCase) fill(in *ImportInput) error {
	if in.Content == nil && in.File != nil {
		b, err := io.ReadAll(in.File)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		in.Content = b
	}
	if len(in.Content) == 0 {
		return badRequest("arquivo vazio")
	}
	if in.ReadingType == "" {
		in.ReadingType = ingestion.ReadingTypeTransformer
	}
	return nil
}

// Validate analyzes a file without writing anything (POST /imports/validate):
// detected format, column mapping, preview, row errors, unknown references
// and duplicates.
func (uc *ImportUseCase) Validate(ctx context.Context, in ImportInput) (*ImportOutput, error) {
	began := time.Now()
	if err := uc.fill(&in); err != nil {
		return nil, err
	}
	a, err := uc.analyze(ctx, in)
	out := &ImportOutput{FileName: in.FileName, ReadingType: string(in.ReadingType), Status: "VALID"}
	if a != nil && a.parse != nil {
		describeTable(out, a.parse)
	}
	if err != nil {
		if errors.Is(err, ErrBadRequest) && a != nil {
			out.Status = "INVALID"
			out.Message = strings.TrimPrefix(err.Error(), ErrBadRequest.Error()+": ")
			out.DurationMS = time.Since(began).Milliseconds()
			return out, nil
		}
		return nil, err
	}

	total := len(a.parse.Success) + len(a.parse.Errors)
	dups, err := uc.dedupe(ctx, in.TenantID, in.ReadingType, a)
	if err != nil {
		return nil, err
	}

	out.TotalRows = total
	out.RowsOK = len(a.rows)
	out.RowsError = len(a.errors)
	out.DuplicatesSkipped = dups
	out.UnknownRefs = a.unknownRefs
	out.MetersAffected = countMeters(a.rows)
	setErrors(out, a.errors, 200)
	if out.RowsOK > 0 {
		s, e := a.start, a.end
		out.PeriodStart, out.PeriodEnd = &s, &e
	}
	if out.RowsOK == 0 {
		out.Status = "INVALID"
	}
	out.Message = fmt.Sprintf("%d linhas prontas para importar, %d com erro, %d duplicadas", out.RowsOK, out.RowsError, dups)
	out.DurationMS = time.Since(began).Milliseconds()
	return out, nil
}

// ImportReadings validates, deduplicates and stores the readings of a file
// in one transaction, records the import history (RF02.4) and recalculates
// the balance of the affected transformers.
func (uc *ImportUseCase) ImportReadings(ctx context.Context, in ImportInput) (*ImportOutput, error) {
	if err := uc.fill(&in); err != nil {
		return nil, err
	}
	now := time.Now()
	imp := &domain.Import{
		ID:            domain.UUID(uuid.New().String()),
		TenantID:      in.TenantID,
		FileName:      in.FileName,
		Status:        domain.ImportStatusProcessing,
		CreatedAt:     now,
		ReadingType:   string(in.ReadingType),
		ColumnMapping: in.Mapping,
	}
	if in.UserID != "" {
		uid := in.UserID
		imp.UserID = &uid
	}
	if err := uc.importRepo.Create(ctx, imp, in.Content); err != nil {
		return nil, fmt.Errorf("create import record: %w", err)
	}
	return uc.process(ctx, imp, in)
}

// process runs (or re-runs) an import already recorded.
func (uc *ImportUseCase) process(ctx context.Context, imp *domain.Import, in ImportInput) (*ImportOutput, error) {
	began := time.Now()
	logs := []string{fmt.Sprintf("%s início do processamento de %s", began.Format(time.RFC3339), in.FileName)}
	out := &ImportOutput{ImportID: imp.ID, FileName: in.FileName, ReadingType: string(in.ReadingType)}

	a, err := uc.analyze(ctx, in)
	if a != nil && a.parse != nil {
		describeTable(out, a.parse)
	}
	if err != nil {
		msg := strings.TrimPrefix(err.Error(), ErrBadRequest.Error()+": ")
		logs = append(logs, "arquivo rejeitado: "+msg)
		uc.finish(ctx, imp, domain.ImportStatusError, 0, 0, 0, 0, []ingestion.ParseError{{Message: msg}}, out.Mapping, logs)
		out.Status = domain.ImportStatusError
		out.Message = msg
		out.Errors = []ingestion.ParseError{{Message: msg}}
		return out, nil
	}
	total := len(a.parse.Success) + len(a.parse.Errors)
	logs = append(logs, fmt.Sprintf("%d linhas lidas (%s), %d válidas, %d com erro",
		total, a.parse.Table.Format, len(a.rows), len(a.errors)))

	dups, err := uc.dedupe(ctx, in.TenantID, in.ReadingType, a)
	if err != nil {
		uc.finish(ctx, imp, domain.ImportStatusError, total, 0, len(a.errors), 0, a.errors, out.Mapping, append(logs, err.Error()))
		return nil, err
	}
	if dups > 0 {
		logs = append(logs, fmt.Sprintf("%d leituras já existentes ignoradas", dups))
	}

	err = uc.db.InTx(ctx, func(tx pgx.Tx) error {
		return uc.insert(ctx, tx, imp.ID, in, a.rows)
	})
	if err != nil {
		logs = append(logs, "falha ao gravar leituras: "+err.Error())
		uc.finish(ctx, imp, domain.ImportStatusError, total, 0, len(a.errors), dups, a.errors, out.Mapping, logs)
		return nil, fmt.Errorf("save readings: %w", err)
	}
	logs = append(logs, fmt.Sprintf("%d leituras gravadas", len(a.rows)))

	// So e erro quando nada foi aproveitado: duplicatas ignoradas contam
	// como leituras ja presentes.
	status := domain.ImportStatusCompleted
	if len(a.rows) == 0 && dups == 0 && len(a.errors) > 0 {
		status = domain.ImportStatusError
	}

	if status == domain.ImportStatusCompleted && !in.SkipRecalc && len(a.rows) > 0 {
		out.Recalculated = uc.recalculate(ctx, a.rows)
		if out.Recalculated.Skipped {
			logs = append(logs, "recálculo automático não executado: "+out.Recalculated.Reason)
		} else {
			logs = append(logs, fmt.Sprintf("balanço recalculado para %d transformador(es) em %d mês(es)",
				out.Recalculated.Transformers, out.Recalculated.Months))
		}
	}

	logs = append(logs, fmt.Sprintf("concluído em %d ms", time.Since(began).Milliseconds()))
	uc.finish(ctx, imp, status, total, len(a.rows), len(a.errors), dups, a.errors, out.Mapping, logs)

	out.Status = status
	out.TotalRows = total
	out.RowsOK = len(a.rows)
	out.RowsError = len(a.errors)
	out.DuplicatesSkipped = dups
	out.UnknownRefs = a.unknownRefs
	out.MetersAffected = countMeters(a.rows)
	setErrors(out, a.errors, 200)
	if len(a.rows) > 0 {
		s, e := a.start, a.end
		out.PeriodStart, out.PeriodEnd = &s, &e
	}
	out.Message = fmt.Sprintf("%d leituras importadas, %d linhas com erro, %d duplicadas ignoradas", out.RowsOK, out.RowsError, dups)
	out.DurationMS = time.Since(began).Milliseconds()

	slog.Info("readings imported", "import_id", imp.ID, "tenant_id", in.TenantID, "type", in.ReadingType,
		"rows_ok", out.RowsOK, "rows_error", out.RowsError, "duplicates", dups, "duration_ms", out.DurationMS)
	return out, nil
}

func (uc *ImportUseCase) insert(ctx context.Context, tx pgx.Tx, importID domain.UUID, in ImportInput, rows []resolvedRow) error {
	now := time.Now()
	imp := importID
	if in.ReadingType == ingestion.ReadingTypeUC {
		readings := make([]*domain.UCReading, 0, len(rows))
		for _, r := range rows {
			readings = append(readings, &domain.UCReading{
				ID:             domain.UUID(uuid.New().String()),
				TenantID:       in.TenantID,
				UCID:           r.meterID,
				TransformerID:  r.transformerID,
				ReadingAt:      r.reading.ReadingAt,
				ConsumptionKWh: r.reading.EnergyKWh,
				ImportID:       &imp,
				CreatedAt:      now,
			})
		}
		_, err := uc.ucReadings.CopyInsert(ctx, tx, readings)
		return err
	}
	readings := make([]*domain.TransformerReading, 0, len(rows))
	for _, r := range rows {
		readings = append(readings, &domain.TransformerReading{
			ID:            domain.UUID(uuid.New().String()),
			TenantID:      in.TenantID,
			TransformerID: r.meterID,
			ReadingAt:     r.reading.ReadingAt,
			EnergyKWh:     r.reading.EnergyKWh,
			DemandKW:      r.reading.DemandKW,
			PowerFactor:   r.reading.PowerFactor,
			ImportID:      &imp,
			CreatedAt:     now,
		})
	}
	_, err := uc.trafoReadings.CopyInsert(ctx, tx, readings)
	return err
}

// recalculate refaz o balanco mensal dos trafos afetados pela importacao
// (fluxo do SDD 03-ARQUITETURA: importacao -> calculo -> alertas).
func (uc *ImportUseCase) recalculate(ctx context.Context, rows []resolvedRow) *RecalcSummary {
	sum := &RecalcSummary{}
	if uc.balance == nil {
		sum.Skipped, sum.Reason = true, "cálculo não configurado"
		return sum
	}
	type tm struct {
		t     domain.UUID
		month time.Time
	}
	set := map[tm]bool{}
	trafos := map[domain.UUID]bool{}
	months := map[time.Time]bool{}
	for _, r := range rows {
		m := domain.MonthStart(r.reading.ReadingAt)
		set[tm{r.transformerID, m}] = true
		trafos[r.transformerID] = true
		months[m] = true
	}
	sum.Transformers, sum.Months = len(trafos), len(months)
	if len(set) > maxAutoRecalc {
		sum.Skipped = true
		sum.Reason = fmt.Sprintf("%d combinações trafo/mês; use o recálculo geral em Balanço", len(set))
		return sum
	}
	for k := range set {
		end := k.month.AddDate(0, 1, 0).Add(-time.Nanosecond)
		if _, err := uc.balance.CalculateBalance(ctx, CalculateBalanceInput{
			TransformerID: k.t, PeriodStart: k.month, PeriodEnd: end,
		}); err == nil {
			sum.Calculated++
		}
	}
	return sum
}

func (uc *ImportUseCase) finish(
	ctx context.Context, imp *domain.Import, status domain.ImportStatus,
	total, ok, bad, dups int, errs []ingestion.ParseError, mapping ingestion.MappingReport, logs []string,
) {
	completed := time.Now()
	imp.Status = status
	imp.TotalRows, imp.RowsOK, imp.RowsError = &total, &ok, &bad
	imp.DuplicatesSkipped = dups
	imp.CompletedAt = &completed

	stored := errs
	truncated := false
	if len(stored) > maxStoredErrors {
		stored, truncated = stored[:maxStoredErrors], true
	}
	imp.ErrorsJSON = map[string]any{
		"errors":       stored,
		"total_errors": len(errs),
		"truncated":    truncated,
		"mapping":      mapping,
		"log":          logs,
	}
	if err := uc.importRepo.Update(ctx, imp); err != nil {
		slog.Error("failed to update import record", "import_id", imp.ID, "error", err)
	}
}

func describeTable(out *ImportOutput, p *ingestion.ParseResult) {
	out.Mapping = p.Mapping
	if p.Table == nil {
		return
	}
	out.Detected = DetectedFormat{Format: p.Table.Format, Delimiter: p.Table.Delimiter, Headers: p.Table.Headers, Rows: len(p.Table.Rows)}
	n := len(p.Table.Rows)
	if n > 10 {
		n = 10
	}
	for i := 0; i < n; i++ {
		row := map[string]any{"_line": p.Table.FirstDataLine + i}
		for j, h := range p.Table.Headers {
			if j < len(p.Table.Rows[i]) {
				row[h] = p.Table.Rows[i][j]
			}
		}
		out.Preview = append(out.Preview, row)
	}
}

func setErrors(out *ImportOutput, errs []ingestion.ParseError, max int) {
	if len(errs) > max {
		out.Errors, out.ErrorsTruncated = errs[:max], true
		return
	}
	out.Errors = errs
	if out.Errors == nil {
		out.Errors = []ingestion.ParseError{}
	}
}

func countMeters(rows []resolvedRow) int {
	set := map[domain.UUID]bool{}
	for _, r := range rows {
		set[r.meterID] = true
	}
	return len(set)
}

// Retry reprocesses an import from the stored file: readings written by the
// previous attempt are removed first, so retrying never duplicates data.
func (uc *ImportUseCase) Retry(ctx context.Context, id domain.UUID) (*ImportOutput, error) {
	imp, err := uc.importRepo.GetByID(ctx, id)
	if err != nil || imp == nil {
		return nil, ErrImportNotFound
	}
	if imp.Status == domain.ImportStatusProcessing && imp.CreatedAt.After(time.Now().Add(-10*time.Minute)) {
		return nil, fmt.Errorf("%w: importação ainda em processamento", ErrConflict)
	}
	content, err := uc.importRepo.GetFileContent(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, badRequest("esta importação não guardou o arquivo original; envie o arquivo novamente")
	}
	rt := ingestion.ReadingType(imp.ReadingType)
	if rt == "" {
		rt = ingestion.ReadingTypeTransformer
	}

	if err := uc.db.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := uc.trafoReadings.DeleteByImport(ctx, tx, imp.TenantID, id); err != nil {
			return err
		}
		_, err := uc.ucReadings.DeleteByImport(ctx, tx, imp.TenantID, id)
		return err
	}); err != nil {
		return nil, fmt.Errorf("clear previous attempt: %w", err)
	}

	imp.Status = domain.ImportStatusProcessing
	in := ImportInput{TenantID: imp.TenantID, Content: content, FileName: imp.FileName, ReadingType: rt, Mapping: imp.ColumnMapping}
	if imp.UserID != nil {
		in.UserID = *imp.UserID
	}
	return uc.process(ctx, imp, in)
}

// DeleteResult reports what an import removal undid.
type DeleteResult struct {
	ImportID        domain.UUID `json:"import_id"`
	ReadingsRemoved int64       `json:"readings_removed"`
}

// Delete desfaz uma importacao a pedido do usuario: remove as leituras que
// ela gravou e marca o registro como excluido (soft delete).
func (uc *ImportUseCase) Delete(ctx context.Context, id domain.UUID) (*DeleteResult, error) {
	imp, err := uc.importRepo.GetByID(ctx, id)
	if err != nil || imp == nil {
		return nil, ErrImportNotFound
	}
	res := &DeleteResult{ImportID: id}
	err = uc.db.InTx(ctx, func(tx pgx.Tx) error {
		n1, err := uc.trafoReadings.DeleteByImport(ctx, tx, imp.TenantID, id)
		if err != nil {
			return err
		}
		n2, err := uc.ucReadings.DeleteByImport(ctx, tx, imp.TenantID, id)
		if err != nil {
			return err
		}
		res.ReadingsRemoved = n1 + n2
		return uc.importRepo.SoftDelete(ctx, tx, id)
	})
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrImportNotFound
	}
	if err != nil {
		return nil, err
	}
	slog.Info("import undone", "import_id", id, "tenant_id", imp.TenantID, "readings_removed", res.ReadingsRemoved)
	return res, nil
}

// GetImport returns an import record by ID
func (uc *ImportUseCase) GetImport(ctx context.Context, id domain.UUID) (*domain.Import, error) {
	importRecord, err := uc.importRepo.GetByID(ctx, id)
	if err != nil || importRecord == nil {
		return nil, ErrImportNotFound
	}
	return importRecord, nil
}

// ListImports returns all imports for a tenant
func (uc *ImportUseCase) ListImports(ctx context.Context, tenantID domain.UUID) ([]*domain.Import, error) {
	imports, err := uc.importRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, errors.New("failed to list imports")
	}
	if imports == nil {
		imports = []*domain.Import{}
	}
	return imports, nil
}

// ListFiltered returns a filtered page of imports and the total.
func (uc *ImportUseCase) ListFiltered(ctx context.Context, tenantID domain.UUID, f repository.ImportFilter) ([]*domain.Import, int, error) {
	items, total, err := uc.importRepo.List(ctx, tenantID, f)
	if err != nil {
		return nil, 0, fmt.Errorf("list imports: %w", err)
	}
	if items == nil {
		items = []*domain.Import{}
	}
	return items, total, nil
}

// Summary returns statistics of the import history.
func (uc *ImportUseCase) Summary(ctx context.Context, tenantID domain.UUID) (*repository.ImportSummary, error) {
	return uc.importRepo.Summary(ctx, tenantID)
}

// Template returns an example file of a kind (RF08.3).
func Template(kind string) ([]byte, string, error) {
	schema, ok := ingestion.SchemaFor(kind)
	if !ok {
		return nil, "", badRequest("tipo de modelo inválido")
	}
	examples := map[string][][]string{
		ingestion.KindTransformerReadings: {
			{"TRF-001", "2025-01-31 23:59:59", "15000,50", "120,5", "0,92"},
			{"TRF-002", "2025-01-31 23:59:59", "9800,00", "80,0", "0,95"},
		},
		ingestion.KindUCReadings: {
			{"UC-1001", "2025-01-31 23:59:59", "250,75"},
			{"UC-1002", "2025-01-31 23:59:59", "180,40"},
		},
		ingestion.KindTransformerRegistry: {
			{"TRF-001", "112,5", "13,8", "220", "-23,550520", "-46,633308", "0,340", "1,450", "10", "SE-CENTRO"},
			{"TRF-002", "75", "13,8", "220", "-23,561000", "-46,655000", "0,250", "1,100", "10", "SE-CENTRO"},
		},
		ingestion.KindUCRegistry: {
			{"UC-1001", "TRF-001", "Sítio Boa Vista", "RURAL"},
			{"UC-1002", "TRF-001", "Mercado Central", "COMERCIAL"},
		},
	}
	return schema.Template(examples[kind]), "modelo_" + kind + ".csv", nil
}
