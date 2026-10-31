package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/usecase"
)

// ExportHandler serves files and reports (RF06)
type ExportHandler struct {
	exportUseCase *usecase.ExportUseCase
	reports       *usecase.ReportUseCase
}

// NewExportHandler creates a new ExportHandler
func NewExportHandler(exportUseCase *usecase.ExportUseCase, reports *usecase.ReportUseCase) *ExportHandler {
	return &ExportHandler{exportUseCase: exportUseCase, reports: reports}
}

func writeFile(w http.ResponseWriter, f *usecase.ExportFile) {
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", f.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(f.Content)))
	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(f.Content)
}

func writeExportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrExportTransformerNotFound):
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "transformador não encontrado", nil))
	case errors.Is(err, usecase.ErrExportNoBalances):
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NO_DATA", "não há balanços calculados no período", nil))
	case errors.Is(err, usecase.ErrExportInvalidFormat):
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "formato deve ser pdf, excel ou csv", nil))
	default:
		writeUseCaseError(w, err, "EXPORT_ERROR")
	}
}

// period reads start/end (or period_start/period_end) query params.
func period(r *http.Request, defStart, defEnd time.Time) (time.Time, time.Time) {
	q := r.URL.Query()
	pick := func(a, b string) string {
		if v := q.Get(a); v != "" {
			return v
		}
		return q.Get(b)
	}
	start := parseDate(pick("start", "period_start"), defStart)
	end := parseDate(pick("end", "period_end"), defEnd)
	if end.Hour() == 0 && end.Minute() == 0 {
		end = end.Add(24*time.Hour - time.Nanosecond)
	}
	return start, end
}

// ExportBalance exports the balances of a transformer
// (GET /exports/balance/{transformer_id}?format=pdf|excel|csv&start=&end=).
func (h *ExportHandler) ExportBalance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "transformer_id")
	if !ok {
		return
	}
	start, end := period(r, time.Now().AddDate(-1, 0, 0), time.Now())
	f, err := h.exportUseCase.ExportBalance(r.Context(), id, usecase.ParseExportFormat(r.URL.Query().Get("format")), start, end)
	if err != nil {
		writeExportError(w, err)
		return
	}
	writeFile(w, f)
}

// ExportTransformerCSV exports one transformer's balances as CSV
// (GET /transformers/{id}/export).
func (h *ExportHandler) ExportTransformerCSV(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	format := usecase.ExportFormatCSV
	if v := r.URL.Query().Get("format"); v != "" {
		format = usecase.ParseExportFormat(v)
	}
	start, end := period(r, time.Now().AddDate(-1, 0, 0), time.Now())
	f, err := h.exportUseCase.ExportBalance(r.Context(), id, format, start, end)
	if err != nil {
		writeExportError(w, err)
		return
	}
	writeFile(w, f)
}

// ExportANEEL exports the tenant consolidated loss report
// (GET /exports/aneel?start=&end=, RF06.2). Default: last 12 months.
func (h *ExportHandler) ExportANEEL(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	defStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(-1, 0, 0)
	start, end := period(r, defStart, now)
	f, err := h.exportUseCase.ExportANEEL(r.Context(), middleware.GetTenantID(r.Context()), start, end)
	if err != nil {
		writeExportError(w, err)
		return
	}
	writeFile(w, f)
}

// ExportReadings exports the reading time series as CSV
// (GET /exports/readings?type=transformer|uc&transformer_id=&start=&end=, RF06.3).
func (h *ExportHandler) ExportReadings(w http.ResponseWriter, r *http.Request) {
	kind := "transformer"
	if strings.HasPrefix(strings.ToLower(r.URL.Query().Get("type")), "uc") {
		kind = "uc"
	}
	var tid domain.UUID
	if v := r.URL.Query().Get("transformer_id"); v != "" {
		if !isUUID(v) {
			request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "transformador não encontrado", nil))
			return
		}
		tid = domain.UUID(v)
	}
	start, end := period(r, time.Now().AddDate(0, -3, 0), time.Now())
	if end.Sub(start) > 400*24*time.Hour && tid == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR",
			"para exportar todos os transformadores use um período de até 13 meses", nil))
		return
	}
	f, err := h.exportUseCase.ExportReadings(r.Context(), middleware.GetTenantID(r.Context()), kind, tid, start, end)
	if err != nil {
		writeExportError(w, err)
		return
	}
	writeFile(w, f)
}

// Comparative returns the month-over-month report as JSON
// (GET /reports/comparative?month=YYYY-MM) or Excel (&format=excel).
func (h *ExportHandler) Comparative(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	month := parseDate(r.URL.Query().Get("month"), time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -1, 0))
	tenantID := middleware.GetTenantID(r.Context())
	if strings.EqualFold(r.URL.Query().Get("format"), "excel") {
		f, err := h.exportUseCase.ExportComparative(r.Context(), tenantID, month)
		if err != nil {
			writeExportError(w, err)
			return
		}
		writeFile(w, f)
		return
	}
	rep, err := h.reports.Comparative(r.Context(), tenantID, month)
	if err != nil {
		writeUseCaseError(w, err, "REPORT_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(rep, ""))
}
