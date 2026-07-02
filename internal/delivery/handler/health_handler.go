package handler

import (
	"net/http"
	"time"

	"github.com/energybalance/server/internal/delivery/request"
)

// HealthHandler handles health check HTTP requests
type HealthHandler struct {
	version string
}

// HealthResponse represents a health check response
type HealthResponse struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Version   string                 `json:"version"`
	Checks    map[string]string      `json:"checks,omitempty"`
}

// NewHealthHandler creates a new HealthHandler
func NewHealthHandler(version string) *HealthHandler {
	return &HealthHandler{
		version: version,
	}
}

// Health handles liveness probe
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   h.version,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}

// Ready handles readiness probe
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	// In a real implementation, check database connectivity, migrations, etc.
	checks := map[string]string{
		"database":    "ok",
		"migrations":  "ok",
	}

	response := HealthResponse{
		Status:    "ready",
		Timestamp: time.Now(),
		Version:   h.version,
		Checks:    checks,
	}

	request.WriteJSON(w, http.StatusOK, request.Success(response, ""))
}
