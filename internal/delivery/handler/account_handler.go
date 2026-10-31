package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/usecase"
)

// AccountHandler serves the profile, tenant users, onboarding, settings and
// audit trail.
type AccountHandler struct {
	account *usecase.AccountUseCase
}

// NewAccountHandler creates an AccountHandler
func NewAccountHandler(account *usecase.AccountUseCase) *AccountHandler {
	return &AccountHandler{account: account}
}

// Me returns the authenticated user, tenant, permissions, plan limits,
// usage and onboarding progress (GET /auth/me).
func (h *AccountHandler) Me(w http.ResponseWriter, r *http.Request) {
	me, err := h.account.GetMe(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "ME_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(me, ""))
}

// ListUsers lists users and pending invites (GET /users)
func (h *AccountHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	page, err := h.account.ListUsers(r.Context(), middleware.GetTenantID(r.Context()), middleware.GetUserID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(page, ""))
}

// UpdateUser changes role, name or active flag (PATCH /users/{id})
func (h *AccountHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Role   *string `json:"role"`
		Name   *string `json:"name"`
		Active *bool   `json:"active"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	in := usecase.UpdateUserInput{Name: req.Name, Active: req.Active}
	if req.Role != nil {
		role := domain.UserRole(strings.ToUpper(*req.Role))
		in.Role = &role
	}
	user, err := h.account.UpdateUser(r.Context(), middleware.GetUserID(r.Context()), id, in)
	if err != nil {
		writeUseCaseError(w, err, "UPDATE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(user, "usuário atualizado"))
}

// RemoveUser soft-deletes a tenant user (DELETE /users/{id})
func (h *AccountHandler) RemoveUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.account.RemoveUser(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		writeUseCaseError(w, err, "DELETE_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(nil, "usuário removido"))
}

// Roles describes the roles and their permissions for the UI.
func (h *AccountHandler) Roles(w http.ResponseWriter, r *http.Request) {
	type roleInfo struct {
		Role        domain.UserRole `json:"role"`
		Label       string          `json:"label"`
		Description string          `json:"description"`
		Permissions []string        `json:"permissions"`
	}
	roles := []roleInfo{
		{domain.UserRoleOwner, "Proprietário", "Controle total do tenant, inclusive plano e outros proprietários", nil},
		{domain.UserRoleAdmin, "Administrador", "Gerencia usuários, plano, cadastros e auditoria", nil},
		{domain.UserRoleManager, "Gerente", "Operação completa e dados financeiros (R$)", nil},
		{domain.UserRoleEngineer, "Engenheiro", "Importa leituras, calcula balanços e configura alertas; sem R$", nil},
		{domain.UserRoleViewer, "Visualizador", "Somente leitura de dashboards e relatórios", nil},
	}
	for i := range roles {
		roles[i].Permissions = domain.PermissionsOf(roles[i].Role)
	}
	request.WriteJSON(w, http.StatusOK, request.Success(roles, ""))
}

// Onboarding returns the onboarding progress (GET /onboarding)
func (h *AccountHandler) Onboarding(w http.ResponseWriter, r *http.Request) {
	st, err := h.account.GetOnboarding(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "ONBOARDING_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(st, ""))
}

// CompleteOnboarding marks the wizard as finished (POST /onboarding/complete)
func (h *AccountHandler) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	st, err := h.account.CompleteOnboarding(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "ONBOARDING_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(st, "onboarding concluído"))
}

// GetSettings returns the tenant settings (GET /tenant/settings)
func (h *AccountHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.account.GetSettings(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "SETTINGS_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(s, ""))
}

// UpdateSettings changes the tenant settings (PUT /tenant/settings)
func (h *AccountHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name          *string  `json:"name"`
		TariffBRLMWh  *float64 `json:"tariff_brl_mwh"`
		ClearTariff   bool     `json:"clear_tariff"`
		Address       *string  `json:"address"`
		AddressNumber *string  `json:"address_number"`
		Province      *string  `json:"province"`
		PostalCode    *string  `json:"postal_code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	s, err := h.account.UpdateSettings(r.Context(), middleware.GetTenantID(r.Context()), usecase.SettingsInput{
		Name: req.Name, TariffBRLMWh: req.TariffBRLMWh, ClearTariff: req.ClearTariff,
		Address: req.Address, AddressNumber: req.AddressNumber, Province: req.Province, PostalCode: req.PostalCode,
	})
	if err != nil {
		writeUseCaseError(w, err, "SETTINGS_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(s, "configurações salvas"))
}

// AuditLog lists the audit trail (GET /audit-logs?table=&operation=&user=&since=&until=)
func (h *AccountHandler) AuditLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := repository.AuditFilter{
		Table:     q.Get("table"),
		Operation: strings.ToUpper(q.Get("operation")),
		UserEmail: q.Get("user"),
		Limit:     queryInt(r, "limit", 50),
		Offset:    queryInt(r, "offset", 0),
	}
	if t := queryDate(r, "since", time.Time{}); !t.IsZero() {
		f.Since = &t
	}
	if t := queryDate(r, "until", time.Time{}); !t.IsZero() {
		end := t.Add(24*time.Hour - time.Nanosecond)
		f.Until = &end
	}
	items, total, err := h.account.ListAudit(r.Context(), middleware.GetTenantID(r.Context()), f)
	if err != nil {
		writeUseCaseError(w, err, "AUDIT_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(page(items, total, f.Offset/max(f.Limit, 1)+1, f.Limit), ""))
}
