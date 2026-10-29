package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/voltiq/server/internal/aneel"
	"github.com/voltiq/server/internal/delivery/request"
)

// AneelSource is the open data client used by AneelHandler.
type AneelSource interface {
	Distributors(ctx context.Context, search string) ([]aneel.Distributor, error)
	CurrentTariff(ctx context.Context, agent string) (*aneel.TariffSet, error)
}

// AneelHandler exposes the ANEEL open data lookups (tarifas homologadas).
// Aplicar a tarifa ao tenant continua em PUT /tenant/settings.
type AneelHandler struct {
	source AneelSource
}

// NewAneelHandler creates an AneelHandler.
func NewAneelHandler(source AneelSource) *AneelHandler {
	return &AneelHandler{source: source}
}

func writeAneelError(w http.ResponseWriter, err error) {
	if errors.Is(err, aneel.ErrUnavailable) {
		slog.Warn("aneel open data unavailable", "error", err)
		request.WriteJSON(w, http.StatusBadGateway, request.Fail("ANEEL_UNAVAILABLE",
			"não foi possível consultar os dados abertos da ANEEL agora; tente novamente mais tarde", nil))
		return
	}
	writeUseCaseError(w, err, "ANEEL_ERROR")
}

// Distributors lists distribuidoras e permissionárias (GET /aneel/distributors?search=).
func (h *AneelHandler) Distributors(w http.ResponseWriter, r *http.Request) {
	list, err := h.source.Distributors(r.Context(), r.URL.Query().Get("search"))
	if err != nil {
		writeAneelError(w, err)
		return
	}
	list = list[:min(len(list), 50)]
	request.WriteJSON(w, http.StatusOK, request.Success(list, ""))
}

// Tariff returns the B1 conventional tariff in force (GET /aneel/tariffs?agent=).
func (h *AneelHandler) Tariff(w http.ResponseWriter, r *http.Request) {
	agent := strings.TrimSpace(r.URL.Query().Get("agent"))
	if agent == "" || len(agent) > 80 {
		request.WriteJSON(w, http.StatusUnprocessableEntity, request.Fail("VALIDATION_ERROR",
			"informe a sigla da distribuidora (agent)", nil))
		return
	}
	set, err := h.source.CurrentTariff(r.Context(), agent)
	if err != nil {
		writeAneelError(w, err)
		return
	}
	if set == nil {
		request.WriteJSON(w, http.StatusNotFound, request.Fail("NOT_FOUND",
			"nenhuma tarifa B1 vigente encontrada para esta distribuidora nos dados abertos da ANEEL", nil))
		return
	}
	request.WriteJSON(w, http.StatusOK, request.Success(set, ""))
}
