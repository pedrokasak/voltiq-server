package handler

import (
	"errors"
	"net/http"
	"strconv"
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
	filter := usecase.ListTenantsFilter{
		Status: r.URL.Query().Get("status"),
		Plan:   r.URL.Query().Get("plan"),
		Search: r.URL.Query().Get("search"),
		Page:   1,
		Limit:  20,
	}

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			filter.Page = page
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 100 {
			filter.Limit = limit
		}
	}

	result, err := h.superAdminUseCase.ListTenants(r.Context(), filter)
	if err != nil {
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("LIST_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(result, ""))
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

// ActivateTenant handles POST /api/v1/admin/tenants/{id}/activate
func (h *SuperAdminHandler) ActivateTenant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	type ActivateRequest struct {
		Plan string `json:"plan"` // optional: trial, starter, pro, enterprise
	}

	var req ActivateRequest
	if err := parseJSON(r, &req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	plan := req.Plan
	if plan == "" {
		plan = "starter"
	}

	// Validate plan
	validPlans := map[string]bool{
		"trial":      true,
		"starter":    true,
		"pro":        true,
		"enterprise": true,
	}
	if !validPlans[plan] {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "invalid plan", nil))
		return
	}

	tenant, err := h.superAdminUseCase.ActivateTenant(r.Context(), domain.UUID(id), domain.TenantPlan(req.Plan))
	if err != nil {
		if errors.Is(err, usecase.ErrTenantNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "tenant not found", nil))
			return
		}
		if errors.Is(err, usecase.ErrTenantAlreadyActive) {
			request.WriteJSON(w, http.StatusBadRequest,
				request.Fail("ALREADY_ACTIVE", "tenant is already active", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("ACTIVATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(tenant, "tenant activated successfully"))
}

// UpdateTenantPlan handles PATCH /api/v1/admin/tenants/{id}/plan
func (h *SuperAdminHandler) UpdateTenantPlan(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	type UpdatePlanRequest struct {
		Plan string `json:"plan"`
	}

	var req UpdatePlanRequest
	if err := parseJSON(r, &req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.Plan == "" {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "plan is required", nil))
		return
	}

	// Validate plan
	validPlans := map[string]bool{
		"trial":      true,
		"starter":    true,
		"pro":        true,
		"enterprise": true,
	}
	if !validPlans[req.Plan] {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "invalid plan", nil))
		return
	}

	tenant, err := h.superAdminUseCase.UpdateTenantPlan(r.Context(), domain.UUID(id), domain.TenantPlan(req.Plan))
	if err != nil {
		if errors.Is(err, usecase.ErrTenantNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "tenant not found", nil))
			return
		}
		if errors.Is(err, usecase.ErrSeatLimitExceeded) {
			request.WriteJSON(w, http.StatusBadRequest,
				request.Fail("SEAT_LIMIT_EXCEEDED", "plan downgrade would exceed current seat count", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("UPDATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(tenant, "plan updated successfully"))
}

// UpdateTenantStatus handles PATCH /api/v1/admin/tenants/{id}/status
func (h *SuperAdminHandler) UpdateTenantStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	type UpdateStatusRequest struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}

	var req UpdateStatusRequest
	if err := parseJSON(r, &req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.Status == "" {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "status is required", nil))
		return
	}

	// Validate status
	validStatuses := map[string]bool{
		"TRIAL":            true,
		"ACTIVE":           true,
		"SUSPENDED":        true,
		"PENDING_PAYMENT":  true,
		"CANCELLED":        true,
	}
	if !validStatuses[req.Status] {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "invalid status", nil))
		return
	}

	tenant, err := h.superAdminUseCase.UpdateTenantStatus(r.Context(), domain.UUID(id), req.Status, req.Reason)
	if err != nil {
		if errors.Is(err, usecase.ErrTenantNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "tenant not found", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("UPDATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(tenant, "status updated successfully"))
}

// Helper functions
func parseJSON(r *http.Request, v interface{}) error {
	// Implementation would use json.NewDecoder(r.Body).Decode(v)
	// This is a placeholder - actual implementation depends on request parsing
	return nil
}
