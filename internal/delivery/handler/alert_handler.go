package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/usecase"
)

// AlertHandler handles alert recipients, transformer groups and the alert
// history (RF05).
type AlertHandler struct {
	alertUseCase *usecase.AlertUseCase
}

// AlertConfigRequest creates or updates an alert recipient. "type" is the
// legacy name of min_level.
type AlertConfigRequest struct {
	TransformerID *string `json:"transformer_id"`
	GroupID       *string `json:"group_id"`
	Type          string  `json:"type"`
	MinLevel      string  `json:"min_level"`
	Channel       string  `json:"channel"`
	Recipient     string  `json:"recipient"`
	Active        *bool   `json:"active"`
}

// NewAlertHandler creates a new AlertHandler
func NewAlertHandler(alertUseCase *usecase.AlertUseCase) *AlertHandler {
	return &AlertHandler{alertUseCase: alertUseCase}
}

func optUUID(s *string) (*domain.UUID, bool) {
	if s == nil || *s == "" {
		return nil, true
	}
	if !isUUID(*s) {
		return nil, false
	}
	id := domain.UUID(*s)
	return &id, true
}

func (req AlertConfigRequest) toInput(w http.ResponseWriter, tenantID domain.UUID) (usecase.RecipientInput, bool) {
	tid, ok1 := optUUID(req.TransformerID)
	gid, ok2 := optUUID(req.GroupID)
	if !ok1 || !ok2 {
		request.WriteJSON(w, http.StatusUnprocessableEntity, request.Fail("VALIDATION_ERROR", "transformador ou grupo inválido", nil))
		return usecase.RecipientInput{}, false
	}
	level := req.MinLevel
	if level == "" {
		level = req.Type
	}
	if strings.EqualFold(level, "BOTH") {
		level = string(domain.AlertTypeWarning)
	}
	return usecase.RecipientInput{
		TenantID: tenantID, TransformerID: tid, GroupID: gid,
		Channel:   domain.AlertChannel(strings.ToUpper(req.Channel)),
		Recipient: req.Recipient, MinLevel: domain.AlertType(strings.ToUpper(level)), Active: req.Active,
	}, true
}

// CreateRecipient adds an alert recipient (POST /alert-recipients)
func (h *AlertHandler) CreateRecipient(w http.ResponseWriter, r *http.Request) {
	var req AlertConfigRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in, ok := req.toInput(w, middleware.GetTenantID(r.Context()))
	if !ok {
		return
	}
	rec, err := h.alertUseCase.CreateRecipient(r.Context(), in)
	if err != nil {
		writeUseCaseError(w, err, "CREATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusCreated, request.Success(rec, "destinatário cadastrado"))
}

// CreateForTransformer adds a recipient for one transformer
// (POST /transformers/{id}/alert-config).
func (h *AlertHandler) CreateForTransformer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req AlertConfigRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	s := string(id)
	req.TransformerID, req.GroupID = &s, nil
	in, ok := req.toInput(w, middleware.GetTenantID(r.Context()))
	if !ok {
		return
	}
	rec, err := h.alertUseCase.CreateRecipient(r.Context(), in)
	if err != nil {
		writeUseCaseError(w, err, "CREATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusCreated, request.Success(rec, "destinatário cadastrado"))
}

// ListRecipients lists recipients (GET /alert-recipients?transformer_id=)
func (h *AlertHandler) ListRecipients(w http.ResponseWriter, r *http.Request) {
	var tid *domain.UUID
	if v := r.URL.Query().Get("transformer_id"); isUUID(v) {
		id := domain.UUID(v)
		tid = &id
	}
	items, err := h.alertUseCase.ListRecipients(r.Context(), middleware.GetTenantID(r.Context()), tid)
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(items, ""))
}

// RecipientsByTransformer lists the recipients of one transformer
// (GET /transformers/{id}/alert-config and legacy /alerts/transformer/{id}).
func (h *AlertHandler) RecipientsByTransformer(w http.ResponseWriter, r *http.Request) {
	param := "id"
	if r.URL.Path != "" && strings.Contains(r.URL.Path, "/alerts/transformer/") {
		param = "transformer_id"
	}
	id, ok := pathUUID(w, r, param)
	if !ok {
		return
	}
	items, err := h.alertUseCase.ListRecipients(r.Context(), middleware.GetTenantID(r.Context()), &id)
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(items, ""))
}

// UpdateRecipient changes a recipient (PUT /alert-recipients/{id})
func (h *AlertHandler) UpdateRecipient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req AlertConfigRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	in, ok := req.toInput(w, middleware.GetTenantID(r.Context()))
	if !ok {
		return
	}
	rec, err := h.alertUseCase.UpdateRecipient(r.Context(), id, in)
	if err != nil {
		writeUseCaseError(w, err, "UPDATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(rec, "destinatário atualizado"))
}

// DeleteRecipient removes a recipient (DELETE /alert-recipients/{id})
func (h *AlertHandler) DeleteRecipient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.alertUseCase.DeleteRecipient(r.Context(), id); err != nil {
		writeUseCaseError(w, err, "DELETE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(nil, "destinatário removido"))
}

// TestRecipient sends a test message (POST /alert-recipients/{id}/test)
func (h *AlertHandler) TestRecipient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.alertUseCase.SendTest(r.Context(), id); err != nil {
		writeUseCaseError(w, err, "TEST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(nil, "mensagem de teste enviada"))
}

// ListGroups lists transformer groups (GET /transformer-groups)
func (h *AlertHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	items, err := h.alertUseCase.ListGroups(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(items, ""))
}

type groupRequest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	TransformerIDs []string `json:"transformer_ids"`
}

func (req groupRequest) ids() []domain.UUID {
	out := make([]domain.UUID, 0, len(req.TransformerIDs))
	for _, id := range req.TransformerIDs {
		if isUUID(id) {
			out = append(out, domain.UUID(id))
		}
	}
	return out
}

// CreateGroup creates a transformer group (POST /transformer-groups)
func (h *AlertHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var req groupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	g, err := h.alertUseCase.SaveGroup(r.Context(), "", usecase.GroupInput{
		TenantID: middleware.GetTenantID(r.Context()), Name: req.Name, Description: req.Description, TransformerIDs: req.ids(),
	})
	if err != nil {
		writeUseCaseError(w, err, "CREATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusCreated, request.Success(g, "grupo criado"))
}

// UpdateGroup changes a group and its members (PUT /transformer-groups/{id})
func (h *AlertHandler) UpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req groupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	g, err := h.alertUseCase.SaveGroup(r.Context(), id, usecase.GroupInput{
		TenantID: middleware.GetTenantID(r.Context()), Name: req.Name, Description: req.Description, TransformerIDs: req.ids(),
	})
	if err != nil {
		writeUseCaseError(w, err, "UPDATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(g, "grupo atualizado"))
}

// DeleteGroup removes a group (DELETE /transformer-groups/{id})
func (h *AlertHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.alertUseCase.DeleteGroup(r.Context(), middleware.GetTenantID(r.Context()), id); err != nil {
		writeUseCaseError(w, err, "DELETE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(nil, "grupo removido"))
}

// ListHistory lists sent alerts (GET /alerts?transformer_id=&status=&channel=&type=&days=&limit=&offset=)
func (h *AlertHandler) ListHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := repository.DeliveryFilter{
		Status:  strings.ToUpper(q.Get("status")),
		Channel: strings.ToUpper(q.Get("channel")),
		Type:    strings.ToUpper(q.Get("type")),
		Limit:   queryInt(r, "limit", 50),
		Offset:  queryInt(r, "offset", 0),
	}
	if v := q.Get("transformer_id"); isUUID(v) {
		f.TransformerID = v
	}
	if days := queryInt(r, "days", 0); days > 0 {
		since := time.Now().AddDate(0, 0, -days)
		f.Since = &since
	}
	items, total, err := h.alertUseCase.ListDeliveries(r.Context(), middleware.GetTenantID(r.Context()), f)
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	request.WriteJSON(w, http.StatusOK, request.Success(items, ""))
}

// GetByID returns one alert of the history (GET /alerts/{id})
func (h *AlertHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	d, err := h.alertUseCase.GetDelivery(r.Context(), id)
	if err != nil {
		writeUseCaseError(w, err, "GET_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(d, ""))
}

// Stats counts alerts of the last days (GET /alerts/stats?days=30)
func (h *AlertHandler) Stats(w http.ResponseWriter, r *http.Request) {
	s, err := h.alertUseCase.Stats(r.Context(), middleware.GetTenantID(r.Context()), queryInt(r, "days", 30))
	if err != nil {
		writeUseCaseError(w, err, "STATS_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(s, ""))
}
