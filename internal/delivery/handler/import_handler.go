package handler

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/ingestion"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/usecase"
)

// maxUploadBytes is the upload limit of the SDD (32 MB).
const maxUploadBytes = 32 << 20

// ImportHandler handles CSV/XLSX import HTTP requests
type ImportHandler struct {
	importUseCase *usecase.ImportUseCase
}

// NewImportHandler creates a new ImportHandler
func NewImportHandler(importUseCase *usecase.ImportUseCase) *ImportHandler {
	return &ImportHandler{importUseCase: importUseCase}
}

// upload is a validated uploaded file.
type upload struct {
	Name    string
	Content []byte
	Mapping map[string]string
}

// readUpload reads the "file" multipart field (or field), validating size,
// extension and content (texto ou XLSX), plus the optional "mapping" JSON.
func readUpload(w http.ResponseWriter, r *http.Request, field string) (*upload, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			request.WriteJSON(w, http.StatusRequestEntityTooLarge, request.Fail("FILE_TOO_LARGE", "arquivo maior que 32 MB", nil))
			return nil, false
		}
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "envie o arquivo como multipart/form-data no campo \"file\"", nil))
		return nil, false
	}
	file, header, err := r.FormFile(field)
	if err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "arquivo não enviado (campo \"file\")", nil))
		return nil, false
	}
	defer file.Close()

	up, ok := validateFile(w, header.Filename, file)
	if !ok {
		return nil, false
	}
	if raw := r.FormValue("mapping"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &up.Mapping); err != nil {
			request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "mapping deve ser um objeto JSON {campo: coluna}", nil))
			return nil, false
		}
	}
	return up, true
}

func validateFile(w http.ResponseWriter, name string, file io.Reader) (*upload, bool) {
	content, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("READ_ERROR", "falha ao ler o arquivo", nil))
		return nil, false
	}
	if len(content) > maxUploadBytes {
		request.WriteJSON(w, http.StatusRequestEntityTooLarge, request.Fail("FILE_TOO_LARGE", "arquivo maior que 32 MB", nil))
		return nil, false
	}
	if len(bytes.TrimSpace(content)) == 0 {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "arquivo vazio", nil))
		return nil, false
	}

	ext := strings.ToLower(filepath.Ext(name))
	isZip := bytes.HasPrefix(content, []byte{0x50, 0x4B, 0x03, 0x04})
	switch {
	case ext == ".xlsx" || isZip:
		if !isZip {
			request.WriteJSON(w, http.StatusBadRequest, request.Fail("INVALID_FILE_TYPE", "arquivo .xlsx inválido", nil))
			return nil, false
		}
	case ext == ".csv" || ext == ".txt" || ext == "":
		sample := content
		if len(sample) > 8192 {
			sample = sample[:8192]
		}
		if bytes.IndexByte(sample, 0) >= 0 {
			request.WriteJSON(w, http.StatusBadRequest, request.Fail("INVALID_FILE_TYPE", "o arquivo não é um CSV de texto", nil))
			return nil, false
		}
	default:
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("INVALID_FILE_TYPE", "formato não suportado; use .csv ou .xlsx", nil))
		return nil, false
	}
	return &upload{Name: filepath.Base(name), Content: content}, true
}

// readRegistryUpload reads a registry import request (file, mapping, dry_run).
func readRegistryUpload(w http.ResponseWriter, r *http.Request) (usecase.RegistryImportInput, bool) {
	up, ok := readUpload(w, r, "file")
	if !ok {
		return usecase.RegistryImportInput{}, false
	}
	dry := r.FormValue("dry_run") == "true" || r.URL.Query().Get("dry_run") == "true"
	return usecase.RegistryImportInput{Content: up.Content, FileName: up.Name, Mapping: up.Mapping, DryRun: dry}, true
}

func readingTypeParam(r *http.Request, def ingestion.ReadingType) ingestion.ReadingType {
	v := strings.ToLower(r.FormValue("type"))
	if v == "" {
		v = strings.ToLower(r.URL.Query().Get("type"))
	}
	switch v {
	case "uc", "ucs", "uc_readings", "consuming_units":
		return ingestion.ReadingTypeUC
	case "transformer", "transformers", "transformer_readings":
		return ingestion.ReadingTypeTransformer
	}
	return def
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
	up, ok := readUpload(w, r, "file")
	if !ok {
		return
	}
	out, err := h.importUseCase.ImportReadings(r.Context(), usecase.ImportInput{
		TenantID:    middleware.GetTenantID(r.Context()),
		UserID:      middleware.GetUserID(r.Context()),
		Content:     up.Content,
		FileName:    up.Name,
		ReadingType: readingType,
		Mapping:     up.Mapping,
		SkipRecalc:  r.FormValue("recalculate") == "false",
	})
	if err != nil {
		writeUseCaseError(w, err, "IMPORT_ERROR")
		return
	}
	status := http.StatusOK
	if out.Status == domain.ImportStatusError {
		status = http.StatusUnprocessableEntity
	}
	request.WriteJSON(w, status, request.Success(out, out.Message))
}

// ValidateCSV analyzes a file without importing it: detected format,
// column mapping, preview, row errors, unknown references and duplicates.
func (h *ImportHandler) ValidateCSV(w http.ResponseWriter, r *http.Request) {
	up, ok := readUpload(w, r, "file")
	if !ok {
		return
	}
	out, err := h.importUseCase.Validate(r.Context(), usecase.ImportInput{
		TenantID:    middleware.GetTenantID(r.Context()),
		Content:     up.Content,
		FileName:    up.Name,
		ReadingType: readingTypeParam(r, ingestion.ReadingTypeTransformer),
		Mapping:     up.Mapping,
	})
	if err != nil {
		writeUseCaseError(w, err, "VALIDATION_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(out, out.Message))
}

// BatchUpload imports several files in one request (fields "files", type).
func (h *ImportHandler) BatchUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4*maxUploadBytes)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "envie os arquivos no campo \"files\"", nil))
		return
	}
	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "nenhum arquivo enviado (campo \"files\")", nil))
		return
	}
	if len(headers) > 20 {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "máximo de 20 arquivos por envio", nil))
		return
	}
	rt := readingTypeParam(r, ingestion.ReadingTypeTransformer)

	results := make([]any, 0, len(headers))
	for _, fh := range headers {
		f, err := fh.Open()
		if err != nil {
			results = append(results, map[string]any{"file_name": fh.Filename, "status": "ERROR", "message": "falha ao abrir o arquivo"})
			continue
		}
		content, err := io.ReadAll(io.LimitReader(f, maxUploadBytes+1))
		f.Close()
		if err != nil || len(content) > maxUploadBytes {
			results = append(results, map[string]any{"file_name": fh.Filename, "status": "ERROR", "message": "arquivo ilegível ou maior que 32 MB"})
			continue
		}
		out, err := h.importUseCase.ImportReadings(r.Context(), usecase.ImportInput{
			TenantID:    middleware.GetTenantID(r.Context()),
			UserID:      middleware.GetUserID(r.Context()),
			Content:     content,
			FileName:    filepath.Base(fh.Filename),
			ReadingType: rt,
		})
		if err != nil {
			results = append(results, map[string]any{"file_name": fh.Filename, "status": "ERROR", "message": "erro ao importar o arquivo"})
			continue
		}
		results = append(results, out)
	}
	request.WriteJSON(w, http.StatusOK, request.Success(results, fmt.Sprintf("%d arquivo(s) processado(s)", len(results))))
}

// GetUploadTemplate downloads an example file (RF08.3).
// ?type=transformer_readings|uc_readings|transformers|consuming_units
func (h *ImportHandler) GetUploadTemplate(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("type")
	switch kind {
	case "", "transformer", "transformers", "transformer_readings":
		kind = ingestion.KindTransformerReadings
	case "uc", "ucs", "uc_readings":
		kind = ingestion.KindUCReadings
	case "transformer_registry":
		kind = ingestion.KindTransformerRegistry
	case "uc_registry":
		kind = ingestion.KindUCRegistry
	}
	content, filename, err := usecase.Template(kind)
	if err != nil {
		writeUseCaseError(w, err, "TEMPLATE_ERROR")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

// GetSchemas lists the columns each import accepts (for the mapping UI).
func (h *ImportHandler) GetSchemas(w http.ResponseWriter, r *http.Request) {
	request.WriteJSON(w, http.StatusOK, request.Success(ingestion.Schemas(), ""))
}

// importView is the API shape of an import (SDD 05-api-endpoints).
func importView(imp *domain.Import) map[string]any {
	progress := 100
	if imp.Status == domain.ImportStatusProcessing {
		progress = 0
		if imp.TotalRows != nil && *imp.TotalRows > 0 && imp.RowsOK != nil {
			progress = *imp.RowsOK * 100 / *imp.TotalRows
		}
	}
	var errs any
	if imp.ErrorsJSON != nil {
		errs = imp.ErrorsJSON["errors"]
	}
	return map[string]any{
		"id":                 imp.ID,
		"import_id":          imp.ID,
		"file_name":          imp.FileName,
		"reading_type":       imp.ReadingType,
		"status":             imp.Status,
		"progress_pct":       progress,
		"total_rows":         imp.TotalRows,
		"rows_ok":            imp.RowsOK,
		"rows_error":         imp.RowsError,
		"duplicates_skipped": imp.DuplicatesSkipped,
		"errors":             errs,
		"can_retry":          imp.HasFile,
		"user_id":            imp.UserID,
		"created_at":         imp.CreatedAt,
		"completed_at":       imp.CompletedAt,
	}
}

func (h *ImportHandler) load(w http.ResponseWriter, r *http.Request) (*domain.Import, bool) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return nil, false
	}
	imp, err := h.importUseCase.GetImport(r.Context(), id)
	if err != nil {
		writeUseCaseError(w, err, "GET_ERROR")
		return nil, false
	}
	return imp, true
}

// GetImport handles getting an import by ID
func (h *ImportHandler) GetImport(w http.ResponseWriter, r *http.Request) {
	if imp, ok := h.load(w, r); ok {
		request.WriteJSON(w, http.StatusOK, request.Success(importView(imp), ""))
	}
}

// GetImportStatus returns the import status with progress (%)
func (h *ImportHandler) GetImportStatus(w http.ResponseWriter, r *http.Request) {
	if imp, ok := h.load(w, r); ok {
		request.WriteJSON(w, http.StatusOK, request.Success(importView(imp), ""))
	}
}

// ListImports handles listing imports (?status=&type=&limit=&offset=)
func (h *ImportHandler) ListImports(w http.ResponseWriter, r *http.Request) {
	items, total, err := h.importUseCase.ListFiltered(r.Context(), middleware.GetTenantID(r.Context()), repository.ImportFilter{
		Status:      strings.ToUpper(r.URL.Query().Get("status")),
		ReadingType: strings.ToUpper(r.URL.Query().Get("type")),
		Limit:       queryInt(r, "limit", 50),
		Offset:      queryInt(r, "offset", 0),
	})
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	views := make([]map[string]any, 0, len(items))
	for _, imp := range items {
		views = append(views, importView(imp))
	}
	w.Header().Set("X-Total-Count", fmt.Sprint(total))
	request.WriteJSON(w, http.StatusOK, request.Success(views, ""))
}

// GetImportHistory is ListImports kept for compatibility.
func (h *ImportHandler) GetImportHistory(w http.ResponseWriter, r *http.Request) {
	h.ListImports(w, r)
}

// GetImportSummary returns statistics of the import history
func (h *ImportHandler) GetImportSummary(w http.ResponseWriter, r *http.Request) {
	s, err := h.importUseCase.Summary(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "SUMMARY_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(s, ""))
}

// DownloadErrorReport returns the row errors of an import as CSV (RF02.3).
func (h *ImportHandler) DownloadErrorReport(w http.ResponseWriter, r *http.Request) {
	imp, ok := h.load(w, r)
	if !ok {
		return
	}
	var buf bytes.Buffer
	buf.WriteString("\ufeff")
	cw := csv.NewWriter(&buf)
	cw.Comma = ';'
	_ = cw.Write([]string{"linha", "coluna", "valor", "erro"})
	if imp.ErrorsJSON != nil {
		if list, ok := imp.ErrorsJSON["errors"].([]any); ok {
			for _, item := range list {
				e, _ := item.(map[string]any)
				_ = cw.Write([]string{fmt.Sprint(e["line"]), str(e["column"]), str(e["value"]), str(e["message"])})
			}
		}
	}
	cw.Flush()
	name := strings.TrimSuffix(imp.FileName, filepath.Ext(imp.FileName))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="erros_`+sanitizeFilename(name)+`.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "arquivo"
	}
	return b.String()
}

// GetImportLogs returns the processing log of an import.
func (h *ImportHandler) GetImportLogs(w http.ResponseWriter, r *http.Request) {
	imp, ok := h.load(w, r)
	if !ok {
		return
	}
	resp := map[string]any{"import_id": imp.ID, "status": imp.Status, "log": []any{}, "mapping": nil, "total_errors": 0}
	if imp.ErrorsJSON != nil {
		for _, k := range []string{"log", "mapping", "total_errors", "truncated"} {
			if v, ok := imp.ErrorsJSON[k]; ok {
				resp[k] = v
			}
		}
	}
	request.WriteJSON(w, http.StatusOK, request.Success(resp, ""))
}

// RetryImport reprocesses an import from the stored file.
func (h *ImportHandler) RetryImport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	out, err := h.importUseCase.Retry(r.Context(), id)
	if err != nil {
		writeUseCaseError(w, err, "RETRY_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(out, out.Message))
}

// DeleteImport undoes an import: removes its readings and the record.
// Requires ?confirm=true because the readings are deleted.
func (h *ImportHandler) DeleteImport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if r.URL.Query().Get("confirm") != "true" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("CONFIRMATION_REQUIRED",
			"esta ação remove as leituras da importação; repita com ?confirm=true", nil))
		return
	}
	res, err := h.importUseCase.Delete(r.Context(), id)
	if err != nil {
		writeUseCaseError(w, err, "DELETE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(res, "importação desfeita"))
}

// stamp formats a time for file names.
func stamp(t time.Time) string { return t.Format("20060102_1504") }
