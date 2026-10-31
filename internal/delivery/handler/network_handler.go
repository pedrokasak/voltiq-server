package handler

import (
	"net/http"
	"time"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/usecase"
)

// NetworkHandler serves substations and manual readings.
type NetworkHandler struct {
	network *usecase.NetworkUseCase
}

// NewNetworkHandler creates a NetworkHandler
func NewNetworkHandler(network *usecase.NetworkUseCase) *NetworkHandler {
	return &NetworkHandler{network: network}
}

type substationRequest struct {
	Code string   `json:"code"`
	Name string   `json:"name"`
	Lat  *float64 `json:"lat"`
	Lng  *float64 `json:"lng"`
}

func (req substationRequest) input() usecase.SubstationInput {
	return usecase.SubstationInput{Code: req.Code, Name: req.Name, Lat: req.Lat, Lng: req.Lng}
}

// ListSubstations handles GET /substations
func (h *NetworkHandler) ListSubstations(w http.ResponseWriter, r *http.Request) {
	items, err := h.network.ListSubstations(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(items, ""))
}

// GetSubstation handles GET /substations/{id}
func (h *NetworkHandler) GetSubstation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	s, err := h.network.GetSubstation(r.Context(), id)
	if err != nil {
		writeUseCaseError(w, err, "GET_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(s, ""))
}

// CreateSubstation handles POST /substations
func (h *NetworkHandler) CreateSubstation(w http.ResponseWriter, r *http.Request) {
	var req substationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	s, err := h.network.CreateSubstation(r.Context(), middleware.GetTenantID(r.Context()), req.input())
	if err != nil {
		writeUseCaseError(w, err, "CREATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusCreated, request.Success(s, "subestação cadastrada"))
}

// UpdateSubstation handles PUT /substations/{id}
func (h *NetworkHandler) UpdateSubstation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req substationRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	s, err := h.network.UpdateSubstation(r.Context(), id, req.input())
	if err != nil {
		writeUseCaseError(w, err, "UPDATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(s, "subestação atualizada"))
}

// DeleteSubstation handles DELETE /substations/{id}
func (h *NetworkHandler) DeleteSubstation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.network.DeleteSubstation(r.Context(), id); err != nil {
		writeUseCaseError(w, err, "DELETE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(nil, "subestação removida"))
}

type manualReadingRequest struct {
	ReadingAt      string   `json:"reading_at"`
	EnergyKWh      *float64 `json:"energy_kwh"`
	ConsumptionKWh *float64 `json:"consumption_kwh"`
	DemandKW       *float64 `json:"demand_kw"`
	PowerFactor    *float64 `json:"power_factor"`
	Recalculate    *bool    `json:"recalculate"`
}

func (req manualReadingRequest) input() usecase.ManualReadingInput {
	in := usecase.ManualReadingInput{DemandKW: req.DemandKW, PowerFactor: req.PowerFactor, Recalculate: true, EnergyKWh: -1}
	if req.Recalculate != nil {
		in.Recalculate = *req.Recalculate
	}
	switch {
	case req.EnergyKWh != nil:
		in.EnergyKWh = *req.EnergyKWh
	case req.ConsumptionKWh != nil:
		in.EnergyKWh = *req.ConsumptionKWh
	}
	loc := domain.BusinessLocation
	var err error
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		var t time.Time
		if layout == time.RFC3339 {
			t, err = time.Parse(layout, req.ReadingAt)
		} else {
			t, err = time.ParseInLocation(layout, req.ReadingAt, loc)
		}
		if err == nil {
			in.ReadingAt = t
			break
		}
	}
	return in
}

// AddTransformerReading handles POST /transformers/{id}/readings
func (h *NetworkHandler) AddTransformerReading(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req manualReadingRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := h.network.AddTransformerReading(r.Context(), middleware.GetTenantID(r.Context()), id, req.input())
	if err != nil {
		writeUseCaseError(w, err, "READING_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusCreated, request.Success(res, res.Message))
}

// AddUCReading handles POST /consuming-units/{id}/readings
func (h *NetworkHandler) AddUCReading(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req manualReadingRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := h.network.AddUCReading(r.Context(), middleware.GetTenantID(r.Context()), id, req.input())
	if err != nil {
		writeUseCaseError(w, err, "READING_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusCreated, request.Success(res, res.Message))
}
