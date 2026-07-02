package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/energybalance/server/internal/domain"
	"github.com/energybalance/server/internal/repository"
)

// ConsumingUnitHandler handles consuming unit CRUD requests
type ConsumingUnitHandler struct {
	ucRepo *repository.ConsumingUnitRepository
}

// NewConsumingUnitHandler creates a new ConsumingUnitHandler
func NewConsumingUnitHandler(ucRepo *repository.ConsumingUnitRepository) *ConsumingUnitHandler {
	return &ConsumingUnitHandler{ucRepo: ucRepo}
}

// CreateConsumingUnitRequest represents a consuming unit creation request
type CreateConsumingUnitRequest struct {
	UCCode        string `json:"uc_code"`
	Name          string `json:"name"`
	Class         string `json:"class"`
	TransformerID string `json:"transformer_id"`
	Active        bool   `json:"active"`
}

// Create handles consuming unit creation
func (h *ConsumingUnitHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID := GetTenantID(r.Context())

	var req CreateConsumingUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	ucID := domain.UUID(uuid.New().String())
	now := time.Now()

	ucClass := domain.UCClass(req.Class)

	uc := &domain.ConsumingUnit{
		ID:            ucID,
		TenantID:      tenantID,
		TransformerID: domain.UUID(req.TransformerID),
		UCCode:        req.UCCode,
		Name:          req.Name,
		Class:         &ucClass,
		Active:        req.Active,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := h.ucRepo.Create(r.Context(), uc); err != nil {
		http.Error(w, "failed to create consuming unit", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(uc)
}

// GetByID handles getting a consuming unit by ID
func (h *ConsumingUnitHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	uc, err := h.ucRepo.GetByID(r.Context(), domain.UUID(id))
	if err != nil {
		http.Error(w, "failed to get consuming unit", http.StatusInternalServerError)
		return
	}

	if uc == nil {
		http.Error(w, "consuming unit not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(uc)
}

// ListByTransformer handles listing consuming units by transformer
func (h *ConsumingUnitHandler) ListByTransformer(w http.ResponseWriter, r *http.Request) {
	transformerID := r.URL.Query().Get("transformer_id")
	if transformerID == "" {
		http.Error(w, "missing transformer_id parameter", http.StatusBadRequest)
		return
	}

	ucs, err := h.ucRepo.GetByTransformer(r.Context(), domain.UUID(transformerID))
	if err != nil {
		http.Error(w, "failed to list consuming units", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ucs)
}

// List handles listing all consuming units for a tenant
func (h *ConsumingUnitHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := GetTenantID(r.Context())

	ucs, err := h.ucRepo.GetByTenant(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "failed to list consuming units", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ucs)
}

// Update handles consuming unit update
func (h *ConsumingUnitHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	uc, err := h.ucRepo.GetByID(r.Context(), domain.UUID(id))
	if err != nil {
		http.Error(w, "failed to get consuming unit", http.StatusInternalServerError)
		return
	}

	if uc == nil {
		http.Error(w, "consuming unit not found", http.StatusNotFound)
		return
	}

	var req CreateConsumingUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	uc.TransformerID = domain.UUID(req.TransformerID)
	uc.UCCode = req.UCCode
	uc.Name = req.Name
	ucClass := domain.UCClass(req.Class)
	uc.Class = &ucClass
	uc.Active = req.Active

	if err := h.ucRepo.Update(r.Context(), uc); err != nil {
		http.Error(w, "failed to update consuming unit", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(uc)
}

// Delete handles consuming unit deletion (soft delete)
func (h *ConsumingUnitHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	if err := h.ucRepo.Delete(r.Context(), domain.UUID(id)); err != nil {
		http.Error(w, "failed to delete consuming unit", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
