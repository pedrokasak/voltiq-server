package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/usecase"
)

// InviteHandler handles invitation HTTP requests
type InviteHandler struct {
	inviteUseCase *usecase.InviteUseCase
	authUseCase   *usecase.AuthUseCase
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

// NewInviteHandler creates a new InviteHandler. authUseCase is optional; with
// it, accepting an invite starts a session.
func NewInviteHandler(inviteUseCase *usecase.InviteUseCase, authUseCase ...*usecase.AuthUseCase) *InviteHandler {
	h := &InviteHandler{inviteUseCase: inviteUseCase}
	if len(authUseCase) > 0 {
		h.authUseCase = authUseCase[0]
	}
	return h
}

func writeInviteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrInviteNotFound):
		request.WriteJSON(w, http.StatusNotFound, request.Fail("INVITE_NOT_FOUND", "convite não encontrado", nil))
	case errors.Is(err, usecase.ErrInviteExpired):
		request.WriteJSON(w, http.StatusGone, request.Fail("INVITE_EXPIRED", "convite expirado; peça um novo ao administrador", nil))
	case errors.Is(err, usecase.ErrInviteAlreadyUsed):
		request.WriteJSON(w, http.StatusConflict, request.Fail("INVITE_USED", "convite já utilizado", nil))
	case errors.Is(err, usecase.ErrInviteCancelled):
		request.WriteJSON(w, http.StatusGone, request.Fail("INVITE_CANCELLED", "convite cancelado", nil))
	case errors.Is(err, usecase.ErrEmailAlreadyInUse):
		request.WriteJSON(w, http.StatusConflict, request.Fail("EMAIL_IN_USE", "este e-mail já possui conta", nil))
	default:
		writeUseCaseError(w, err, "INVITE_ERROR")
	}
}

// CreateInvite handles creating a new user invitation
func (h *InviteHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	var req CreateInviteRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	output, err := h.inviteUseCase.CreateInvite(r.Context(), usecase.CreateInviteInput{
		TenantID:  middleware.GetTenantID(r.Context()),
		Email:     req.Email,
		Role:      domain.UserRole(req.Role),
		InvitedBy: middleware.GetUserID(r.Context()),
		ActorRole: domain.UserRole(middleware.GetRole(r.Context())),
	})
	if err != nil {
		writeInviteError(w, err)
		return
	}

	request.WriteJSON(w, http.StatusCreated, request.Success(inviteResponse(output), "invite created successfully"))
}

func inviteResponse(o *usecase.CreateInviteOutput) map[string]any {
	return map[string]any{
		"invite_id":  o.Invite.ID,
		"email":      o.Invite.Email,
		"role":       o.Invite.Role,
		"token":      o.Token,
		"invite_url": o.InviteURL,
		"email_sent": o.EmailSent,
		"expires_at": o.ExpiresAt,
		"status":     o.Invite.Status,
	}
}

// ListInvites lists the pending invites of the tenant
func (h *InviteHandler) ListInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := h.inviteUseCase.ListPending(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "LIST_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(invites, ""))
}

// ResendInvite renews and resends a pending invite
func (h *InviteHandler) ResendInvite(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "token")
	if !ok {
		return
	}
	output, err := h.inviteUseCase.ResendInvite(r.Context(), id)
	if err != nil {
		writeInviteError(w, err)
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(inviteResponse(output), "invite resent"))
}

// ValidateInvite handles validating an invite token
func (h *InviteHandler) ValidateInvite(w http.ResponseWriter, r *http.Request) {
	invite, err := h.inviteUseCase.ValidateInvite(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeInviteError(w, err)
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(map[string]any{
		"invite_id":   invite.ID,
		"email":       invite.Email,
		"role":        invite.Role,
		"tenant_id":   invite.TenantID,
		"tenant_name": h.inviteUseCase.TenantName(r.Context(), invite.TenantID),
		"expires_at":  invite.ExpiresAt,
		"status":      invite.Status,
	}, "invite is valid"))
}

// AcceptInvite creates the user of an invite and starts the session
func (h *InviteHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req AcceptInviteRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	user, err := h.inviteUseCase.AcceptInvite(r.Context(), usecase.AcceptInviteInput{
		Token:    chi.URLParam(r, "token"),
		Password: req.Password,
		Name:     req.Name,
	})
	if err != nil {
		writeInviteError(w, err)
		return
	}

	response := map[string]any{"user": user}
	if h.authUseCase != nil {
		if session, err := h.authUseCase.IssueFor(user); err == nil {
			setRefreshCookie(w, session.RefreshToken, session.RefreshExpiry)
			for k, v := range sessionResponse(session) {
				response[k] = v
			}
		}
	}
	request.WriteJSON(w, http.StatusOK, request.Success(response, "invite accepted successfully"))
}

// CancelInvite handles cancelling an invitation
func (h *InviteHandler) CancelInvite(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "token")
	if !ok {
		return
	}
	if err := h.inviteUseCase.CancelInvite(r.Context(), id); err != nil {
		writeInviteError(w, err)
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(nil, "invite cancelled successfully"))
}
