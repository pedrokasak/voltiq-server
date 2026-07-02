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

// InviteHandler handles invitation HTTP requests
type InviteHandler struct {
	inviteUseCase *usecase.InviteUseCase
}

// CreateInviteRequest represents an invite creation request
type CreateInviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// AcceptInviteRequest represents an invite acceptance request
type AcceptInviteRequest struct {
	Password string `json:"password"`
	Name     string `json:"name"`
}

// NewInviteHandler creates a new InviteHandler
func NewInviteHandler(inviteUseCase *usecase.InviteUseCase) *InviteHandler {
	return &InviteHandler{
		inviteUseCase: inviteUseCase,
	}
}

// CreateInvite handles creating a new user invitation
func (h *InviteHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())

	var req CreateInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.Email == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "email is required", nil))
		return
	}

	role := domain.UserRoleViewer
	if req.Role != "" {
		role = domain.UserRole(req.Role)
	}

	output, err := h.inviteUseCase.CreateInvite(r.Context(), usecase.CreateInviteInput{
		TenantID:  tenantID,
		Email:     req.Email,
		Role:      role,
		InvitedBy: userID,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("INVITE_ERROR", err.Error(), nil))
		return
	}

	response := map[string]any{
		"invite_id":  output.Invite.ID,
		"email":      output.Invite.Email,
		"role":       output.Invite.Role,
		"token":      output.Token,
		"expires_at": output.ExpiresAt,
		"status":     output.Invite.Status,
	}

	request.WriteJSON(w, http.StatusCreated, request.Success(response, "invite created successfully"))
}

// ValidateInvite handles validating an invite token
func (h *InviteHandler) ValidateInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "token is required", nil))
		return
	}

	invite, err := h.inviteUseCase.ValidateInvite(r.Context(), token)
	if err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("INVITE_INVALID", err.Error(), nil))
		return
	}

	response := map[string]any{
		"invite_id":  invite.ID,
		"email":      invite.Email,
		"role":       invite.Role,
		"tenant_id":  invite.TenantID,
		"expires_at": invite.ExpiresAt,
		"status":     invite.Status,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, "invite is valid"))
}

// AcceptInvite handles accepting an invitation
func (h *InviteHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "token is required", nil))
		return
	}

	var req AcceptInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.Password == "" || req.Name == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "password and name are required", nil))
		return
	}

	user, err := h.inviteUseCase.AcceptInvite(r.Context(), usecase.AcceptInviteInput{
		Token:    token,
		Password: req.Password,
		Name:     req.Name,
	})
	if err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("INVITE_ACCEPT_ERROR", err.Error(), nil))
		return
	}

	response := map[string]any{
		"user": user,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, "invite accepted successfully"))
}

// CancelInvite handles cancelling an invitation
func (h *InviteHandler) CancelInvite(w http.ResponseWriter, r *http.Request) {
	inviteID := chi.URLParam(r, "id")
	if inviteID == "" {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "id is required", nil))
		return
	}

	if err := h.inviteUseCase.CancelInvite(r.Context(), domain.UUID(inviteID)); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("INVITE_CANCEL_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(nil, "invite cancelled successfully"))
}
