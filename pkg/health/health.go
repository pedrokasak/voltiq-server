package health

import (
	"context"
	"database/sql"
	"time"
)

// HealthChecker provides health check functionality
type HealthChecker struct {
	db      *sql.DB
	version string
}

// HealthStatus represents the health status of a component
type HealthStatus struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// HealthReport represents the overall health report
type HealthReport struct {
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Version   string                 `json:"version"`
	Checks    map[string]HealthStatus `json:"checks"`
}

// NewHealthChecker creates a new HealthChecker
func NewHealthChecker(db *sql.DB, version string) *HealthChecker {
	return &HealthChecker{
		db:      db,
		version: version,
	}
}

// Check performs a health check
func (h *HealthChecker) Check(ctx context.Context) HealthReport {
	report := HealthReport{
		Status:    "healthy",
		Timestamp: time.Now(),
		Version:   h.version,
		Checks:    make(map[string]HealthStatus),
	}

	// Check database
	dbStatus := h.checkDatabase(ctx)
	report.Checks["database"] = dbStatus
	if dbStatus.Status != "healthy" {
		report.Status = "unhealthy"
	}

	return report
}

// checkDatabase checks database connectivity
func (h *HealthChecker) checkDatabase(ctx context.Context) HealthStatus {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		return HealthStatus{
			Status:  "unhealthy",
			Message: err.Error(),
		}
	}

	return HealthStatus{
		Status: "healthy",
	}
}

// IsHealthy returns true if all checks pass
func (h *HealthChecker) IsHealthy(ctx context.Context) bool {
	report := h.Check(ctx)
	return report.Status == "healthy"
}
