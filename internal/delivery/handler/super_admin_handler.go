package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

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
	return &SuperAdminHandler{
		superAdminUseCase: superAdminUseCase,
	}
}

// ListTenants handles GET /api/v1/admin/tenants
func (h *SuperAdminHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.superAdminUseCase.ListTenants(r.Context())
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(tenants, ""))
}

// GetTenantByID handles GET /api/v1/admin/tenants/{id}
func (h *SuperAdminHandler) GetTenantByID(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	tenant, err := h.superAdminUseCase.GetTenantByID(r.Context(), domain.UUID(id))
	if err != nil {
		if errors.Is(err, usecase.ErrTenantNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "tenant not found", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("GET_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(tenant, ""))
}

// ListTenantUsers handles GET /api/v1/admin/tenants/{id}/users
func (h *SuperAdminHandler) ListTenantUsers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	users, err := h.superAdminUseCase.ListTenantUsers(r.Context(), domain.UUID(id))
	if err != nil {
		if errors.Is(err, usecase.ErrTenantNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "tenant not found", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(users, ""))
}
