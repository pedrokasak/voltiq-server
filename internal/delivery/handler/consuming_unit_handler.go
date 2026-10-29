package handler

import (
	"net/http"
	"strings"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/ingestion"
	"github.com/voltiq/server/internal/usecase"
)

// ConsumingUnitHandler handles consuming unit HTTP requests
type ConsumingUnitHandler struct {
	ucUseCase      *usecase.ConsumingUnitUseCase
	registryImport *usecase.RegistryImportUseCase
}

// CreateConsumingUnitRequest represents a consuming unit creation request
type CreateConsumingUnitRequest struct {
	TransformerID string `json:"transformer_id"`
	UCCode        string `json:"uc_code"`
	Name          string `json:"name"`
	Class         string `json:"class"`
	Active        *bool  `json:"active"`
}

// UpdateConsumingUnitRequest is a partial update: omitted fields are kept.
type UpdateConsumingUnitRequest struct {
	TransformerID *string `json:"transformer_id"`
	UCCode        *string `json:"uc_code"`
	Name          *string `json:"name"`
	Class         *string `json:"class"`
	Active        *bool   `json:"active"`
}

// NewConsumingUnitHandler creates a new ConsumingUnitHandler
func NewConsumingUnitHandler(ucUseCase *usecase.ConsumingUnitUseCase, registryImport *usecase.RegistryImportUseCase) *ConsumingUnitHandler {
	return &ConsumingUnitHandler{ucUseCase: ucUseCase, registryImport: registryImport}
}

// parseClass accepts the domain enum or the Portuguese names of the SDD.
func parseClass(raw string) (*domain.UCClass, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, true
	}
	if c := domain.UCClass(strings.ToUpper(raw)); domain.ValidUCClass(c) {
		return &c, true
	}
	return ingestion.ParseUCClass(raw)
}

func invalidClass(w http.ResponseWriter) {
	request.WriteJSON(w, http.StatusUnprocessableEntity, request.Fail("VALIDATION_ERROR", "dados inválidos",
		[]domain.FieldError{{Field: "class", Message: "classe inválida"}}))
}

// Create handles consuming unit creation
func (h *ConsumingUnitHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateConsumingUnitRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	class, ok := parseClass(req.Class)
	if !ok {
		invalidClass(w)
		return
	}
	if req.TransformerID != "" && !isUUID(req.TransformerID) {
		request.WriteJSON(w, http.StatusUnprocessableEntity, request.Fail("VALIDATION_ERROR", "dados inválidos",
			[]domain.FieldError{{Field: "transformer_id", Message: "transformador não encontrado"}}))
		return
	}

	unit, err := h.ucUseCase.CreateConsumingUnit(r.Context(), usecase.CreateConsumingUnitInput{
		TenantID:      middleware.GetTenantID(r.Context()),
		TransformerID: domain.UUID(req.TransformerID),
		UCCode:        req.UCCode,
		Name:          req.Name,
		Class:         class,
		Active:        req.Active,
	})
	if err != nil {
		writeUseCaseError(w, err, "CREATE_ERROR")
		return
	}

	request.WriteJSON(w, http.StatusCreated, request.Success(unit, "consuming unit created successfully"))
}

// GetByID handles getting a consuming unit by ID
func (h *ConsumingUnitHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	unit, err := h.ucUseCase.GetConsumingUnitByID(r.Context(), id)
	if err != nil {
		writeUseCaseError(w, err, "GET_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(unit, ""))
}

// List handles listing all consuming units for a tenant
func (h *ConsumingUnitHandler) List(w http.ResponseWriter, r *http.Request) {
	units, err := h.ucUseCase.ListConsumingUnits(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(units, ""))
}

// ListByTransformer handles listing consuming units by transformer
func (h *ConsumingUnitHandler) ListByTransformer(w http.ResponseWriter, r *http.Request) {
	transformerID, ok := pathUUID(w, r, "transformer_id")
	if !ok {
		return
	}
	units, err := h.ucUseCase.ListConsumingUnitsByTransformer(r.Context(), transformerID)
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(units, ""))
}

// Update handles a partial consuming unit update
func (h *ConsumingUnitHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req UpdateConsumingUnitRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in := usecase.UpdateConsumingUnitInput{ID: id, UCCode: req.UCCode, Name: req.Name, Active: req.Active}
	if req.Class != nil {
		class, ok := parseClass(*req.Class)
		if !ok {
			invalidClass(w)
			return
		}
		in.Class = class
	}
	if req.TransformerID != nil {
		if !isUUID(*req.TransformerID) {
			request.WriteJSON(w, http.StatusUnprocessableEntity, request.Fail("VALIDATION_ERROR", "dados inválidos",
				[]domain.FieldError{{Field: "transformer_id", Message: "transformador não encontrado"}}))
			return
		}
		tid := domain.UUID(*req.TransformerID)
		in.TransformerID = &tid
	}

	unit, err := h.ucUseCase.UpdateConsumingUnit(r.Context(), in)
	if err != nil {
		writeUseCaseError(w, err, "UPDATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(unit, "consuming unit updated successfully"))
}

// Delete handles consuming unit deletion (soft delete)
// NOTE: This performs a soft delete by updating deleted_at, it does NOT remove the record from the database
// Following SDD 04-DESIGN-DETALHADO/01-modelo-de-dados.md convention
func (h *ConsumingUnitHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.ucUseCase.DeleteConsumingUnit(r.Context(), id); err != nil {
		writeUseCaseError(w, err, "DELETE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(nil, "consuming unit deleted successfully"))
}

// Consumption returns the monthly consumption history of a UC (?months=12).
func (h *ConsumingUnitHandler) Consumption(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	hist, err := h.ucUseCase.GetConsumptionHistory(r.Context(), id, queryInt(r, "months", 12))
	if err != nil {
		writeUseCaseError(w, err, "CONSUMPTION_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(hist, ""))
}

// Import imports the consuming unit registry from a CSV/XLSX file.
func (h *ConsumingUnitHandler) Import(w http.ResponseWriter, r *http.Request) {
	in, ok := readRegistryUpload(w, r)
	if !ok {
		return
	}
	in.Kind = usecase.RegistryConsumingUnits
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
