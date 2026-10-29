package handler

import (
	"net/http"
	"time"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/usecase"
)

// BalanceHandler handles energy balance HTTP requests
type BalanceHandler struct {
	balanceUseCase *usecase.BalanceUseCase
	reports        *usecase.ReportUseCase
}

// CalculateBalanceRequest represents a balance calculation request.
// Energy values are optional manual inputs (RF02.5); without them the
// readings of the period are used.
type CalculateBalanceRequest struct {
	PeriodStart         string   `json:"period_start"`
	PeriodEnd           string   `json:"period_end"`
	EnergyInjectedKWh   *float64 `json:"energy_injected_kwh"`
	TotalConsumptionKWh *float64 `json:"total_consumption_kwh"`
	// Aliases used by the web client.
	EnergySuppliedKWh *float64 `json:"energy_supplied_kwh"`
	EnergyBilledKWh   *float64 `json:"energy_billed_kwh"`
}

// NewBalanceHandler creates a new BalanceHandler
func NewBalanceHandler(balanceUseCase *usecase.BalanceUseCase, reports *usecase.ReportUseCase) *BalanceHandler {
	return &BalanceHandler{balanceUseCase: balanceUseCase, reports: reports}
}

// defaultPeriod is the previous calendar month.
func defaultPeriod() (time.Time, time.Time) {
	now := time.Now()
	firstThis := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return firstThis.AddDate(0, -1, 0), firstThis.AddDate(0, 0, -1)
}

func (h *BalanceHandler) role(r *http.Request) domain.UserRole {
	return domain.UserRole(middleware.GetRole(r.Context()))
}

// Calculate handles balance calculation for a transformer
func (h *BalanceHandler) Calculate(w http.ResponseWriter, r *http.Request) {
	transformerID, ok := pathUUID(w, r, "transformer_id")
	if !ok {
		return
	}

	var req CalculateBalanceRequest
	if r.ContentLength != 0 && !decodeJSON(w, r, &req) {
		return
	}

	defStart, defEnd := defaultPeriod()
	in := usecase.CalculateBalanceInput{
		TransformerID:        transformerID,
		PeriodStart:          parseDate(req.PeriodStart, defStart),
		PeriodEnd:            parseDate(req.PeriodEnd, defEnd),
		ManualInjectedKWh:    firstNonNil(req.EnergyInjectedKWh, req.EnergySuppliedKWh),
		ManualConsumptionKWh: firstNonNil(req.TotalConsumptionKWh, req.EnergyBilledKWh),
	}
	for _, v := range []*float64{in.ManualInjectedKWh, in.ManualConsumptionKWh} {
		if v != nil && *v < 0 {
			request.WriteJSON(w, http.StatusUnprocessableEntity, request.Fail("VALIDATION_ERROR", "energia não pode ser negativa", nil))
			return
		}
	}
	if in.ManualInjectedKWh != nil && *in.ManualInjectedKWh == 0 {
		request.WriteJSON(w, http.StatusUnprocessableEntity, request.Fail("VALIDATION_ERROR", "energia injetada deve ser maior que zero", nil))
		return
	}

	output, err := h.balanceUseCase.CalculateBalance(r.Context(), in)
	if err != nil {
		writeUseCaseError(w, err, "CALCULATION_ERROR")
		return
	}

	response := map[string]any{
		"balance": middleware.GetBalanceDTO(output.Balance, h.role(r)),
		"method":  output.Method,
	}
	// Campos do balanco no nivel raiz mantem o contrato do SDD.
	if dto, ok := middleware.GetBalanceDTO(output.Balance, h.role(r)).(*middleware.BalanceResponseDTO); ok {
		response["id"] = dto.ID
		response["status"] = dto.Status
		response["loss_pct"] = dto.LossPct
		response["loss_kwh"] = dto.LossKWh
		response["energy_injected_kwh"] = dto.EnergyInjectedKWh
		response["total_consumption_kwh"] = dto.TotalConsumptionKWh
		response["technical_loss_kwh"] = dto.TechnicalLossKWh
		response["non_technical_loss_kwh"] = dto.NonTechnicalLossKWh
		response["limit_pct"] = dto.LossLimitPct
		response["uc_count"] = dto.UCCount
		response["period_start"] = dto.PeriodStart
		response["period_end"] = dto.PeriodEnd
		response["calculated_at"] = dto.CalculatedAt
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, "balance calculated successfully"))
}

func firstNonNil(vals ...*float64) *float64 {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

// CalculateAll recalculates every active transformer of the tenant for a
// period (POST /balance/calculate-all {period_start, period_end}).
func (h *BalanceHandler) CalculateAll(w http.ResponseWriter, r *http.Request) {
	var req CalculateBalanceRequest
	if r.ContentLength != 0 && !decodeJSON(w, r, &req) {
		return
	}
	defStart, defEnd := defaultPeriod()
	res, err := h.balanceUseCase.CalculateAll(r.Context(), middleware.GetTenantID(r.Context()),
		parseDate(req.PeriodStart, defStart), parseDate(req.PeriodEnd, defEnd))
	if err != nil {
		writeUseCaseError(w, err, "CALCULATION_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(res, "balances recalculated"))
}

// ListByTransformer handles listing balances for a transformer
// (?period_start=&period_end=, default last 12 months).
func (h *BalanceHandler) ListByTransformer(w http.ResponseWriter, r *http.Request) {
	transformerID, ok := pathUUID(w, r, "transformer_id")
	if !ok {
		return
	}
	end := queryDate(r, "period_end", time.Now())
	start := queryDate(r, "period_start", end.AddDate(-1, 0, 0))

	balances, err := h.balanceUseCase.ListBalanceByTransformer(r.Context(), transformerID, start, end)
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}

	role := h.role(r)
	response := make([]any, 0, len(balances))
	for _, b := range balances {
		response = append(response, middleware.GetBalanceDTO(b, role))
	}
	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// Latest handles getting the latest balance for a transformer
func (h *BalanceHandler) Latest(w http.ResponseWriter, r *http.Request) {
	transformerID, ok := pathUUID(w, r, "transformer_id")
	if !ok {
		return
	}
	balance, err := h.balanceUseCase.GetLatestBalance(r.Context(), transformerID)
	if err != nil {
		writeUseCaseError(w, err, "GET_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(middleware.GetBalanceDTO(balance, h.role(r)), ""))
}

// TechnicalLoss calculates technical losses (PRODIST M7) without storing
func (h *BalanceHandler) TechnicalLoss(w http.ResponseWriter, r *http.Request) {
	transformerID, ok := pathUUID(w, r, "transformer_id")
	if !ok {
		return
	}
	defStart, defEnd := defaultPeriod()
	result, err := h.balanceUseCase.CalculateTechnicalLoss(r.Context(), usecase.CalculateBalanceInput{
		TransformerID: transformerID,
		PeriodStart:   queryDate(r, "period_start", defStart),
		PeriodEnd:     queryDate(r, "period_end", defEnd),
	})
	if err != nil {
		writeUseCaseError(w, err, "CALCULATION_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(result, ""))
}

// LossAnalysis returns the monthly loss breakdown of a transformer
// (GET /transformers/{id}/loss-analysis?months=12).
func (h *BalanceHandler) LossAnalysis(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	res, err := h.reports.TransformerLossAnalysis(r.Context(), id, queryInt(r, "months", 12), h.role(r))
	if err != nil {
		writeUseCaseError(w, err, "ANALYSIS_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(res, ""))
}

// ReadingSeries returns the energy series of a transformer
// (GET /transformers/{id}/readings?bucket=day|week|month&period_start=&period_end=).
func (h *BalanceHandler) ReadingSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	end := queryDate(r, "period_end", time.Now())
	start := queryDate(r, "period_start", end.AddDate(-1, 0, 0))
	res, err := h.reports.TransformerSeries(r.Context(), id, start, end, r.URL.Query().Get("bucket"))
	if err != nil {
		writeUseCaseError(w, err, "SERIES_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(res, ""))
}

// TopConsumers returns the UCs of a transformer ordered by consumption in a
// period (GET /transformers/{id}/consumers?period_start=&period_end=&limit=).
func (h *BalanceHandler) TopConsumers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	defStart, defEnd := defaultPeriod()
	res, err := h.reports.TopConsumers(r.Context(), id,
		queryDate(r, "period_start", defStart), queryDate(r, "period_end", defEnd), queryInt(r, "limit", 50))
	if err != nil {
		writeUseCaseError(w, err, "CONSUMERS_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(res, ""))
}
