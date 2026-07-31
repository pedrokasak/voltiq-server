package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/usecase"
)

// FinancialHandler handles financial data endpoints
type FinancialHandler struct {
	financialUseCase *usecase.FinancialUseCase
}

// NewFinancialHandler creates a new FinancialHandler
func NewFinancialHandler(financialUseCase *usecase.FinancialUseCase) *FinancialHandler {
	return &FinancialHandler{
		financialUseCase: financialUseCase,
	}
}

// GetFinancialTable handles GET /api/v1/dashboard/financial-table
func (h *FinancialHandler) GetFinancialTable(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == "" {
		request.WriteJSON(w, http.StatusUnauthorized,
			request.Fail("UNAUTHORIZED", "tenant not found", nil))
		return
	}

	// Parse tariff parameter (optional, defaults to 450 R$/MWh)
	tariffStr := chi.URLParam(r, "tariff")
	tariff := 450.0 // default R$/MWh
	if tariffStr != "" {
		if t, err := strconv.ParseFloat(tariffStr, 64); err == nil && t > 0 {
			tariff = t
		}
	}

	data, err := h.financialUseCase.GetFinancialTable(r.Context(), domain.UUID(tenantID), tariff)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("FINANCIAL_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(data, ""))
}