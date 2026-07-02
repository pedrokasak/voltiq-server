package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/energybalance/server/internal/delivery/request"
	"github.com/energybalance/server/internal/domain"
	"github.com/energybalance/server/internal/usecase"
)

// BalanceHandler handles energy balance HTTP requests
type BalanceHandler struct {
	balanceUseCase *usecase.BalanceUseCase
}

// CalculateBalanceRequest represents a balance calculation request
type CalculateBalanceRequest struct {
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
}

// NewBalanceHandler creates a new BalanceHandler
func NewBalanceHandler(balanceUseCase *usecase.BalanceUseCase) *BalanceHandler {
	return &BalanceHandler{
		balanceUseCase: balanceUseCase,
	}
}

// Calculate handles balance calculation for a transformer
func (h *BalanceHandler) Calculate(w http.ResponseWriter, r *http.Request) {
	transformerID := chi.URLParam(r, "transformer_id")
	if transformerID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "transformer_id is required", nil))
		return
	}

	var req CalculateBalanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	periodStart := time.Now().AddDate(0, -1, 0)
	periodEnd := time.Now()

	if req.PeriodStart != "" {
		if parsed, err := time.Parse("2006-01-02", req.PeriodStart); err == nil {
			periodStart = parsed
		}
	}

	if req.PeriodEnd != "" {
		if parsed, err := time.Parse("2006-01-02", req.PeriodEnd); err == nil {
			periodEnd = parsed
		}
	}

	output, err := h.balanceUseCase.CalculateBalance(r.Context(), usecase.CalculateBalanceInput{
		TransformerID: domain.UUID(transformerID),
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("CALCULATION_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(output.Balance, "balance calculated successfully"))
}

// ListByTransformer handles listing balances for a transformer
func (h *BalanceHandler) ListByTransformer(w http.ResponseWriter, r *http.Request) {
	transformerID := chi.URLParam(r, "transformer_id")
	if transformerID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "transformer_id is required", nil))
		return
	}

	periodStartStr := r.URL.Query().Get("period_start")
	periodEndStr := r.URL.Query().Get("period_end")

	periodStart := time.Now().AddDate(0, -1, 0)
	periodEnd := time.Now()

	if periodStartStr != "" {
		if parsed, err := time.Parse("2006-01-02", periodStartStr); err == nil {
			periodStart = parsed
		}
	}

	if periodEndStr != "" {
		if parsed, err := time.Parse("2006-01-02", periodEndStr); err == nil {
			periodEnd = parsed
		}
	}

	balances, err := h.balanceUseCase.ListBalanceByTransformer(r.Context(), domain.UUID(transformerID), periodStart, periodEnd)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(balances, ""))
}

// Latest handles getting the latest balance for a transformer
func (h *BalanceHandler) Latest(w http.ResponseWriter, r *http.Request) {
	transformerID := chi.URLParam(r, "transformer_id")
	if transformerID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "transformer_id is required", nil))
		return
	}

	balance, err := h.balanceUseCase.GetLatestBalance(r.Context(), domain.UUID(transformerID))
	if err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "balance not found", nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(balance, ""))
}

// TechnicalLoss handles calculating technical losses according to PRODIST M7
func (h *BalanceHandler) TechnicalLoss(w http.ResponseWriter, r *http.Request) {
	transformerID := chi.URLParam(r, "transformer_id")
	if transformerID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "transformer_id is required", nil))
		return
	}

	periodStartStr := r.URL.Query().Get("period_start")
	periodEndStr := r.URL.Query().Get("period_end")

	periodStart := time.Now().AddDate(0, -1, 0)
	periodEnd := time.Now()

	if periodStartStr != "" {
		if parsed, err := time.Parse("2006-01-02", periodStartStr); err == nil {
			periodStart = parsed
		}
	}

	if periodEndStr != "" {
		if parsed, err := time.Parse("2006-01-02", periodEndStr); err == nil {
			periodEnd = parsed
		}
	}

	result, err := h.balanceUseCase.CalculateTechnicalLoss(r.Context(), usecase.CalculateBalanceInput{
		TransformerID: domain.UUID(transformerID),
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("CALCULATION_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(result, ""))
}
