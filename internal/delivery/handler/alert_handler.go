package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/usecase"
)

// AlertHandler handles alert configuration HTTP requests
type AlertHandler struct {
	alertUseCase *usecase.AlertUseCase
}

// AlertConfigRequest represents an alert configuration request
type AlertConfigRequest struct {
	TransformerID string `json:"transformer_id"`
	Type          string `json:"type"`   // WARNING, CRITICAL
	Channel       string `json:"channel"` // EMAIL, WHATSAPP
	Recipient     string `json:"recipient"`
}

// NewAlertHandler creates a new AlertHandler
func NewAlertHandler(alertUseCase *usecase.AlertUseCase) *AlertHandler {
	return &AlertHandler{
		alertUseCase: alertUseCase,
	}
}

// Create handles creating an alert configuration
func (h *AlertHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	var req AlertConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.TransformerID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "transformer_id is required", nil))
		return
	}

	h.createAlertFor(w, r, tenantID, domain.UUID(req.TransformerID), req)
}

// CreateForTransformer handles POST /api/v1/transformers/{id}/alert-config.
// The transformer ID is read from the URL path, so the body does not need it.
func (h *AlertHandler) CreateForTransformer(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	transformerID := chi.URLParam(r, "id")
	if transformerID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	var req AlertConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	h.createAlertFor(w, r, tenantID, domain.UUID(transformerID), req)
}

// createAlertFor centralizes validation and the usecase call for both Create paths
func (h *AlertHandler) createAlertFor(
	w http.ResponseWriter,
	r *http.Request,
	tenantID domain.UUID,
	transformerID domain.UUID,
	req AlertConfigRequest,
) {
	if req.Type == "" || req.Channel == "" || req.Recipient == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "type, channel, and recipient are required", nil))
		return
	}

	// Validate type
	if req.Type != string(domain.AlertTypeWarning) && req.Type != string(domain.AlertTypeCritical) {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "type must be WARNING or CRITICAL", nil))
		return
	}

	// Validate channel
	if req.Channel != string(domain.AlertChannelEmail) && req.Channel != string(domain.AlertChannelWhatsapp) {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "channel must be EMAIL or WHATSAPP", nil))
		return
	}

	alert, err := h.alertUseCase.CreateAlert(r.Context(), usecase.CreateAlertInput{
		TenantID:      tenantID,
		TransformerID: transformerID,
		Type:          domain.AlertType(req.Type),
		Channel:       domain.AlertChannel(req.Channel),
		Recipient:     req.Recipient,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("CREATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusCreated, request.Success(alert, "alert configuration created successfully"))
}

// GetByTransformer handles getting alert configurations for a transformer
func (h *AlertHandler) GetByTransformer(w http.ResponseWriter, r *http.Request) {
	transformerID := chi.URLParam(r, "transformer_id")
	if transformerID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "transformer_id is required", nil))
		return
	}

	alerts, err := h.alertUseCase.GetAlertsByTransformer(r.Context(), domain.UUID(transformerID))
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(alerts, ""))
}

// GetByID handles getting an alert configuration by ID
func (h *AlertHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	alert, err := h.alertUseCase.GetAlertByID(r.Context(), domain.UUID(id))
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("GET_ERROR", err.Error(), nil))
		return
	}

	if alert == nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "alert not found", nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(alert, ""))
}

// Update handles updating an alert configuration
func (h *AlertHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	var req AlertConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.Type != "" && req.Type != string(domain.AlertTypeWarning) && req.Type != string(domain.AlertTypeCritical) {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "type must be WARNING or CRITICAL", nil))
		return
	}

	if req.Channel != "" && req.Channel != string(domain.AlertChannelEmail) && req.Channel != string(domain.AlertChannelWhatsapp) {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "channel must be EMAIL or WHATSAPP", nil))
		return
	}

	alert, err := h.alertUseCase.UpdateAlert(r.Context(), usecase.UpdateAlertInput{
		ID:        domain.UUID(id),
		Type:      domain.AlertType(req.Type),
		Channel:   domain.AlertChannel(req.Channel),
		Recipient: req.Recipient,
	})
	if err != nil {
		if err == usecase.ErrAlertNotFound {
			request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "alert not found", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("UPDATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(alert, "alert configuration updated successfully"))
}

// Delete handles deleting an alert configuration
func (h *AlertHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	if err := h.alertUseCase.DeleteAlert(r.Context(), domain.UUID(id)); err != nil {
		if err == usecase.ErrAlertNotFound {
			request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "alert not found", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("DELETE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(nil, "alert configuration deleted successfully"))
}

// ListByTenant handles listing all alert configurations for a tenant
func (h *AlertHandler) ListByTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	alerts, err := h.alertUseCase.ListAlertsByTenant(r.Context(), tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(alerts, ""))
}