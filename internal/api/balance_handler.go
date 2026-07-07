package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// BalanceHandler handles balance calculation requests
type BalanceHandler struct {
	balanceRepo *repository.BalanceRepository
}

// NewBalanceHandler creates a new BalanceHandler
func NewBalanceHandler(
	balanceRepo *repository.BalanceRepository,
) *BalanceHandler {
	return &BalanceHandler{
		balanceRepo: balanceRepo,
	}
}

// CalculateRequest represents a balance calculation request
type CalculateRequest struct {
	TransformerID domain.UUID `json:"transformer_id"`
	PeriodStart   time.Time   `json:"period_start"`
	PeriodEnd     time.Time   `json:"period_end"`
}

// Calculate handles balance calculation for a transformer
func (h *BalanceHandler) Calculate(w http.ResponseWriter, r *http.Request) {
	tenantID := GetTenantID(r.Context())

	transformerID := r.URL.Query().Get("transformer_id")
	if transformerID == "" {
		http.Error(w, "missing transformer_id parameter", http.StatusBadRequest)
		return
	}

	periodStartStr := r.URL.Query().Get("period_start")
	periodEndStr := r.URL.Query().Get("period_end")

	var periodStart, periodEnd time.Time
	var err error

	if periodStartStr != "" {
		periodStart, err = time.Parse("2006-01-02", periodStartStr)
		if err != nil {
			http.Error(w, "invalid period_start format, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
	} else {
		periodStart = time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Now().Location())
	}

	if periodEndStr != "" {
		periodEnd, err = time.Parse("2006-01-02", periodEndStr)
		if err != nil {
			http.Error(w, "invalid period_end format, use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
	} else {
		periodEnd = time.Now()
	}

	balances, err := h.balanceRepo.GetByTransformerAndPeriod(
		r.Context(),
		domain.UUID(transformerID),
		periodStart,
		periodEnd,
	)
	if err != nil {
		http.Error(w, "failed to calculate balance: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]any{
		"tenant_id":      tenantID,
		"transformer_id": transformerID,
		"period_start":   periodStart,
		"period_end":     periodEnd,
		"balances":       balances,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListByTransformer lists balances for a transformer
func (h *BalanceHandler) ListByTransformer(w http.ResponseWriter, r *http.Request) {
	transformerID := r.URL.Query().Get("transformer_id")
	if transformerID == "" {
		http.Error(w, "missing transformer_id parameter", http.StatusBadRequest)
		return
	}

	periodStartStr := r.URL.Query().Get("period_start")
	periodEndStr := r.URL.Query().Get("period_end")

	var periodStart, periodEnd time.Time
	var err error

	if periodStartStr != "" {
		periodStart, err = time.Parse("2006-01-02", periodStartStr)
		if err != nil {
			http.Error(w, "invalid period_start format", http.StatusBadRequest)
			return
		}
	} else {
		periodStart = time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Now().Location())
	}

	if periodEndStr != "" {
		periodEnd, err = time.Parse("2006-01-02", periodEndStr)
		if err != nil {
			http.Error(w, "invalid period_end format", http.StatusBadRequest)
			return
		}
	} else {
		periodEnd = time.Now()
	}

	balances, err := h.balanceRepo.GetByTransformerAndPeriod(r.Context(), domain.UUID(transformerID), periodStart, periodEnd)
	if err != nil {
		http.Error(w, "failed to list balances", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(balances)
}

// Latest gets the latest balance for a transformer
func (h *BalanceHandler) Latest(w http.ResponseWriter, r *http.Request) {
	transformerID := r.URL.Query().Get("transformer_id")
	if transformerID == "" {
		http.Error(w, "missing transformer_id parameter", http.StatusBadRequest)
		return
	}

	balance, err := h.balanceRepo.GetLatestByTransformer(r.Context(), domain.UUID(transformerID))
	if err != nil {
		http.Error(w, "failed to get latest balance", http.StatusInternalServerError)
		return
	}

	if balance == nil {
		http.Error(w, "no balance found for transformer", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(balance)
}
