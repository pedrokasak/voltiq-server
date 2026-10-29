package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/usecase"
)

// TrainingHandler exposes the in-app training progress.
type TrainingHandler struct {
	training *usecase.TrainingUseCase
}

// NewTrainingHandler creates a TrainingHandler.
func NewTrainingHandler(training *usecase.TrainingUseCase) *TrainingHandler {
	return &TrainingHandler{training: training}
}

// Progress returns the current user's progress (GET /training/progress).
func (h *TrainingHandler) Progress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := h.training.Status(ctx, middleware.GetTenantID(ctx), middleware.GetUserID(ctx))
	if err != nil {
		writeUseCaseError(w, err, "TRAINING_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(s, ""))
}

// Complete marks a lesson as completed (PUT /training/progress/{lesson} {quiz_score}).
func (h *TrainingHandler) Complete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QuizScore *int `json:"quiz_score"`
	}
	if r.ContentLength > 0 && !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	s, err := h.training.Complete(ctx, middleware.GetTenantID(ctx), middleware.GetUserID(ctx),
		chi.URLParam(r, "lesson"), req.QuizScore)
	if err != nil {
		writeUseCaseError(w, err, "TRAINING_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(s, ""))
}

// Reset marks a lesson as not completed (DELETE /training/progress/{lesson}).
func (h *TrainingHandler) Reset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := h.training.Reset(ctx, middleware.GetTenantID(ctx), middleware.GetUserID(ctx), chi.URLParam(r, "lesson"))
	if err != nil {
		writeUseCaseError(w, err, "TRAINING_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(s, ""))
}

// Team returns the team's progress (GET /training/team, users:manage).
func (h *TrainingHandler) Team(w http.ResponseWriter, r *http.Request) {
	t, err := h.training.Team(r.Context(), middleware.GetTenantID(r.Context()))
	if err != nil {
		writeUseCaseError(w, err, "TRAINING_ERROR")
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(t, ""))
}
