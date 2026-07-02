package usecase

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/energybalance/server/internal/domain"
	"github.com/energybalance/server/internal/ingestion"
	"github.com/energybalance/server/internal/repository"
)

var (
	ErrImportNotFound = errors.New("import not found")
	ErrInvalidFile    = errors.New("invalid file format")
)

// ImportUseCase handles CSV import business logic
type ImportUseCase struct {
	parser           *ingestion.CSVParser
	importRepo       *repository.ImportRepository
	transformerRepo  *repository.TransformerReadingRepository
	ucReadingRepo    *repository.UCReadingRepository
}

// ImportInput contains import request data
type ImportInput struct {
	TenantID    domain.UUID
	UserID      domain.UUID
	File        io.Reader
	FileName    string
	ReadingType ingestion.ReadingType
}

// ImportOutput contains import result
type ImportOutput struct {
	ImportID  domain.UUID
	Status    domain.ImportStatus
	TotalRows int
	RowsOK    int
	RowsError int
	Message   string
}

// NewImportUseCase creates a new ImportUseCase
func NewImportUseCase(
	parser *ingestion.CSVParser,
	importRepo *repository.ImportRepository,
	transformerRepo *repository.TransformerReadingRepository,
	ucReadingRepo *repository.UCReadingRepository,
) *ImportUseCase {
	return &ImportUseCase{
		parser:          parser,
		importRepo:      importRepo,
		transformerRepo: transformerRepo,
		ucReadingRepo:   ucReadingRepo,
	}
}

// ImportReadings processes a CSV file and imports readings
func (uc *ImportUseCase) ImportReadings(ctx context.Context, input ImportInput) (*ImportOutput, error) {
	importID := domain.UUID(time.Now().Format("20060102150405"))
	
	importRecord := &domain.Import{
		ID:        importID,
		TenantID:  input.TenantID,
		UserID:    &input.UserID,
		FileName:  input.FileName,
		Status:    domain.ImportStatusProcessing,
		CreatedAt: time.Now(),
	}

	if err := uc.importRepo.Create(ctx, importRecord); err != nil {
		return nil, errors.New("failed to create import record")
	}

	parseResult, err := uc.parser.Parse(input.File, input.ReadingType)
	if err != nil {
		uc.updateImportError(ctx, importID, err.Error())
		return nil, ErrInvalidFile
	}

	totalRows := len(parseResult.Success) + len(parseResult.Errors)
	rowsOK := len(parseResult.Success)
	rowsError := len(parseResult.Errors)

	importRecord.TotalRows = &totalRows
	importRecord.RowsOK = &rowsOK
	importRecord.RowsError = &rowsError

	if len(parseResult.Errors) > 0 {
		errorsMap := make(map[string]any)
		for _, e := range parseResult.Errors {
			errorsMap[string(rune(e.Line))] = e.Message
		}
		importRecord.ErrorsJSON = errorsMap
	}

	if input.ReadingType == ingestion.ReadingTypeTransformer {
		readings := uc.parser.ToTransformerReadings(parseResult.Success, input.TenantID, &importID)
		if err := uc.transformerRepo.CreateBatch(ctx, readings); err != nil {
			importRecord.Status = domain.ImportStatusError
			uc.importRepo.Update(ctx, importRecord)
			return nil, errors.New("failed to save transformer readings")
		}
	} else {
		readings := uc.parser.ToUCReadings(parseResult.Success, input.TenantID, &importID)
		if err := uc.ucReadingRepo.CreateBatch(ctx, readings); err != nil {
			importRecord.Status = domain.ImportStatusError
			uc.importRepo.Update(ctx, importRecord)
			return nil, errors.New("failed to save UC readings")
		}
	}

	completedAt := time.Now()
	importRecord.Status = domain.ImportStatusCompleted
	importRecord.CompletedAt = &completedAt

	if err := uc.importRepo.Update(ctx, importRecord); err != nil {
		return nil, errors.New("failed to update import status")
	}

	return &ImportOutput{
		ImportID:  importID,
		Status:    importRecord.Status,
		TotalRows: totalRows,
		RowsOK:    rowsOK,
		RowsError: rowsError,
		Message:   "Imported " + string(rune(rowsOK)) + " readings successfully",
	}, nil
}

// GetImport returns an import record by ID
func (uc *ImportUseCase) GetImport(ctx context.Context, id domain.UUID) (*domain.Import, error) {
	importRecord, err := uc.importRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrImportNotFound
	}

	if importRecord == nil {
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

	return imports, nil
}

func (uc *ImportUseCase) updateImportError(ctx context.Context, importID domain.UUID, errorMsg string) {
	importRecord := &domain.Import{
		ID:     importID,
		Status: domain.ImportStatusError,
		ErrorsJSON: map[string]any{
			"global": errorMsg,
		},
		CompletedAt: ptr(time.Now()),
	}
	uc.importRepo.Update(ctx, importRecord)
}

func ptr[T any](v T) *T {
	return &v
}
