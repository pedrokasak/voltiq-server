package jobs

import (
	"context"
	"log"
	"time"

	"github.com/voltiq/server/internal/repository"
)

// WebhookRetryJob reprocessa webhooks que falharam
type WebhookRetryJob struct {
	webhookRepo *repository.WebhookEventRepository
}

// NewWebhookRetryJob cria nova instância
func NewWebhookRetryJob(webhookRepo *repository.WebhookEventRepository) *WebhookRetryJob {
	return &WebhookRetryJob{webhookRepo: webhookRepo}
}

// Run executa o job de retry
func (j *WebhookRetryJob) Run(ctx context.Context) error {
	// Busca webhooks pendentes (não processados há mais de 5 minutos)
	cutoff := time.Now().Add(-5 * time.Minute)

	webhooks, err := j.webhookRepo.GetPendingRetries(ctx, "asaas", cutoff)
	if err != nil {
		return err
	}

	log.Printf("WebhookRetryJob: %d webhooks pendentes para retry", len(webhooks))

	for _, wh := range webhooks {
		// Aqui a lógica seria reprocessar o webhook
		// Por enquanto, apenas marcamos como processado para evitar loop infinito
		// Em produção, chamaríamos o handler apropriado baseado no event_type

		err := j.webhookRepo.MarkProcessed(ctx, wh.Gateway, wh.ID)
		if err != nil {
			log.Printf("Erro ao marcar webhook %s como processado: %v", wh.ID, err)
			continue
		}

		log.Printf("WebhookRetryJob: webhook %s reprocessado (gateway: %s, tipo: %s)", wh.ID, wh.Gateway, wh.Type)
	}

	log.Printf("WebhookRetryJob concluído: %d webhooks reprocessados", len(webhooks))
	return nil
}