# Subagent: alerts
# Escopo: sistema de alertas por email, configuração por trafo e notificações

## Quando chamar este subagent
- Implementar envio de email no alert_usecase.go
- Adicionar novo tipo de alerta
- Configurar destinatários por trafo
- Depurar alertas não disparando
- Adicionar canal WhatsApp (Fase 2)

## Estado atual (Agosto 2026)

### Pronto
- `internal/email/service.go` — Resend client funcional
- `internal/email/templates.go` — templates HTML de alerta
- `internal/usecase/alert_usecase.go` — lógica de config e idempotência
- `internal/delivery/handler/alert_handler.go` — endpoints HTTP
- Router: `POST /transformers/{id}/alert-config` registrado

### Falta implementar
O método `ProcessBalanceAlert` existe mas NÃO chama `emailSvc.Send()`.
Esta é a task mais urgente do sistema de alertas.

## Implementação necessária

```go
// internal/usecase/alert_usecase.go
// No método sendEmailAlert(), após renderizar o template:

subject, htmlBody, err := email.RenderAlertEmail(emailData)
if err != nil {
    // log e marcar como ERROR
    return
}

// ISTO ESTÁ FALTANDO:
if err := uc.emailSvc.Send(ctx, recipient.Email, subject, htmlBody); err != nil {
    slog.Error("falha ao enviar alerta",
        "error", err,
        "recipient", recipient.Email,
        "transformer", transformer.Code,
    )
    uc.updateAlertStatus(ctx, alertID, domain.AlertDeliveryError, nil)
    return
}

sentAt := time.Now()
uc.updateAlertStatus(ctx, alertID, domain.AlertDeliverySent, &sentAt)
```

## Integração com balance_usecase

Após salvar balanço, disparar alerta em goroutine separada:

```go
// internal/usecase/balance_usecase.go
// Após salvar resultado do balanço:

if result.Status != domain.BalanceStatusNormal {
    go func() {
        recipients, _ := uc.alertRepo.GetRecipientsByTransformer(ctx, transformer.ID)
        uc.alertUseCase.ProcessBalanceAlert(ctx, result, transformer, recipients)
    }()
}
```

## Arquivos de escopo

```
internal/usecase/alert_usecase.go
internal/delivery/handler/alert_handler.go
internal/email/service.go
internal/email/templates.go
internal/repository/alert_repository.go
migrations/005_imports_alerts.sql   ← tabela alerts
migrations/009_alert_config.sql     ← config por trafo (criar se não existir)
```

## Tipos de alerta

```go
const (
    AlertTypeWarning  AlertType = "WARNING"   // 80-100% do limite ANEEL
    AlertTypeCritical AlertType = "CRITICAL"  // acima do limite ANEEL
)

const (
    AlertChannelEmail    AlertChannel = "EMAIL"
    AlertChannelWhatsApp AlertChannel = "WHATSAPP" // Fase 2
    AlertChannelApp      AlertChannel = "APP"       // notificação in-app
)
```

## Idempotência — não duplicar alertas

```go
// Verificar antes de enviar:
exists, _ := uc.alertRepo.ExistsForBalance(ctx, balanceID, AlertChannelEmail)
if exists {
    slog.Info("alerta já enviado para este balanço, ignorando",
        "balance_id", balanceID,
        "channel", AlertChannelEmail,
    )
    return
}
```

## Threshold por trafo

Cada trafo tem threshold configurável (padrão: 80% do limite ANEEL).
Se limite ANEEL = 6%, threshold padrão = 4,8%.

```go
// Calcular threshold dinâmico:
threshold := transformer.LossLimitPct * 0.80
if config.ThresholdPct > 0 {
    threshold = config.ThresholdPct // usar config específica se existir
}

// Só alertar se perda >= threshold:
if balance.LossPct >= threshold {
    // disparar alerta
}
```
