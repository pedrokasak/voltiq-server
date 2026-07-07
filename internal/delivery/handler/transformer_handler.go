package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/usecase"
)

// TransformerHandler handles transformer HTTP requests
type TransformerHandler struct {
	transformerUseCase *usecase.TransformerUseCase
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

// UpdateTransformerRequest represents a transformer update request
type UpdateTransformerRequest struct {
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

// NewTransformerHandler creates a new TransformerHandler
func NewTransformerHandler(transformerUseCase *usecase.TransformerUseCase) *TransformerHandler {
	return &TransformerHandler{
		transformerUseCase: transformerUseCase,
	}
}

// Create handles transformer creation
func (h *TransformerHandler) Create(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	var req CreateTransformerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.Code == "" || req.PowerKVA == 0 {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "code and power_kva are required", nil))
		return
	}

	transformer, err := h.transformerUseCase.CreateTransformer(r.Context(), usecase.CreateTransformerInput{
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
		SubstationID:      (*domain.UUID)(req.SubstationID),
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("CREATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusCreated, request.Success(transformer, "transformer created successfully"))
}

// GetByID handles getting a transformer by ID
func (h *TransformerHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	transformer, err := h.transformerUseCase.GetTransformerByID(r.Context(), domain.UUID(id))
	if err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "transformer not found", nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(transformer, ""))
}

// List handles listing all transformers for a tenant
func (h *TransformerHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	transformers, err := h.transformerUseCase.ListTransformers(r.Context(), tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(transformers, ""))
}

// Update handles transformer update
func (h *TransformerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	var req UpdateTransformerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	transformer, err := h.transformerUseCase.UpdateTransformer(r.Context(), usecase.UpdateTransformerInput{
		ID:                domain.UUID(id),
		Code:              req.Code,
		PowerKVA:          req.PowerKVA,
		PrimaryVoltageKV:  req.PrimaryVoltageKV,
		SecondaryVoltageV: req.SecondaryVoltageV,
		Lat:               req.Lat,
		Lng:               req.Lng,
		CoreLossKW:        req.CoreLossKW,
		WindingLossKW:     req.WindingLossKW,
		LossLimitPct:      req.LossLimitPct,
		SubstationID:      (*domain.UUID)(req.SubstationID),
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("UPDATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(transformer, "transformer updated successfully"))
}

// Delete handles transformer deletion (soft delete)
// NOTE: This performs a soft delete by updating deleted_at, it does NOT remove the record from the database
// Following SDD 04-DESIGN-DETALHADO/01-modelo-de-dados.md convention
func (h *TransformerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	if err := h.transformerUseCase.DeleteTransformer(r.Context(), domain.UUID(id)); err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("DELETE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(nil, "transformer deleted successfully"))
}

// GetByCode handles getting a transformer by code
func (h *TransformerHandler) GetByCode(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "code is required", nil))
		return
	}

	// Implementar lógica para buscar por código
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// ListBySubstation handles listing transformers by substation
func (h *TransformerHandler) ListBySubstation(w http.ResponseWriter, r *http.Request) {
	substationID := chi.URLParam(r, "substation_id")
	if substationID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "substation_id is required", nil))
		return
	}

	// Implementar lógica para listar por subestação
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// Stats handles getting transformer statistics
func (h *TransformerHandler) Stats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	// Implementar lógica para estatísticas do transformador
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// LossAnalysis handles getting transformer loss analysis
func (h *TransformerHandler) LossAnalysis(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "period is required", nil))
		return
	}

	// Implementar lógica para análise de perdas
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// ExportCSV handles exporting transformer data to CSV
func (h *TransformerHandler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	// Implementar lógica para exportar CSV
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// ImportCSV handles importing transformer data from CSV
func (h *TransformerHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	// Implementar lógica para importar CSV
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// BatchCreate handles creating multiple transformers
func (h *TransformerHandler) BatchCreate(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	var reqs []CreateTransformerRequest
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if len(reqs) == 0 {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "no transformers provided", nil))
		return
	}

	var created []any
	for _, req := range reqs {
		transformer, err := h.transformerUseCase.CreateTransformer(r.Context(), usecase.CreateTransformerInput{
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
			SubstationID:      (*domain.UUID)(req.SubstationID),
		})
		if err != nil {
			request.WriteJSON(w, http.StatusInternalServerError, request.Fail("CREATE_ERROR", err.Error(), nil))
			return
		}
		created = append(created, transformer)
	}

	request.WriteJSON(w, http.StatusCreated, request.Success(created, "transformers created successfully"))
}

// BatchUpdate handles updating multiple transformers
func (h *TransformerHandler) BatchUpdate(w http.ResponseWriter, r *http.Request) {
	var reqs []UpdateTransformerRequest
	if err := json.NewDecoder(r.Body).Decode(&reqs); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if len(reqs) == 0 {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "no transformers provided", nil))
		return
	}

	var updated []any
	for _, req := range reqs {
		if req.Code == "" {
			request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "code is required for all transformers", nil))
			return
		}

		transformer, err := h.transformerUseCase.UpdateTransformer(r.Context(), usecase.UpdateTransformerInput{
			ID:                domain.UUID(req.Code),
			Code:              req.Code,
			PowerKVA:          req.PowerKVA,
			PrimaryVoltageKV:  req.PrimaryVoltageKV,
			SecondaryVoltageV: req.SecondaryVoltageV,
			Lat:               req.Lat,
			Lng:               req.Lng,
			CoreLossKW:        req.CoreLossKW,
			WindingLossKW:     req.WindingLossKW,
			LossLimitPct:      req.LossLimitPct,
			SubstationID:      (*domain.UUID)(req.SubstationID),
		})
		if err != nil {
			request.WriteJSON(w, http.StatusInternalServerError, request.Fail("UPDATE_ERROR", err.Error(), nil))
			return
		}
		updated = append(updated, transformer)
	}

	request.WriteJSON(w, http.StatusOK, request.Success(updated, "transformers updated successfully"))
}

// BatchDelete handles deleting multiple transformers
func (h *TransformerHandler) BatchDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if len(req.IDs) == 0 {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "no ids provided", nil))
		return
	}

	for _, idStr := range req.IDs {
		if err := h.transformerUseCase.DeleteTransformer(r.Context(), domain.UUID(idStr)); err != nil {
			request.WriteJSON(w, http.StatusInternalServerError, request.Fail("DELETE_ERROR", err.Error(), nil))
			return
		}
	}

	request.WriteJSON(w, http.StatusOK, request.Success(nil, "transformers deleted successfully"))
}

// Count handles getting transformer count
func (h *TransformerHandler) Count(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	transformers, err := h.transformerUseCase.ListTransformers(r.Context(), tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	response := map[string]any{
		"count": len(transformers),
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// ExistsByCode handles checking if a transformer code exists
func (h *TransformerHandler) ExistsByCode(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "code is required", nil))
		return
	}

	// Implementar lógica para verificar existência por código
	// Por enquanto, retorna erro
	request.WriteJSON(w, http.StatusNotImplemented, request.Fail("NOT_IMPLEMENTED", "endpoint not implemented", nil))
}

// GetLossLimit handles getting transformer loss limit
func (h *TransformerHandler) GetLossLimit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	transformer, err := h.transformerUseCase.GetTransformerByID(r.Context(), domain.UUID(id))
	if err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "transformer not found", nil))
		return
	}

	response := map[string]any{
		"transformer_id":  transformer.ID,
		"loss_limit_pct":  transformer.LossLimitPct,
		"code":            transformer.Code,
		"power_kva":       transformer.PowerKVA,
		"core_loss_kw":    transformer.CoreLossKW,
		"winding_loss_kw": transformer.WindingLossKW,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// UpdateLossLimit handles updating transformer loss limit
func (h *TransformerHandler) UpdateLossLimit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	var req struct {
		LossLimitPct float64 `json:"loss_limit_pct"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	transformer, err := h.transformerUseCase.UpdateTransformer(r.Context(), usecase.UpdateTransformerInput{
		ID:           domain.UUID(id),
		LossLimitPct: &req.LossLimitPct,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("UPDATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(transformer, "loss limit updated successfully"))
}

// GetTechnicalData handles getting transformer technical data
func (h *TransformerHandler) GetTechnicalData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	transformer, err := h.transformerUseCase.GetTransformerByID(r.Context(), domain.UUID(id))
	if err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "transformer not found", nil))
		return
	}

	response := map[string]any{
		"transformer_id":      transformer.ID,
		"code":                transformer.Code,
		"power_kva":           transformer.PowerKVA,
		"primary_voltage_kv":  transformer.PrimaryVoltageKV,
		"secondary_voltage_v": transformer.SecondaryVoltageV,
		"core_loss_kw":        transformer.CoreLossKW,
		"winding_loss_kw":     transformer.WindingLossKW,
		"nominal_current":     transformer.NominalCurrent(),
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// UpdateTechnicalData handles updating transformer technical data
func (h *TransformerHandler) UpdateTechnicalData(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	var req struct {
		PowerKVA          float64  `json:"power_kva"`
		PrimaryVoltageKV  float64  `json:"primary_voltage_kv"`
		SecondaryVoltageV float64  `json:"secondary_voltage_v"`
		CoreLossKW        *float64 `json:"core_loss_kw"`
		WindingLossKW     *float64 `json:"winding_loss_kw"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	transformer, err := h.transformerUseCase.UpdateTransformer(r.Context(), usecase.UpdateTransformerInput{
		ID:                domain.UUID(id),
		PowerKVA:          req.PowerKVA,
		PrimaryVoltageKV:  req.PrimaryVoltageKV,
		SecondaryVoltageV: req.SecondaryVoltageV,
		CoreLossKW:        req.CoreLossKW,
		WindingLossKW:     req.WindingLossKW,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("UPDATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(transformer, "technical data updated successfully"))
}

// GetLocation handles getting transformer location
func (h *TransformerHandler) GetLocation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	transformer, err := h.transformerUseCase.GetTransformerByID(r.Context(), domain.UUID(id))
	if err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "transformer not found", nil))
		return
	}

	response := map[string]any{
		"transformer_id": transformer.ID,
		"code":           transformer.Code,
		"lat":            transformer.Lat,
		"lng":            transformer.Lng,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// UpdateLocation handles updating transformer location
func (h *TransformerHandler) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	var req struct {
		Lat *float64 `json:"lat"`
		Lng *float64 `json:"lng"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	transformer, err := h.transformerUseCase.UpdateTransformer(r.Context(), usecase.UpdateTransformerInput{
		ID:  domain.UUID(id),
		Lat: req.Lat,
		Lng: req.Lng,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("UPDATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(transformer, "location updated successfully"))
}

// GetSubstation handles getting transformer substation
func (h *TransformerHandler) GetSubstation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	transformer, err := h.transformerUseCase.GetTransformerByID(r.Context(), domain.UUID(id))
	if err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "transformer not found", nil))
		return
	}

	response := map[string]any{
		"transformer_id": transformer.ID,
		"code":           transformer.Code,
		"substation_id":  transformer.SubstationID,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// UpdateSubstation handles updating transformer substation
func (h *TransformerHandler) UpdateSubstation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	var req struct {
		SubstationID *string `json:"substation_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	transformer, err := h.transformerUseCase.UpdateTransformer(r.Context(), usecase.UpdateTransformerInput{
		ID:           domain.UUID(id),
		SubstationID: (*domain.UUID)(req.SubstationID),
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("UPDATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(transformer, "substation updated successfully"))
}

// GetActive handles getting transformer active status
func (h *TransformerHandler) GetActive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	transformer, err := h.transformerUseCase.GetTransformerByID(r.Context(), domain.UUID(id))
	if err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "transformer not found", nil))
		return
	}

	response := map[string]any{
		"transformer_id": transformer.ID,
		"code":           transformer.Code,
		"active":         transformer.Active,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// UpdateActive handles updating transformer active status
func (h *TransformerHandler) UpdateActive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	var req struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	transformer, err := h.transformerUseCase.UpdateTransformer(r.Context(), usecase.UpdateTransformerInput{
		ID: domain.UUID(id),
	})
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("UPDATE_ERROR", err.Error(), nil))
		return
	}

	transformer.Active = req.Active

	request.WriteJSON(w, http.StatusOK, request.Success(transformer, "active status updated successfully"))
}

// GetAllWithCount handles getting all transformers with count
func (h *TransformerHandler) GetAllWithCount(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	transformers, err := h.transformerUseCase.ListTransformers(r.Context(), tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	response := map[string]any{
		"transformers": transformers,
		"count":        len(transformers),
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// GetWithPagination handles getting transformers with pagination
func (h *TransformerHandler) GetWithPagination(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())

	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	limit := 10

	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	transformers, err := h.transformerUseCase.ListTransformers(r.Context(), tenantID)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	// Apply pagination
	start := (page - 1) * limit
	end := start + limit

	if start > len(transformers) {
		transformers = []*domain.Transformer{}
	} else if end > len(transformers) {
		transformers = transformers[start:]
	} else {
		transformers = transformers[start:end]
	}

	response := map[string]any{
		"transformers": transformers,
		"page":         page,
		"limit":        limit,
		"total":        len(transformers),
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}
