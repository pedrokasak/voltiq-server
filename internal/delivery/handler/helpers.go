package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/usecase"
)

// maxJSONBody limits JSON request bodies (uploads use multipart limits).
const maxJSONBody = 1 << 20

// decodeJSON decodes the request body into dst, writing a 400 on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "corpo da requisição inválido", nil))
		return false
	}
	return true
}

// pathUUID reads a UUID URL parameter, writing a 404 when it is malformed so
// malformed IDs behave like unknown ones.
func pathUUID(w http.ResponseWriter, r *http.Request, name string) (domain.UUID, bool) {
	raw := chi.URLParam(r, name)
	if _, err := uuid.Parse(raw); err != nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "recurso não encontrado", nil))
		return "", false
	}
	return domain.UUID(raw), true
}

// isUUID reports whether s is a valid UUID.
func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// queryInt reads a positive integer query parameter with a default.
func queryInt(r *http.Request, name string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil && v >= 0 {
		return v
	}
	return def
}

// queryDate parses a YYYY-MM-DD query parameter.
func queryDate(r *http.Request, name string, def time.Time) time.Time {
	return parseDate(r.URL.Query().Get(name), def)
}

func parseDate(v string, def time.Time) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t
	}
	// Datas sem fuso sao do horario de Brasilia (periodos de balanco).
	for _, layout := range []string{"2006-01-02", "2006-01"} {
		if t, err := time.ParseInLocation(layout, v, domain.BusinessLocation); err == nil {
			return t
		}
	}
	return def
}

// writeUseCaseError maps domain and use case errors to HTTP responses so
// handlers do not leak internal messages (CLAUDE.md: erro generico ao usuario).
func writeUseCaseError(w http.ResponseWriter, err error, fallbackCode string) {
	var vErr *domain.ValidationError
	var limitErr *usecase.PlanLimitError
	switch {
	case errors.As(err, &vErr):
		request.WriteJSON(w, http.StatusUnprocessableEntity, request.Fail("VALIDATION_ERROR", "dados inválidos", vErr.Fields))
	case errors.As(err, &limitErr):
		request.WriteJSON(w, http.StatusPaymentRequired, request.Fail("PLAN_LIMIT_REACHED",
			"limite do plano atingido; faça upgrade para continuar", limitErr))
	case errors.Is(err, usecase.ErrTransformerNotFound),
		errors.Is(err, usecase.ErrConsumingUnitNotFound),
		errors.Is(err, usecase.ErrImportNotFound),
		errors.Is(err, usecase.ErrAlertNotFound),
		errors.Is(err, usecase.ErrBalanceNotFound),
		errors.Is(err, usecase.ErrNotFound),
		errors.Is(err, usecase.ErrUserNotFound),
		errors.Is(err, usecase.ErrTenantNotFound),
		errors.Is(err, usecase.ErrInviteNotFound),
		errors.Is(err, repository.ErrNotFound):
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND", "recurso não encontrado", nil))
	case errors.Is(err, usecase.ErrSeatLimitExceeded):
		request.WriteJSON(w, http.StatusPaymentRequired, request.Fail("PLAN_LIMIT_REACHED", "limite de usuários do plano atingido", nil))
	case errors.Is(err, usecase.ErrEmailAlreadyInUse):
		request.WriteJSON(w, http.StatusConflict, request.Fail("EMAIL_IN_USE", "este e-mail já está em uso", nil))
	case errors.Is(err, usecase.ErrInvalidPlan):
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("INVALID_PLAN", "plano inválido", nil))
	case errors.Is(err, usecase.ErrTransformerCodeExists),
		errors.Is(err, usecase.ErrUCCodeExists),
		errors.Is(err, usecase.ErrConflict):
		request.WriteJSON(w, http.StatusConflict, request.Fail("CONFLICT", conflictMessage(err), nil))
	case errors.Is(err, usecase.ErrInsufficientData):
		request.WriteJSON(w, http.StatusUnprocessableEntity, request.Fail("INSUFFICIENT_DATA",
			"faltam leituras no período: é preciso a energia do transformador e o consumo das UCs", nil))
	case errors.Is(err, usecase.ErrInvalidPeriod):
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", "período inválido: a data final deve ser posterior à inicial", nil))
	case errors.Is(err, usecase.ErrForbidden):
		request.WriteJSON(w, http.StatusForbidden, request.Fail("FORBIDDEN", detail(err, usecase.ErrForbidden, "permissão insuficiente"), nil))
	case errors.Is(err, usecase.ErrBadRequest):
		request.WriteJSON(w, http.StatusBadRequest, request.Fail("VALIDATION_ERROR", detail(err, usecase.ErrBadRequest, "requisição inválida"), nil))
	default:
		slog.Error("request failed", "code", fallbackCode, "error", err)
		request.WriteJSON(w, http.StatusInternalServerError, request.Fail(fallbackCode, "erro interno ao processar a requisição", nil))
	}
}

// detail extracts the user-facing message wrapped as "<sentinel>: <msg>".
func detail(err, sentinel error, def string) string {
	msg := strings.TrimPrefix(err.Error(), sentinel.Error()+": ")
	if msg == err.Error() || msg == "" {
		return def
	}
	return msg
}

func conflictMessage(err error) string {
	switch {
	case errors.Is(err, usecase.ErrTransformerCodeExists):
		return "já existe um transformador com este código"
	case errors.Is(err, usecase.ErrUCCodeExists):
		return "já existe uma unidade consumidora com este código"
	}
	if msg := detail(err, usecase.ErrConflict, ""); msg != "" {
		return msg
	}
	return "conflito com um registro existente"
}

// page builds the standard paginated payload.
func page(data any, total, pageNum, limit int) map[string]any {
	totalPages := 0
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}
	return map[string]any{
		"data":        data,
		"total":       total,
		"page":        pageNum,
		"limit":       limit,
		"total_pages": totalPages,
	}
}
