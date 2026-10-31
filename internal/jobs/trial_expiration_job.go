package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/repository"
)

// TrialExpirationJob verifica e suspende trials expirados
type TrialExpirationJob struct {
	tenantRepo *repository.TenantRepository
}

// NewTrialExpirationJob cria nova instância
func NewTrialExpirationJob(tenantRepo *repository.TenantRepository) *TrialExpirationJob {
	return &TrialExpirationJob{tenantRepo: tenantRepo}
}

// Run executa o job
func (j *TrialExpirationJob) Run(ctx context.Context) error {
	now := time.Now()

	// Busca tenants em TRIAL com trial_expires_at expirado
	tenants, err := j.tenantRepo.GetExpiredTrials(ctx, now)
	if err != nil {
		return err
	}

	for _, tenant := range tenants {
		previousStatus := tenant.Status
		tenant.Status = domain.TenantStatusSuspended
		tenant.SuspendedAt = &now
		tenant.UpdatedAt = now

		if err := j.tenantRepo.Update(ctx, tenant); err != nil {
			slog.Error("trial suspension failed", "tenant_id", tenant.ID, "error", err)
			continue
		}

		// Log de auditoria
		slog.Info("tenant suspended: trial expired", "tenant_id", tenant.ID, "from_status", previousStatus,
			"to_status", tenant.Status, "trial_expires_at", tenant.TrialExpiresAt)
	}

	slog.Info("trial expiration done", "suspended", len(tenants))
	return nil
}
