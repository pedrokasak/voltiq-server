package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/usecase"
)

// ExportHandler handles export artifact HTTP requests (PDF/Excel balances)
type ExportHandler struct {
	exportUseCase *usecase.ExportUseCase
}

// NewExportHandler creates a new ExportHandler
func NewExportHandler(exportUseCase *usecase.ExportUseCase) *ExportHandler {
	return &ExportHandler{
		exportUseCase: exportUseCase,
	}
}

// ExportBalance handles GET /api/v1/exports/balance/{transformer_id}?format=pdf|excel
func (h *ExportHandler) ExportBalance(w http.ResponseWriter, r *http.Request) {
	transformerID := chi.URLParam(r, "transformer_id")
	if strings.TrimSpace(transformerID) == "" {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "transformer_id is required", nil))
		return
	}

	format := usecase.ParseExportFormat(r.URL.Query().Get("format"))

	body, contentType, ext, err := h.exportUseCase.ExportBalance(
		r.Context(),
		domain.UUID(transformerID),
		format,
	)
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrExportTransformerNotFound):
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "transformer not found", nil))
		case errors.Is(err, usecase.ErrExportNoBalances):
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "no balances available for the requested period", nil))
		case errors.Is(err, usecase.ErrExportInvalidFormat):
			request.WriteJSON(w, http.StatusBadRequest,
				request.Fail("VALIDATION_ERROR", "format must be pdf or excel", nil))
		default:
			request.WriteJSON(w, http.StatusInternalServerError,
				request.Fail("EXPORT_ERROR", err.Error(), nil))
		}
		return
	}

	filename := fmt.Sprintf("balance-%s.%s", transformerID, ext)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
