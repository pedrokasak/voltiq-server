package handler

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/energybalance/server/internal/delivery/middleware"
	"github.com/energybalance/server/internal/delivery/request"
	"github.com/energybalance/server/internal/domain"
	"github.com/energybalance/server/internal/ingestion"
	"github.com/energybalance/server/internal/usecase"
)

// ImportHandler handles CSV import HTTP requests
type ImportHandler struct {
	importUseCase *usecase.ImportUseCase
}

// NewImportHandler creates a new ImportHandler
func NewImportHandler(importUseCase *usecase.ImportUseCase) *ImportHandler {
	return &ImportHandler{
		importUseCase: importUseCase,
	}
}

// UploadTransformerReadings handles transformer readings CSV upload
func (h *ImportHandler) UploadTransformerReadings(w http.ResponseWriter, r *http.Request) {
	h.handleUpload(w, r, ingestion.ReadingTypeTransformer)
}

// UploadUCReadings handles UC readings CSV upload
func (h *ImportHandler) UploadUCReadings(w http.ResponseWriter, r *http.Request) {
	h.handleUpload(w, r, ingestion.ReadingTypeUC)
}

func (h *ImportHandler) handleUpload(w http.ResponseWriter, r *http.Request, readingType ingestion.ReadingType) {
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())

	file, header, err := r.FormFile("file")
	if err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "missing file", nil))
		return
	}
	defer file.Close()

	output, err := h.importUseCase.ImportReadings(r.Context(), usecase.ImportInput{
		TenantID:    tenantID,
		UserID:      userID,
		File:        file,
		FileName:    header.Filename,
		ReadingType: readingType,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("IMPORT_ERROR", err.Error(), nil))
		return
	}

	response := map[string]any{
		"import_id":  output.ImportID,
		"status":     output.Status,
		"total_rows": output.TotalRows,
		"rows_ok":    output.RowsOK,
		"rows_error": output.RowsError,
		"message":    output.Message,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, "import completed successfully"))
}

// GetImport handles getting an import by ID
func (h *ImportHandler) GetImport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	importRecord, err := h.importUseCase.GetImport(r.Context(), domain.UUID(id))
	if err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "import not found", nil))
		return
	}

	response := map[string]any{
		"import_id":    importRecord.ID,
		"file_name":    importRecord.FileName,
		"status":       importRecord.Status,
		"total_rows":   importRecord.TotalRows,
		"rows_ok":      importRecord.RowsOK,
		"rows_error":   importRecord.RowsError,
		"errors":       importRecord.ErrorsJSON,
		"created_at":   importRecord.CreatedAt,
		"completed_at": importRecord.CompletedAt,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// ListImports handles listing all imports for a tenant
func (h *ImportHandler) ListImports(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	imports, err := h.importUseCase.ListImports(r.Context(), tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	response := map[string]any{
		"imports": imports,
		"count":   len(imports),
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// GetImportStatus handles getting import status with detailed progress
func (h *ImportHandler) GetImportStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	importRecord, err := h.importUseCase.GetImport(r.Context(), domain.UUID(id))
	if err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "import not found", nil))
		return
	}

	// Calculate progress percentage
	progress := 0
	if importRecord.TotalRows != nil && *importRecord.TotalRows > 0 {
		if importRecord.Status == domain.ImportStatusCompleted {
			progress = 100
		} else if importRecord.RowsOK != nil {
			progress = (*importRecord.RowsOK * 100) / *importRecord.TotalRows
		}
	}

	response := map[string]any{
		"import_id":    importRecord.ID,
		"file_name":    importRecord.FileName,
		"status":       importRecord.Status,
		"progress_pct": progress,
		"total_rows":   importRecord.TotalRows,
		"rows_ok":      importRecord.RowsOK,
		"rows_error":   importRecord.RowsError,
		"errors":       importRecord.ErrorsJSON,
		"created_at":   importRecord.CreatedAt,
		"completed_at": importRecord.CompletedAt,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// RetryImport handles retrying a failed import
func (h *ImportHandler) RetryImport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	// Implementar lógica de retry
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// CancelImport handles cancelling a processing import
func (h *ImportHandler) CancelImport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	// Implementar lógica de cancelamento
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// DeleteImport handles deleting an import record
func (h *ImportHandler) DeleteImport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	// Implementar lógica de deleção
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// GetImportSummary handles getting import summary statistics
func (h *ImportHandler) GetImportSummary(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	imports, err := h.importUseCase.ListImports(r.Context(), tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	totalImports := len(imports)
	completed := 0
	processing := 0
	failed := 0
	totalRows := 0
	totalRowsOK := 0
	totalRowsError := 0

	for _, imp := range imports {
		switch imp.Status {
		case domain.ImportStatusCompleted:
			completed++
		case domain.ImportStatusProcessing:
			processing++
		case domain.ImportStatusError:
			failed++
		}

		if imp.TotalRows != nil {
			totalRows += *imp.TotalRows
		}
		if imp.RowsOK != nil {
			totalRowsOK += *imp.RowsOK
		}
		if imp.RowsError != nil {
			totalRowsError += *imp.RowsError
		}
	}

	response := map[string]any{
		"total_imports":   totalImports,
		"completed":       completed,
		"processing":      processing,
		"failed":          failed,
		"total_rows":      totalRows,
		"total_rows_ok":   totalRowsOK,
		"total_rows_error": totalRowsError,
		"success_rate":    0.0,
	}

	if totalRows > 0 {
		response["success_rate"] = float64(totalRowsOK) / float64(totalRows) * 100
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// DownloadErrorReport handles downloading error report for a failed import
func (h *ImportHandler) DownloadErrorReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	importRecord, err := h.importUseCase.GetImport(r.Context(), domain.UUID(id))
	if err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "import not found", nil))
		return
	}

	if importRecord.ErrorsJSON == nil || len(importRecord.ErrorsJSON) == 0 {
		request.WriteJSON(w, http.StatusOK, request.Success(map[string]any{
			"message": "no errors found",
		}, ""))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(importRecord.ErrorsJSON, ""))
}

// GetImportLogs handles getting import processing logs
func (h *ImportHandler) GetImportLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	// Implementar lógica para buscar logs
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// BatchUpload handles batch upload of multiple files
func (h *ImportHandler) BatchUpload(w http.ResponseWriter, r *http.Request) {
	// Implementar lógica para upload em lote
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// GetUploadTemplate handles downloading CSV upload template
func (h *ImportHandler) GetUploadTemplate(w http.ResponseWriter, r *http.Request) {
	readingType := r.URL.Query().Get("type")
	if readingType == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "type is required (transformer or uc)", nil))
		return
	}

	// Implementar lógica para baixar template
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// ValidateCSV handles validating CSV file before upload
func (h *ImportHandler) ValidateCSV(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("file")
	if err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "missing file", nil))
		return
	}
	defer file.Close()

	// Ler o arquivo para validação
	content, err := io.ReadAll(file)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("READ_ERROR", "failed to read file", nil))
		return
	}

	// Implementar validação do CSV
	// Por enquanto, retorna sucesso
	response := map[string]any{
		"valid":     true,
		"file_size": len(content),
		"message":   "CSV validation not implemented yet",
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, "file validated"))
}

// GetImportHistory handles getting import history with filters
func (h *ImportHandler) GetImportHistory(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	status := r.URL.Query().Get("status")
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	imports, err := h.importUseCase.ListImports(r.Context(), tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	// Filter by status if provided
	if status != "" {
		filtered := []*domain.Import{}
		for _, imp := range imports {
			if string(imp.Status) == status {
				filtered = append(filtered, imp)
			}
		}
		imports = filtered
	}

	// Apply limit
	if len(imports) > limit {
		imports = imports[:limit]
	}

	response := map[string]any{
		"imports": imports,
		"count":   len(imports),
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}
