package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// TransformerHandler handles transformer CRUD requests
type TransformerHandler struct {
	transformerRepo *repository.TransformerRepository
}

// NewTransformerHandler creates a new TransformerHandler
func NewTransformerHandler(transformerRepo *repository.TransformerRepository) *TransformerHandler {
	return &TransformerHandler{transformerRepo: transformerRepo}
}

// CreateTransformerRequest represents a transformer creation request
type CreateTransformerRequest struct {
	Code              string   `json:"code"`
	PowerKVA          float64  `json:"power_kva"`
	PrimaryVoltageKV  float64  `json:"primary_voltage_kv"`
	SecondaryVoltageV float64  `json:"secondary_voltage_v"`
	Lat               *float64 `json:"lat"`
	Lng               *float64 `json:"lng"`
	CoreLossKW        *float64 `json:"core_loss_kw"`
	WindingLossKW     *float64 `json:"winding_loss_kw"`
	LossLimitPct      *float64 `json:"loss_limit_pct"`
	SubstationID      *string  `json:"substation_id"`
}

// Create handles transformer creation
func (h *TransformerHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID := GetTenantID(r.Context())

	var req CreateTransformerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	transformerID := domain.UUID(uuid.New().String())
	now := time.Now()

	transformer := &domain.Transformer{
		ID:                transformerID,
		TenantID:          tenantID,
		Code:              req.Code,
		PowerKVA:          req.PowerKVA,
		PrimaryVoltageKV:  req.PrimaryVoltageKV,
		SecondaryVoltageV: req.SecondaryVoltageV,
		Lat:               req.Lat,
		Lng:               req.Lng,
		CoreLossKW:        req.CoreLossKW,
		WindingLossKW:     req.WindingLossKW,
		LossLimitPct:      req.LossLimitPct,
		Active:            true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if req.SubstationID != nil {
		transformer.SubstationID = (*domain.UUID)(req.SubstationID)
	}

	if err := h.transformerRepo.Create(r.Context(), transformer); err != nil {
		http.Error(w, "failed to create transformer", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(transformer)
}

// GetByID handles getting a transformer by ID
func (h *TransformerHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	transformer, err := h.transformerRepo.GetByID(r.Context(), domain.UUID(id))
	if err != nil {
		http.Error(w, "failed to get transformer", http.StatusInternalServerError)
		return
	}

	if transformer == nil {
		http.Error(w, "transformer not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(transformer)
}

// List handles listing all transformers for a tenant
func (h *TransformerHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := GetTenantID(r.Context())

	transformers, err := h.transformerRepo.GetByTenant(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "failed to list transformers", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(transformers)
}

// Update handles transformer update
func (h *TransformerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	transformer, err := h.transformerRepo.GetByID(r.Context(), domain.UUID(id))
	if err != nil {
		http.Error(w, "failed to get transformer", http.StatusInternalServerError)
		return
	}

	if transformer == nil {
		http.Error(w, "transformer not found", http.StatusNotFound)
		return
	}

	var req CreateTransformerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	transformer.Code = req.Code
	transformer.PowerKVA = req.PowerKVA
	transformer.PrimaryVoltageKV = req.PrimaryVoltageKV
	transformer.SecondaryVoltageV = req.SecondaryVoltageV
	transformer.Lat = req.Lat
	transformer.Lng = req.Lng
	transformer.CoreLossKW = req.CoreLossKW
	transformer.WindingLossKW = req.WindingLossKW
	transformer.LossLimitPct = req.LossLimitPct

	if req.SubstationID != nil {
		transformer.SubstationID = (*domain.UUID)(req.SubstationID)
	}

	if err := h.transformerRepo.Update(r.Context(), transformer); err != nil {
		http.Error(w, "failed to update transformer", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(transformer)
}

// Delete handles transformer deletion (soft delete)
func (h *TransformerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	if err := h.transformerRepo.Delete(r.Context(), domain.UUID(id)); err != nil {
		http.Error(w, "failed to delete transformer", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
