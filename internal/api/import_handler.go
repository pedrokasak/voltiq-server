package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/ingestion"
	"github.com/voltiq/server/internal/repository"
)

// ImportHandler handles CSV import requests
type ImportHandler struct {
	parser          *ingestion.CSVParser
	importRepo      *repository.ImportRepository
	transformerRepo *repository.TransformerReadingRepository
	ucReadingRepo   *repository.UCReadingRepository
}

// NewImportHandler creates a new ImportHandler
func NewImportHandler(
	parser *ingestion.CSVParser,
	importRepo *repository.ImportRepository,
	transformerRepo *repository.TransformerReadingRepository,
	ucReadingRepo *repository.UCReadingRepository,
) *ImportHandler {
	return &ImportHandler{
		parser:          parser,
		importRepo:      importRepo,
		transformerRepo: transformerRepo,
		ucReadingRepo:   ucReadingRepo,
	}
}

// ImportResponse represents an import response
type ImportResponse struct {
	ImportID  string `json:"import_id"`
	Status    string `json:"status"`
	TotalRows int    `json:"total_rows"`
	RowsOK    int    `json:"rows_ok"`
	RowsError int    `json:"rows_error"`
	Message   string `json:"message"`
}

// UploadTransformerReadings handles transformer readings upload
func (h *ImportHandler) UploadTransformerReadings(w http.ResponseWriter, r *http.Request) {
	h.handleUpload(w, r, ingestion.ReadingTypeTransformer)
}

// UploadUCReadings handles UC readings upload
func (h *ImportHandler) UploadUCReadings(w http.ResponseWriter, r *http.Request) {
	h.handleUpload(w, r, ingestion.ReadingTypeUC)
}

func (h *ImportHandler) handleUpload(w http.ResponseWriter, r *http.Request, readingType ingestion.ReadingType) {
	tenantID := GetTenantID(r.Context())
	userID := GetUserID(r.Context())

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "failed to parse multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	importID := domain.UUID(uuid.New().String())
	importRecord := &domain.Import{
		ID:        importID,
		TenantID:  tenantID,
		UserID:    &userID,
		FileName:  header.Filename,
		Status:    domain.ImportStatusProcessing,
		CreatedAt: time.Now(),
	}

	if err := h.importRepo.Create(r.Context(), importRecord); err != nil {
		http.Error(w, "failed to create import record", http.StatusInternalServerError)
		return
	}

	parseResult, err := h.parser.Parse(file, readingType)
	if err != nil {
		h.updateImportError(importID, err.Error())
		http.Error(w, "failed to parse CSV", http.StatusBadRequest)
		return
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
			errorsMap[fmt.Sprintf("line_%d", e.Line)] = e.Message
		}
		importRecord.ErrorsJSON = errorsMap
	}

	if readingType == ingestion.ReadingTypeTransformer {
		readings := h.parser.ToTransformerReadings(parseResult.Success, tenantID, &importID)
		if err := h.transformerRepo.CreateBatch(r.Context(), readings); err != nil {
			importRecord.Status = domain.ImportStatusError
			h.importRepo.Update(r.Context(), importRecord)
			http.Error(w, "failed to save transformer readings", http.StatusInternalServerError)
			return
		}
	} else {
		readings := h.parser.ToUCReadings(parseResult.Success, tenantID, &importID)
		if err := h.ucReadingRepo.CreateBatch(r.Context(), readings); err != nil {
			importRecord.Status = domain.ImportStatusError
			h.importRepo.Update(r.Context(), importRecord)
			http.Error(w, "failed to save UC readings", http.StatusInternalServerError)
			return
		}
	}

	completedAt := time.Now()
	importRecord.Status = domain.ImportStatusCompleted
	importRecord.CompletedAt = &completedAt

	if err := h.importRepo.Update(r.Context(), importRecord); err != nil {
		http.Error(w, "failed to update import status", http.StatusInternalServerError)
		return
	}

	response := ImportResponse{
		ImportID:  string(importID),
		Status:    string(importRecord.Status),
		TotalRows: totalRows,
		RowsOK:    rowsOK,
		RowsError: rowsError,
		Message:   fmt.Sprintf("imported %d readings successfully", rowsOK),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (h *ImportHandler) updateImportError(importID domain.UUID, errorMsg string) {
	ctx := context.Background()
	importRecord := &domain.Import{
		ID:     importID,
		Status: domain.ImportStatusError,
		ErrorsJSON: map[string]any{
			"global": errorMsg,
		},
		CompletedAt: ptr(time.Now()),
	}
	h.importRepo.Update(ctx, importRecord)
}

func ptr[T any](v T) *T {
	return &v
}
