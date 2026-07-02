package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/energybalance/server/internal/delivery/middleware"
	"github.com/energybalance/server/internal/delivery/request"
	"github.com/energybalance/server/internal/domain"
	"github.com/energybalance/server/internal/usecase"
)

// ConsumingUnitHandler handles consuming unit HTTP requests
type ConsumingUnitHandler struct {
	ucUseCase *usecase.ConsumingUnitUseCase
}

// CreateConsumingUnitRequest represents a consuming unit creation request
type CreateConsumingUnitRequest struct {
	TransformerID string `json:"transformer_id"`
	UCCode        string `json:"uc_code"`
	Name          string `json:"name"`
	Class         string `json:"class"`
	Active        bool   `json:"active"`
}

// UpdateConsumingUnitRequest represents a consuming unit update request
type UpdateConsumingUnitRequest struct {
	TransformerID string `json:"transformer_id"`
	UCCode        string `json:"uc_code"`
	Name          string `json:"name"`
	Class         string `json:"class"`
	Active        bool   `json:"active"`
}

// NewConsumingUnitHandler creates a new ConsumingUnitHandler
func NewConsumingUnitHandler(ucUseCase *usecase.ConsumingUnitUseCase) *ConsumingUnitHandler {
	return &ConsumingUnitHandler{
		ucUseCase: ucUseCase,
	}
}

// Create handles consuming unit creation
func (h *ConsumingUnitHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	var req CreateConsumingUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.UCCode == "" || req.TransformerID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "uc_code and transformer_id are required", nil))
		return
	}

	ucClass := domain.UCClassResidential
	if req.Class != "" {
		ucClass = domain.UCClass(req.Class)
	}

	uc, err := h.ucUseCase.CreateConsumingUnit(r.Context(), usecase.CreateConsumingUnitInput{
		TenantID:      tenantID,
		TransformerID: domain.UUID(req.TransformerID),
		UCCode:        req.UCCode,
		Name:          req.Name,
		Class:         ucClass,
		Active:        req.Active,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("CREATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusCreated, request.Success(uc, "consuming unit created successfully"))
}

// GetByID handles getting a consuming unit by ID
func (h *ConsumingUnitHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	uc, err := h.ucUseCase.GetConsumingUnitByID(r.Context(), domain.UUID(id))
	if err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "consuming unit not found", nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(uc, ""))
}

// List handles listing all consuming units for a tenant
func (h *ConsumingUnitHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	ucs, err := h.ucUseCase.ListConsumingUnits(r.Context(), tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(ucs, ""))
}

// ListByTransformer handles listing consuming units by transformer
func (h *ConsumingUnitHandler) ListByTransformer(w http.ResponseWriter, r *http.Request) {
	transformerID := chi.URLParam(r, "transformer_id")
	if transformerID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "transformer_id is required", nil))
		return
	}

	ucs, err := h.ucUseCase.ListConsumingUnitsByTransformer(r.Context(), domain.UUID(transformerID))
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(ucs, ""))
}

// Update handles consuming unit update
func (h *ConsumingUnitHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	var req UpdateConsumingUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	ucClass := domain.UCClassResidential
	if req.Class != "" {
		ucClass = domain.UCClass(req.Class)
	}

	uc, err := h.ucUseCase.UpdateConsumingUnit(r.Context(), usecase.UpdateConsumingUnitInput{
		ID:            domain.UUID(id),
		TransformerID: domain.UUID(req.TransformerID),
		UCCode:        req.UCCode,
		Name:          req.Name,
		Class:         ucClass,
		Active:        req.Active,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("UPDATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(uc, "consuming unit updated successfully"))
}

// Delete handles consuming unit deletion (soft delete)
// NOTE: This performs a soft delete by updating deleted_at, it does NOT remove the record from the database
// Following SDD 04-DESIGN-DETALHADO/01-modelo-de-dados.md convention
func (h *ConsumingUnitHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	if err := h.ucUseCase.DeleteConsumingUnit(r.Context(), domain.UUID(id)); err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("DELETE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(nil, "consuming unit deleted successfully"))
}
