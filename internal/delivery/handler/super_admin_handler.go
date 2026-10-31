package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/usecase"
)

// SuperAdminHandler handles cross-tenant administrative HTTP requests.
// All routes served by this handler must be guarded by the SUPER_ADMIN role
// middleware at the router level.
type SuperAdminHandler struct {
	superAdminUseCase *usecase.SuperAdminUseCase
}

// NewSuperAdminHandler creates a new SuperAdminHandler
func NewSuperAdminHandler(superAdminUseCase *usecase.SuperAdminUseCase) *SuperAdminHandler {
	return &SuperAdminHandler{superAdminUseCase: superAdminUseCase}
}

func actor(r *http.Request) string {
	return middleware.GetUserEmail(r.Context())
}

func writeAdminError(w http.ResponseWriter, err error, code string) {
	switch {
	case errors.Is(err, usecase.ErrTenantAlreadyActive):
		request.WriteJSON(w, http.StatusConflict, request.Fail("ALREADY_ACTIVE", "tenant já está ativo", nil))
	case errors.Is(err, usecase.ErrSeatLimitExceeded):
		request.WriteJSON(w, http.StatusConflict, request.Fail("SEAT_LIMIT_EXCEEDED",
			"o tenant tem mais usuários do que o plano permite", nil))
	default:
		writeUseCaseError(w, err, code)
	}
}

// ListTenants handles GET /api/v1/admin/tenants?status=&plan=&search=&page=&limit=
// Retorna tenants com contadores de uso para a tela de administracao.
func (h *SuperAdminHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := usecase.ListTenantsFilter{
		Status: strings.ToUpper(q.Get("status")),
		Plan:   strings.ToLower(q.Get("plan")),
		Search: strings.TrimSpace(q.Get("search")),
		Page:   queryInt(r, "page", 1),
		Limit:  queryInt(r, "limit", 20),
	}
	result, err := h.superAdminUseCase.ListOverview(r.Context(), filter)
	if err != nil {
		writeAdminError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(result, ""))
}

// GetTenantByID handles GET /api/v1/admin/tenants/{id}
// Retorna tenant, usuarios, uso, pagamentos, cobranca e acoes administrativas.
func (h *SuperAdminHandler) GetTenantByID(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	detail, err := h.superAdminUseCase.GetTenantDetail(r.Context(), id)
	if err != nil {
		writeAdminError(w, err, "GET_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(detail, ""))
}

// ListTenantUsers handles GET /api/v1/admin/tenants/{id}/users
func (h *SuperAdminHandler) ListTenantUsers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	users, err := h.superAdminUseCase.ListTenantUsers(r.Context(), id)
	if err != nil {
		writeAdminError(w, err, "LIST_ERROR")
		return
	}
	if users == nil {
		users = []*domain.User{}
	}
	request.WriteJSON(w, http.StatusOK, request.Success(users, ""))
}

// ActivateTenant handles POST /api/v1/admin/tenants/{id}/activate
func (h *SuperAdminHandler) ActivateTenant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Plan string `json:"plan"`
	}
	if r.ContentLength != 0 && !decodeJSON(w, r, &req) {
		return
	}
	tenant, err := h.superAdminUseCase.ActivateTenant(r.Context(), id, domain.TenantPlan(strings.ToLower(req.Plan)), actor(r))
	if err != nil {
		writeAdminError(w, err, "ACTIVATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(tenant, "tenant ativado"))
}

// UpdateTenantPlan handles PATCH /api/v1/admin/tenants/{id}/plan
func (h *SuperAdminHandler) UpdateTenantPlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Plan string `json:"plan"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	tenant, err := h.superAdminUseCase.UpdateTenantPlan(r.Context(), id, domain.TenantPlan(strings.ToLower(req.Plan)), actor(r))
	if err != nil {
		writeAdminError(w, err, "UPDATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(tenant, "plano atualizado"))
}

// UpdateTenantStatus handles PATCH /api/v1/admin/tenants/{id}/status
func (h *SuperAdminHandler) UpdateTenantStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		request.WriteJSON(w, http.StatusUnprocessableEntity, request.Fail("VALIDATION_ERROR", "informe o motivo da alteração", nil))
		return
	}
	tenant, err := h.superAdminUseCase.UpdateTenantStatus(r.Context(), id, strings.ToUpper(req.Status), req.Reason, actor(r))
	if err != nil {
		writeAdminError(w, err, "UPDATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(tenant, "status atualizado"))
}

// ExtendTrial handles POST /api/v1/admin/tenants/{id}/extend-trial {days, reason}
func (h *SuperAdminHandler) ExtendTrial(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Days   int    `json:"days"`
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	tenant, err := h.superAdminUseCase.ExtendTrial(r.Context(), id, req.Days, req.Reason, actor(r))
	if err != nil {
		writeAdminError(w, err, "TRIAL_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(tenant, "trial estendido"))
}

// BillingMetrics handles GET /api/v1/admin/billing/metrics?period=30d|90d|12m
func (h *SuperAdminHandler) BillingMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := h.superAdminUseCase.SaaSMetrics(r.Context(), usecase.ParsePeriodDays(r.URL.Query().Get("period")))
	if err != nil {
		writeAdminError(w, err, "METRICS_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(m, ""))
}

// AuditLog handles GET /api/v1/admin/audit?limit=
func (h *SuperAdminHandler) AuditLog(w http.ResponseWriter, r *http.Request) {
	items, err := h.superAdminUseCase.PlatformAudit(r.Context(), queryInt(r, "limit", 50))
	if err != nil {
		writeAdminError(w, err, "AUDIT_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(items, ""))
}
