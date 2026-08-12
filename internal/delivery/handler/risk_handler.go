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

// RiskHandler handles risk score and anomaly HTTP requests
type RiskHandler struct {
	riskUseCase *usecase.RiskUseCase
}

// NewRiskHandler creates a new RiskHandler
func NewRiskHandler(riskUseCase *usecase.RiskUseCase) *RiskHandler {
	return &RiskHandler{
		riskUseCase: riskUseCase,
	}
}

// GetRiskScore handles getting risk score for a specific transformer
func (h *RiskHandler) GetRiskScore(w http.ResponseWriter, r *http.Request) {
	transformerID := chi.URLParam(r, "transformer_id")
	if transformerID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "transformer_id is required", nil))
		return
	}

	output, err := h.riskUseCase.GetRiskScore(r.Context(), domain.UUID(transformerID))
	if err != nil {
		if err == usecase.ErrTransformerNotFound {
			request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "transformer not found", nil))
			return
		}
		if err == usecase.ErrBalanceNotFound {
			request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "no balance data available for this transformer", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("CALCULATION_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(output.RiskScore, ""))
}

// GetAllRiskScores handles getting risk scores for all transformers of a tenant
func (h *RiskHandler) GetAllRiskScores(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	scores, err := h.riskUseCase.GetAllRiskScores(r.Context(), tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(scores, ""))
}

// GetTransformerAnomalies handles getting anomalies for a specific transformer
func (h *RiskHandler) GetTransformerAnomalies(w http.ResponseWriter, r *http.Request) {
	transformerID := chi.URLParam(r, "transformer_id")
	if transformerID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "transformer_id is required", nil))
		return
	}

	// Optional query parameter for months back
	monthsBack := 12
	if mb := r.URL.Query().Get("months_back"); mb != "" {
		if v, err := strconv.Atoi(mb); err == nil && v > 0 && v <= 36 {
			monthsBack = v
		}
	}

	output, err := h.riskUseCase.GetAnomalies(r.Context(), domain.UUID(transformerID), monthsBack)
	if err != nil {
		if err == usecase.ErrTransformerNotFound {
			request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "transformer not found", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("CALCULATION_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(output.Anomalies, ""))
}

// GetAllTransformersAnomalies handles getting anomalies for all transformers of a tenant
func (h *RiskHandler) GetAllTransformersAnomalies(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	// Optional query parameter for months back
	monthsBack := 12
	if mb := r.URL.Query().Get("months_back"); mb != "" {
		if v, err := strconv.Atoi(mb); err == nil && v > 0 && v <= 36 {
			monthsBack = v
		}
	}

	anomalies, err := h.riskUseCase.GetAllAnomalies(r.Context(), tenantID, monthsBack)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(anomalies, ""))
}