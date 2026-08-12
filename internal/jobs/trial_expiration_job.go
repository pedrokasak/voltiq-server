package jobs

import (
	"context"
	"log"
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
			log.Printf("Erro ao suspender tenant %s: %v", tenant.ID, err)
			continue
		}

		// Log de auditoria
		log.Printf("AUDIT: tenant %s (%s) status alterado %s -> %s (trial expirado em %s)",
			tenant.ID, tenant.Name, previousStatus, tenant.Status, tenant.TrialExpiresAt)
	}

	log.Printf("TrialExpirationJob concluído: %d tenants suspensos", len(tenants))
	return nil
}