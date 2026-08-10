# Subagent: billing
# Escopo: tudo relacionado a planos, pagamentos, Asaas, trial e ciclo de vida do tenant

## Quando chamar este subagent
- Configurar ou depurar integração com Asaas
- Alterar lógica de ativação/suspensão de plano
- Implementar novo email do ciclo de vida (trial expirando, pagamento confirmado)
- Adicionar nova funcionalidade de billing
- Depurar webhook do Asaas

## NUNCA fazer neste subagent
- Alterar cálculos PRODIST M7 → usar subagent prodist
- Alterar lógica de autenticação → usar subagent auth
- Alterar migrations 001–010 → criar nova migration sequencial

## Arquivos de escopo

```
internal/usecase/billing_usecase.go       ← lógica principal
internal/usecase/billing_usecase_test.go  ← testes
internal/delivery/handler/billing_handler.go
internal/repository/billing_repository.go
internal/payment/                         ← AsaasProvider
internal/email/templates.go               ← templates de emails billing
migrations/007_billing_fase_a.sql
migrations/008_billing_fase_b_payment.sql
migrations/009_billing_fase_c_dunning_metrics.sql
migrations/010_billing_fase_c_proration.sql
web/src/pages/billing/BillingPage.tsx
web/src/api/billing.ts
```

## Planos e limites

```go
var PlanLimits = map[string]struct{ MaxUsers, MaxTransformers int }{
    "trial":      {1,  10},
    "starter":    {3,  50},
    "pro":        {10, 250},
    "enterprise": {-1, -1}, // -1 = ilimitado
}

var PlanPrices = map[string]float64{
    "trial":      0,
    "starter":    890,
    "pro":        2490,
    "enterprise": 6900,
}
```

## Status do tenant

```
trial            → trial ativo (dentro do prazo)
active           → pagante ativo
expired          → trial expirou sem assinar
suspended        → inadimplente ou suspenso manualmente
pending_payment  → aguardando confirmação do primeiro pagamento
cancelled        → cancelou a assinatura
```

## Fluxo de webhook Asaas

```
POST /api/v1/webhooks/asaas
  → validar ASAAS_WEBHOOK_KEY
  → verificar idempotência (payment_webhook_events)
  → processar evento:
    PAYMENT_RECEIVED    → ativar plano
    PAYMENT_OVERDUE     → iniciar dunning
    SUBSCRIPTION_DELETED → marcar como cancelled
```

## Variáveis de ambiente necessárias

```env
ASAAS_API_KEY=aact_xxx        # chave da API Asaas
ASAAS_WEBHOOK_KEY=xxx         # validação de webhooks
ASAAS_SANDBOX=true            # false em produção
RESEND_API_KEY=re_xxx         # emails de dunning
```

## Emails do ciclo de vida implementados

| Gatilho | Template | Status |
|---|---|---|
| Trial dia 7 | trial_warning_7d | implementado |
| Trial dia 2 | trial_warning_2d | implementado |
| Trial expirou | trial_expired | implementado |
| Pagamento confirmado | payment_confirmed | implementado |
| Dunning stage 1 (3d) | dunning_1 | implementado |
| Dunning stage 2 (7d) | dunning_2 | implementado |
| Dunning stage 3 (14d) | dunning_3 + suspender | implementado |

## Testar em sandbox

```bash
# Simular pagamento confirmado via Asaas sandbox:
curl -X POST https://api.asaas.com/v3/payments/{id}/payWithCreditCard \
  -H "access_token: $ASAAS_API_KEY" \
  -d '{"creditCard": {...}}'
```
