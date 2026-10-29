package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/usecase"
)

// TransformerHandler handles transformer HTTP requests
type TransformerHandler struct {
	transformerUseCase *usecase.TransformerUseCase
	registryImport     *usecase.RegistryImportUseCase
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

// UpdateTransformerRequest is a partial update: omitted fields are kept.
type UpdateTransformerRequest struct {
	ID                string   `json:"id,omitempty"`
	Code              *string  `json:"code"`
	PowerKVA          *float64 `json:"power_kva"`
	PrimaryVoltageKV  *float64 `json:"primary_voltage_kv"`
	SecondaryVoltageV *float64 `json:"secondary_voltage_v"`
	Lat               *float64 `json:"lat"`
	Lng               *float64 `json:"lng"`
	CoreLossKW        *float64 `json:"core_loss_kw"`
	WindingLossKW     *float64 `json:"winding_loss_kw"`
	LossLimitPct      *float64 `json:"loss_limit_pct"`
	SubstationID      *string  `json:"substation_id"`
	Active            *bool    `json:"active"`
}

func (req UpdateTransformerRequest) toInput(id domain.UUID) usecase.UpdateTransformerInput {
	in := usecase.UpdateTransformerInput{
		ID:                id,
		Code:              req.Code,
		PowerKVA:          req.PowerKVA,
		PrimaryVoltageKV:  req.PrimaryVoltageKV,
		SecondaryVoltageV: req.SecondaryVoltageV,
		Lat:               req.Lat,
		Lng:               req.Lng,
		CoreLossKW:        req.CoreLossKW,
		WindingLossKW:     req.WindingLossKW,
		LossLimitPct:      req.LossLimitPct,
		Active:            req.Active,
	}
	if req.SubstationID != nil {
		if *req.SubstationID == "" {
			in.ClearSubstation = true
		} else {
			sid := domain.UUID(*req.SubstationID)
			in.SubstationID = &sid
		}
	}
	return in
}

func (req CreateTransformerRequest) toInput(tenantID domain.UUID) usecase.CreateTransformerInput {
	in := usecase.CreateTransformerInput{
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
	}
	if req.SubstationID != nil && *req.SubstationID != "" {
		sid := domain.UUID(*req.SubstationID)
		in.SubstationID = &sid
	}
	return in
}

// NewTransformerHandler creates a new TransformerHandler
func NewTransformerHandler(transformerUseCase *usecase.TransformerUseCase, registryImport *usecase.RegistryImportUseCase) *TransformerHandler {
	return &TransformerHandler{
		transformerUseCase: transformerUseCase,
		registryImport:     registryImport,
	}
}

// Create handles transformer creation
func (h *TransformerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateTransformerRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	transformer, err := h.transformerUseCase.CreateTransformer(r.Context(), req.toInput(middleware.GetTenantID(r.Context())))
	if err != nil {
		writeUseCaseError(w, err, "CREATE_ERROR")
		return
	}

	request.WriteJSON(w, http.StatusCreated, request.Success(transformer, "transformer created successfully"))
}

// GetByID handles getting a transformer by ID
func (h *TransformerHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}

	transformer, err := h.transformerUseCase.GetTransformerByID(r.Context(), id)
	if err != nil {
		writeUseCaseError(w, err, "GET_ERROR")
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(transformer, ""))
}

// filterFromQuery reads listing filters: search, status, substation_id,
// group_id, active, order_by, limit, offset/page.
func filterFromQuery(r *http.Request) (repository.TransformerFilter, int) {
	q := r.URL.Query()
	f := repository.TransformerFilter{
		Search:       strings.TrimSpace(q.Get("search")),
		Status:       strings.ToUpper(q.Get("status")),
		SubstationID: q.Get("substation_id"),
		GroupID:      q.Get("group_id"),
		OrderBy:      q.Get("order_by"),
		Limit:        queryInt(r, "limit", 0),
		Offset:       queryInt(r, "offset", 0),
	}
	if f.SubstationID != "" && !isUUID(f.SubstationID) {
		f.SubstationID = "00000000-0000-0000-0000-000000000000"
	}
	if f.GroupID != "" && !isUUID(f.GroupID) {
		f.GroupID = "00000000-0000-0000-0000-000000000000"
	}
	if a := q.Get("active"); a != "" {
		v := a == "true"
		f.Active = &v
	}
	if f.Limit > 500 {
		f.Limit = 500
	}
	pageNum := queryInt(r, "page", 0)
	if pageNum > 0 && f.Limit > 0 {
		f.Offset = (pageNum - 1) * f.Limit
	}
	return f, pageNum
}

// List returns the tenant's transformers with their latest balance status.
// Filters: search, status (NORMAL|WARNING|CRITICAL|NO_DATA), substation_id,
// group_id, active; pagination via limit/offset (total in X-Total-Count).
func (h *TransformerHandler) List(w http.ResponseWriter, r *http.Request) {
	f, _ := filterFromQuery(r)
	items, total, err := h.transformerUseCase.ListSummaries(r.Context(), middleware.GetTenantID(r.Context()), f)
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	request.WriteJSON(w, http.StatusOK, request.Success(items, ""))
}

// GetWithPagination returns {data, total, page, limit, total_pages}.
func (h *TransformerHandler) GetWithPagination(w http.ResponseWriter, r *http.Request) {
	f, pageNum := filterFromQuery(r)
	if f.Limit == 0 {
		f.Limit = 20
	}
	if pageNum == 0 {
		pageNum = f.Offset/f.Limit + 1
		f.Offset = (pageNum - 1) * f.Limit
	}
	items, total, err := h.transformerUseCase.ListSummaries(r.Context(), middleware.GetTenantID(r.Context()), f)
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(page(items, total, pageNum, f.Limit), ""))
}

// GetAllWithCount returns every transformer and the count.
func (h *TransformerHandler) GetAllWithCount(w http.ResponseWriter, r *http.Request) {
	f, _ := filterFromQuery(r)
	items, total, err := h.transformerUseCase.ListSummaries(r.Context(), middleware.GetTenantID(r.Context()), f)
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(map[string]any{"transformers": items, "count": total}, ""))
}

// Count handles getting transformer count and the plan quota
func (h *TransformerHandler) Count(w http.ResponseWriter, r *http.Request) {
	n, err := h.transformerUseCase.CountTransformers(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "COUNT_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(map[string]any{"count": n}, ""))
}

// Update handles a partial transformer update
func (h *TransformerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req UpdateTransformerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	h.update(w, r, req.toInput(id), "transformer updated successfully")
}

func (h *TransformerHandler) update(w http.ResponseWriter, r *http.Request, in usecase.UpdateTransformerInput, msg string) {
	transformer, err := h.transformerUseCase.UpdateTransformer(r.Context(), in)
	if err != nil {
		writeUseCaseError(w, err, "UPDATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(transformer, msg))
}

// Delete handles transformer deletion (soft delete)
// NOTE: This performs a soft delete by updating deleted_at, it does NOT remove the record from the database
// Following SDD 04-DESIGN-DETALHADO/01-modelo-de-dados.md convention
func (h *TransformerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}

	if err := h.transformerUseCase.DeleteTransformer(r.Context(), id); err != nil {
		writeUseCaseError(w, err, "DELETE_ERROR")
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(nil, "transformer deleted successfully"))
}

// GetByCode handles getting a transformer by code
func (h *TransformerHandler) GetByCode(w http.ResponseWriter, r *http.Request) {
	transformer, err := h.transformerUseCase.GetTransformerByCode(r.Context(), middleware.GetTenantID(r.Context()), chi.URLParam(r, "code"))
	if err != nil {
		writeUseCaseError(w, err, "GET_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(transformer, ""))
}

// ExistsByCode handles checking if a transformer code exists
func (h *TransformerHandler) ExistsByCode(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	exists, err := h.transformerUseCase.TransformerExistsByCode(r.Context(), middleware.GetTenantID(r.Context()), code)
	if err != nil {
		writeUseCaseError(w, err, "CHECK_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(map[string]any{"code": code, "exists": exists}, ""))
}

// ListBySubstation handles listing transformers by substation
func (h *TransformerHandler) ListBySubstation(w http.ResponseWriter, r *http.Request) {
	substationID, ok := pathUUID(w, r, "substation_id")
	if !ok {
		return
	}
	items, err := h.transformerUseCase.ListBySubstation(r.Context(), middleware.GetTenantID(r.Context()), substationID)
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(items, ""))
}

// Stats handles getting transformer statistics (last 12 months)
func (h *TransformerHandler) Stats(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	stats, err := h.transformerUseCase.GetTransformerStats(r.Context(), id)
	if err != nil {
		writeUseCaseError(w, err, "STATS_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(stats, ""))
}

// ImportCSV imports the transformer registry from a CSV/XLSX file.
// Form fields: file, dry_run (true to only validate), mapping (JSON object
// {target_column: file_header}).
func (h *TransformerHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	in, ok := readRegistryUpload(w, r)
	if !ok {
		return
	}
	in.Kind = usecase.RegistryTransformers
	h.runRegistryImport(w, r, in)
}

func (h *TransformerHandler) runRegistryImport(w http.ResponseWriter, r *http.Request, in usecase.RegistryImportInput) {
	in.TenantID = middleware.GetTenantID(r.Context())
	in.UserID = middleware.GetUserID(r.Context())
	out, err := h.registryImport.Import(r.Context(), in)
	if err != nil {
		writeUseCaseError(w, err, "IMPORT_ERROR")
		return
	}
	status := http.StatusOK
	if !in.DryRun && out.Created > 0 {
		status = http.StatusCreated
	}
	request.WriteJSON(w, status, request.Success(out, out.Message))
}

// BatchCreate handles creating multiple transformers. All transformers are
// validated first; nothing is created when any of them is invalid.
func (h *TransformerHandler) BatchCreate(w http.ResponseWriter, r *http.Request) {
	var reqs []CreateTransformerRequest
	if !decodeJSON(w, r, &reqs) {
		return
	}
	if len(reqs) == 0 {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "nenhum transformador informado", nil))
		return
	}
	tenantID := middleware.GetTenantID(r.Context())

	rows := make([]usecase.CreateTransformerInput, 0, len(reqs))
	for _, req := range reqs {
		rows = append(rows, req.toInput(tenantID))
	}
	created, err := h.registryImport.CreateTransformers(r.Context(), tenantID, rows)
	if err != nil {
		writeUseCaseError(w, err, "CREATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusCreated, request.Success(created, "transformers created successfully"))
}

// BatchUpdate handles updating multiple transformers identified by "id".
func (h *TransformerHandler) BatchUpdate(w http.ResponseWriter, r *http.Request) {
	var reqs []UpdateTransformerRequest
	if !decodeJSON(w, r, &reqs) {
		return
	}
	if len(reqs) == 0 {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "nenhum transformador informado", nil))
		return
	}

	updated := make([]any, 0, len(reqs))
	for i, req := range reqs {
		if !isUUID(req.ID) {
			request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR",
				"id inválido no item "+strconv.Itoa(i+1), nil))
			return
		}
		transformer, err := h.transformerUseCase.UpdateTransformer(r.Context(), req.toInput(domain.UUID(req.ID)))
		if err != nil {
			writeUseCaseError(w, err, "UPDATE_ERROR")
			return
		}
		updated = append(updated, transformer)
	}

	request.WriteJSON(w, http.StatusOK, request.Success(updated, "transformers updated successfully"))
}

// BatchDelete handles soft-deleting multiple transformers
func (h *TransformerHandler) BatchDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.IDs) == 0 {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "nenhum id informado", nil))
		return
	}

	deleted := 0
	for _, idStr := range req.IDs {
		if !isUUID(idStr) {
			continue
		}
		if err := h.transformerUseCase.DeleteTransformer(r.Context(), domain.UUID(idStr)); err == nil {
			deleted++
		}
	}

	request.WriteJSON(w, http.StatusOK, request.Success(map[string]any{"deleted": deleted}, "transformers deleted successfully"))
}

// GetLossLimit handles getting transformer loss limit
func (h *TransformerHandler) GetLossLimit(w http.ResponseWriter, r *http.Request) {
	h.withTransformer(w, r, func(t *domain.Transformer) any {
		return map[string]any{
			"transformer_id":  t.ID,
			"loss_limit_pct":  t.LossLimitPctValue(),
			"is_default":      t.LossLimitPct == nil,
			"code":            t.Code,
			"power_kva":       t.PowerKVA,
			"core_loss_kw":    t.CoreLossKW,
			"winding_loss_kw": t.WindingLossKW,
		}
	})
}

// UpdateLossLimit handles updating transformer loss limit
func (h *TransformerHandler) UpdateLossLimit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		LossLimitPct *float64 `json:"loss_limit_pct"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.LossLimitPct == nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "loss_limit_pct é obrigatório", nil))
		return
	}
	h.update(w, r, usecase.UpdateTransformerInput{ID: id, LossLimitPct: req.LossLimitPct}, "loss limit updated successfully")
}

// GetTechnicalData handles getting transformer technical data
func (h *TransformerHandler) GetTechnicalData(w http.ResponseWriter, r *http.Request) {
	h.withTransformer(w, r, func(t *domain.Transformer) any {
		return map[string]any{
			"transformer_id":      t.ID,
			"code":                t.Code,
			"power_kva":           t.PowerKVA,
			"primary_voltage_kv":  t.PrimaryVoltageKV,
			"secondary_voltage_v": t.SecondaryVoltageV,
			"core_loss_kw":        t.CoreLossKW,
			"winding_loss_kw":     t.WindingLossKW,
			"nominal_current":     t.NominalCurrent(),
			"prodist_ready":       t.CoreLossKW != nil && t.WindingLossKW != nil,
		}
	})
}

// UpdateTechnicalData handles updating transformer technical data
func (h *TransformerHandler) UpdateTechnicalData(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		PowerKVA          *float64 `json:"power_kva"`
		PrimaryVoltageKV  *float64 `json:"primary_voltage_kv"`
		SecondaryVoltageV *float64 `json:"secondary_voltage_v"`
		CoreLossKW        *float64 `json:"core_loss_kw"`
		WindingLossKW     *float64 `json:"winding_loss_kw"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	h.update(w, r, usecase.UpdateTransformerInput{
		ID:                id,
		PowerKVA:          req.PowerKVA,
		PrimaryVoltageKV:  req.PrimaryVoltageKV,
		SecondaryVoltageV: req.SecondaryVoltageV,
		CoreLossKW:        req.CoreLossKW,
		WindingLossKW:     req.WindingLossKW,
	}, "technical data updated successfully")
}

// GetLocation handles getting transformer location
func (h *TransformerHandler) GetLocation(w http.ResponseWriter, r *http.Request) {
	h.withTransformer(w, r, func(t *domain.Transformer) any {
		return map[string]any{"transformer_id": t.ID, "code": t.Code, "lat": t.Lat, "lng": t.Lng}
	})
}

// UpdateLocation handles updating transformer location
func (h *TransformerHandler) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Lat *float64 `json:"lat"`
		Lng *float64 `json:"lng"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	in := usecase.UpdateTransformerInput{ID: id, Lat: req.Lat, Lng: req.Lng}
	if req.Lat == nil && req.Lng == nil {
		in.ClearLocation = true
	}
	h.update(w, r, in, "location updated successfully")
}

// GetSubstation handles getting transformer substation
func (h *TransformerHandler) GetSubstation(w http.ResponseWriter, r *http.Request) {
	h.withTransformer(w, r, func(t *domain.Transformer) any {
		return map[string]any{"transformer_id": t.ID, "code": t.Code, "substation_id": t.SubstationID}
	})
}

// UpdateSubstation handles updating transformer substation
func (h *TransformerHandler) UpdateSubstation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req UpdateTransformerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.SubstationID == nil {
		empty := ""
		req.SubstationID = &empty
	}
	h.update(w, r, UpdateTransformerRequest{SubstationID: req.SubstationID}.toInput(id), "substation updated successfully")
}

// GetActive handles getting transformer active status
func (h *TransformerHandler) GetActive(w http.ResponseWriter, r *http.Request) {
	h.withTransformer(w, r, func(t *domain.Transformer) any {
		return map[string]any{"transformer_id": t.ID, "code": t.Code, "active": t.Active}
	})
}

// UpdateActive handles activating or deactivating a transformer (RF01.3)
func (h *TransformerHandler) UpdateActive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Active *bool `json:"active"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Active == nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "active é obrigatório", nil))
		return
	}
	h.update(w, r, usecase.UpdateTransformerInput{ID: id, Active: req.Active}, "active status updated successfully")
}

func (h *TransformerHandler) withTransformer(w http.ResponseWriter, r *http.Request, view func(*domain.Transformer) any) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	t, err := h.transformerUseCase.GetTransformerByID(r.Context(), id)
	if err != nil {
		writeUseCaseError(w, err, "GET_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(view(t), ""))
}
