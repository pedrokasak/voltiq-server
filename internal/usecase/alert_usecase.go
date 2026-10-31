package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/email"
	"github.com/voltiq/server/internal/notify"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/reqctx"
)

var (
	ErrAlertNotFound = errors.New("alert not found")
)

// AlertUseCase configures alert recipients and groups, dispatches alerts
// after each balance calculation (RF05.1, RF05.2) and keeps the delivery
// history (RF05.4). It implements BalanceNotifier.
type AlertUseCase struct {
	alertRepo  *repository.AlertRepository
	userRepo   *repository.UserRepository
	tenantRepo *repository.TenantRepository

	emailProvider email.EmailProvider
	templates     *email.TemplateLoader
	whatsapp      notify.WhatsAppSender
	appURL        string

	jobs chan alertJob
	wg   sync.WaitGroup
}

type alertJob struct {
	delivery *domain.AlertDelivery
	balance  *domain.TransformerBalance
	code     string
}

// AlertDeps groups the optional senders of the alert use case.
type AlertDeps struct {
	UserRepo      *repository.UserRepository
	TenantRepo    *repository.TenantRepository
	EmailProvider email.EmailProvider
	Templates     *email.TemplateLoader
	WhatsApp      notify.WhatsAppSender
	AppURL        string
	Workers       int
}

// NewAlertUseCase creates a new AlertUseCase. Call Start to run the
// delivery workers; without them alerts are sent synchronously.
func NewAlertUseCase(alertRepo *repository.AlertRepository, deps ...AlertDeps) *AlertUseCase {
	uc := &AlertUseCase{alertRepo: alertRepo}
	if len(deps) > 0 {
		d := deps[0]
		uc.userRepo, uc.tenantRepo = d.UserRepo, d.TenantRepo
		uc.emailProvider, uc.templates, uc.whatsapp = d.EmailProvider, d.Templates, d.WhatsApp
		uc.appURL = strings.TrimRight(d.AppURL, "/")
	}
	return uc
}

// Start runs n delivery workers until ctx is cancelled.
func (uc *AlertUseCase) Start(ctx context.Context, n int) {
	if n <= 0 {
		n = 2
	}
	uc.jobs = make(chan alertJob, 1000)
	for i := 0; i < n; i++ {
		uc.wg.Add(1)
		go func() {
			defer uc.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job := <-uc.jobs:
					uc.deliver(context.Background(), job)
				}
			}
		}()
	}
}

// ---------------------------------------------------------------- recipients

// RecipientInput creates or updates an alert recipient.
type RecipientInput struct {
	TenantID      domain.UUID
	TransformerID *domain.UUID
	GroupID       *domain.UUID
	Channel       domain.AlertChannel
	Recipient     string
	MinLevel      domain.AlertType
	Active        *bool
}

func validateRecipient(in *RecipientInput) error {
	v := &domain.ValidationError{}
	switch in.Channel {
	case domain.AlertChannelEmail:
		addr, err := mail.ParseAddress(strings.TrimSpace(in.Recipient))
		if err != nil {
			v.Fields = append(v.Fields, domain.FieldError{Field: "recipient", Message: "e-mail inválido"})
		} else {
			in.Recipient = strings.ToLower(addr.Address)
		}
	case domain.AlertChannelWhatsapp:
		phone, err := notify.NormalizePhone(in.Recipient)
		if err != nil {
			v.Fields = append(v.Fields, domain.FieldError{Field: "recipient", Message: err.Error()})
		} else {
			in.Recipient = phone
		}
		// RF05.2: WhatsApp apenas para status CRITICO
		in.MinLevel = domain.AlertTypeCritical
	default:
		v.Fields = append(v.Fields, domain.FieldError{Field: "channel", Message: "canal deve ser EMAIL ou WHATSAPP"})
	}
	if in.MinLevel == "" {
		in.MinLevel = domain.AlertTypeWarning
	}
	if in.MinLevel != domain.AlertTypeWarning && in.MinLevel != domain.AlertTypeCritical {
		v.Fields = append(v.Fields, domain.FieldError{Field: "min_level", Message: "nível deve ser WARNING ou CRITICAL"})
	}
	if in.TransformerID != nil && in.GroupID != nil {
		v.Fields = append(v.Fields, domain.FieldError{Field: "group_id", Message: "informe transformador ou grupo, não ambos"})
	}
	return v.OrNil()
}

// CreateRecipient adds an alert recipient.
func (uc *AlertUseCase) CreateRecipient(ctx context.Context, in RecipientInput) (*domain.AlertRecipient, error) {
	if err := validateRecipient(&in); err != nil {
		return nil, err
	}
	if in.GroupID != nil {
		if g, err := uc.alertRepo.GetGroup(ctx, *in.GroupID); err != nil || g == nil {
			return nil, &domain.ValidationError{Fields: []domain.FieldError{{Field: "group_id", Message: "grupo não encontrado"}}}
		}
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	a := &domain.AlertRecipient{
		ID: domain.UUID(uuid.New().String()), TenantID: in.TenantID, TransformerID: in.TransformerID, GroupID: in.GroupID,
		Channel: in.Channel, Recipient: in.Recipient, MinLevel: in.MinLevel, Active: active, CreatedAt: time.Now(),
	}
	if err := uc.alertRepo.CreateRecipient(ctx, a); err != nil {
		return nil, fmt.Errorf("create recipient: %w", err)
	}
	return uc.alertRepo.GetRecipient(ctx, a.ID)
}

// UpdateRecipient changes an alert recipient.
func (uc *AlertUseCase) UpdateRecipient(ctx context.Context, id domain.UUID, in RecipientInput) (*domain.AlertRecipient, error) {
	cur, err := uc.alertRepo.GetRecipient(ctx, id)
	if err != nil || cur == nil {
		return nil, ErrAlertNotFound
	}
	if in.Channel == "" {
		in.Channel = cur.Channel
	}
	if in.Recipient == "" {
		in.Recipient = cur.Recipient
	}
	if in.MinLevel == "" {
		in.MinLevel = cur.MinLevel
	}
	if err := validateRecipient(&in); err != nil {
		return nil, err
	}
	cur.TransformerID, cur.GroupID = in.TransformerID, in.GroupID
	cur.Channel, cur.Recipient, cur.MinLevel = in.Channel, in.Recipient, in.MinLevel
	if in.Active != nil {
		cur.Active = *in.Active
	}
	if err := uc.alertRepo.UpdateRecipient(ctx, cur); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrAlertNotFound
		}
		return nil, err
	}
	return uc.alertRepo.GetRecipient(ctx, id)
}

// ListRecipients lists recipients (optionally of one transformer).
func (uc *AlertUseCase) ListRecipients(ctx context.Context, tenantID domain.UUID, transformerID *domain.UUID) ([]*domain.AlertRecipient, error) {
	return uc.alertRepo.ListRecipients(ctx, tenantID, transformerID)
}

// DeleteRecipient removes a recipient.
func (uc *AlertUseCase) DeleteRecipient(ctx context.Context, id domain.UUID) error {
	if err := uc.alertRepo.DeleteRecipient(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrAlertNotFound
		}
		return err
	}
	return nil
}

// ---------------------------------------------------------------- groups

// GroupInput creates or updates a transformer group.
type GroupInput struct {
	TenantID       domain.UUID
	Name           string
	Description    string
	TransformerIDs []domain.UUID
}

// SaveGroup creates (id == "") or updates a transformer group.
func (uc *AlertUseCase) SaveGroup(ctx context.Context, id domain.UUID, in GroupInput) (*domain.TransformerGroup, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, &domain.ValidationError{Fields: []domain.FieldError{{Field: "name", Message: "nome é obrigatório"}}}
	}
	g := &domain.TransformerGroup{ID: id, TenantID: in.TenantID, Name: name, Description: strings.TrimSpace(in.Description),
		TransformerIDs: in.TransformerIDs, CreatedAt: time.Now()}
	create := id == ""
	if create {
		g.ID = domain.UUID(uuid.New().String())
	}
	if err := uc.alertRepo.SaveGroup(ctx, g, create); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: já existe um grupo com este nome", ErrConflict)
		}
		return nil, err
	}
	groups, err := uc.alertRepo.ListGroups(ctx, in.TenantID)
	if err != nil {
		return nil, err
	}
	for _, x := range groups {
		if x.ID == g.ID {
			return x, nil
		}
	}
	return g, nil
}

// ListGroups lists transformer groups.
func (uc *AlertUseCase) ListGroups(ctx context.Context, tenantID domain.UUID) ([]*domain.TransformerGroup, error) {
	return uc.alertRepo.ListGroups(ctx, tenantID)
}

// DeleteGroup removes a group and its recipients.
func (uc *AlertUseCase) DeleteGroup(ctx context.Context, tenantID, id domain.UUID) error {
	if err := uc.alertRepo.DeleteGroup(ctx, tenantID, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// ---------------------------------------------------------------- history

// ListDeliveries lists the alert history (RF05.4).
func (uc *AlertUseCase) ListDeliveries(ctx context.Context, tenantID domain.UUID, f repository.DeliveryFilter) ([]*domain.AlertDelivery, int, error) {
	return uc.alertRepo.ListDeliveries(ctx, tenantID, f)
}

// GetDelivery returns one alert of the history.
func (uc *AlertUseCase) GetDelivery(ctx context.Context, id domain.UUID) (*domain.AlertDelivery, error) {
	d, err := uc.alertRepo.GetDelivery(ctx, id)
	if err != nil || d == nil {
		return nil, ErrAlertNotFound
	}
	return d, nil
}

// Stats counts the alerts of the last days.
func (uc *AlertUseCase) Stats(ctx context.Context, tenantID domain.UUID, days int) (*repository.AlertStats, error) {
	if days <= 0 {
		days = 30
	}
	return uc.alertRepo.Stats(ctx, tenantID, time.Now().AddDate(0, 0, -days))
}

// ---------------------------------------------------------------- dispatch

// BalanceCalculated dispara os alertas de um balanco em ATENCAO ou CRITICO
// para os destinatarios que cobrem o trafo. Sem destinatario configurado,
// avisa por e-mail os usuarios OWNER e ADMIN do tenant.
func (uc *AlertUseCase) BalanceCalculated(ctx context.Context, t *domain.Transformer, b *domain.TransformerBalance) {
	if b.Status != domain.BalanceStatusWarning && b.Status != domain.BalanceStatusCritical {
		return
	}
	level := domain.AlertTypeWarning
	if b.Status == domain.BalanceStatusCritical {
		level = domain.AlertTypeCritical
	}

	recipients, err := uc.alertRepo.RecipientsFor(ctx, t.TenantID, t.ID)
	if err != nil {
		slog.Error("failed to load alert recipients", "transformer_id", t.ID, "error", err)
		return
	}
	if len(recipients) == 0 {
		recipients = uc.fallbackRecipients(ctx, t.TenantID)
	}

	for _, rc := range recipients {
		if level == domain.AlertTypeWarning && rc.MinLevel == domain.AlertTypeCritical {
			continue
		}
		if rc.Channel == domain.AlertChannelWhatsapp && level != domain.AlertTypeCritical {
			continue
		}
		var recipientID *domain.UUID
		if rc.ID != "" {
			id := rc.ID
			recipientID = &id
		}
		loss, limit := b.LossPct, t.LossLimitPctValue()
		d := &domain.AlertDelivery{
			ID: domain.UUID(uuid.New().String()), TenantID: t.TenantID, TransformerID: t.ID, BalanceID: b.ID,
			RecipientID: recipientID, Type: level, Channel: rc.Channel, Recipient: rc.Recipient,
			DeliveryStatus: domain.AlertDeliveryPending, LossPct: &loss, LimitPct: &limit, CreatedAt: time.Now(),
		}
		created, err := uc.alertRepo.CreateDelivery(ctx, d)
		if err != nil {
			slog.Error("failed to record alert", "transformer_id", t.ID, "error", err)
			continue
		}
		if !created {
			continue // ja alertado para este balanco
		}
		job := alertJob{delivery: d, balance: b, code: t.Code}
		if uc.jobs == nil {
			uc.deliver(detach(ctx), job)
			continue
		}
		select {
		case uc.jobs <- job:
		default:
			uc.deliver(detach(ctx), job)
		}
	}
}

// detach keeps the request identity but drops its cancellation.
func detach(ctx context.Context) context.Context {
	out := context.Background()
	if id, ok := reqctx.FromContext(ctx); ok {
		out = reqctx.WithIdentity(out, id)
	}
	return out
}

func (uc *AlertUseCase) fallbackRecipients(ctx context.Context, tenantID domain.UUID) []*domain.AlertRecipient {
	if uc.userRepo == nil {
		return nil
	}
	users, err := uc.userRepo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil
	}
	var out []*domain.AlertRecipient
	for _, u := range users {
		if u.Active && (u.Role == domain.UserRoleOwner || u.Role == domain.UserRoleAdmin) {
			out = append(out, &domain.AlertRecipient{Channel: domain.AlertChannelEmail, Recipient: u.Email, MinLevel: domain.AlertTypeWarning, Active: true})
		}
	}
	return out
}

func (uc *AlertUseCase) deliver(ctx context.Context, job alertJob) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	err := uc.send(ctx, job)
	status := domain.AlertDeliverySent
	var msg *string
	if err != nil {
		status = domain.AlertDeliveryError
		m := err.Error()
		if len(m) > 500 {
			m = m[:500]
		}
		msg = &m
	}
	if mErr := uc.alertRepo.MarkDelivery(ctx, job.delivery.ID, status, msg); mErr != nil {
		slog.Error("failed to update alert delivery", "alert_id", job.delivery.ID, "error", mErr)
	}
	slog.Info("alert processed", "alert_id", job.delivery.ID, "transformer", job.code,
		"channel", job.delivery.Channel, "status", status, "loss_pct", job.balance.LossPct)
}

func (uc *AlertUseCase) send(ctx context.Context, job alertJob) error {
	d, b := job.delivery, job.balance
	tenantName := ""
	if uc.tenantRepo != nil {
		if t, err := uc.tenantRepo.GetByID(ctx, d.TenantID); err == nil && t != nil {
			tenantName = t.Name
		}
	}
	label, color, bg := "ATENÇÃO", "#b45309", "#fffbeb"
	if d.Type == domain.AlertTypeCritical {
		label, color, bg = "CRÍTICO", "#dc2626", "#fef2f2"
	}
	period := b.PeriodStart.Format("02/01/2006") + " a " + b.PeriodEnd.Format("02/01/2006")
	limit := 10.0
	if d.LimitPct != nil {
		limit = *d.LimitPct
	}

	switch d.Channel {
	case domain.AlertChannelWhatsapp:
		if uc.whatsapp == nil {
			return notify.ErrNotConfigured
		}
		msg := fmt.Sprintf("*Voltiq - alerta %s*\nTransformador %s: perda de %s%% (limite %s%%) no período %s.\nPerda total: %s kWh.\n%s",
			label, job.code, formatBR(b.LossPct, 2), formatBR(limit, 1), period, formatBR(b.LossKWh, 0),
			uc.appURL+"/transformers/"+string(d.TransformerID))
		return uc.whatsapp.SendText(ctx, d.Recipient, msg)
	default:
		if uc.emailProvider == nil {
			return notify.ErrNotConfigured
		}
		subject := fmt.Sprintf("[%s] Transformador %s com perda de %s%%", label, job.code, formatBR(b.LossPct, 2))
		nonTech := "-"
		if b.NonTechnicalLossKWh != nil {
			nonTech = formatBR(*b.NonTechnicalLossKWh, 0) + " kWh"
		}
		html := ""
		if uc.templates != nil {
			var err error
			html, err = uc.templates.Render("alert_balance", map[string]any{
				"tenant_name": tenantName, "transformer_code": job.code, "status_label": label,
				"color": color, "color_bg": bg, "loss_pct": formatBR(b.LossPct, 2), "limit_pct": formatBR(limit, 1),
				"period": period, "injected_kwh": formatBR(b.EnergyInjectedKWh, 0),
				"consumption_kwh": formatBR(b.TotalConsumptionKWh, 0), "loss_kwh": formatBR(b.LossKWh, 0),
				"non_technical_kwh": nonTech, "transformer_url": uc.appURL + "/transformers/" + string(d.TransformerID),
			})
			if err != nil {
				return err
			}
		}
		text := fmt.Sprintf("%s\n\nPeríodo: %s\nPerda: %s%% (limite %s%%)\nPerda total: %s kWh\n\n%s",
			subject, period, formatBR(b.LossPct, 2), formatBR(limit, 1), formatBR(b.LossKWh, 0),
			uc.appURL+"/transformers/"+string(d.TransformerID))
		return uc.emailProvider.SendEmail(ctx, email.SendEmailInput{To: []string{d.Recipient}, Subject: subject, HTMLBody: html, TextBody: text})
	}
}

// RetryFailed reenvia alertas com erro (ate 3 tentativas). Usado pelo
// agendador de jobs.
func (uc *AlertUseCase) RetryFailed(ctx context.Context, balanceRepo *repository.BalanceRepository) (int, error) {
	items, err := uc.alertRepo.FailedForRetry(ctx, 3, time.Now().Add(-5*time.Minute))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range items {
		b, err := balanceRepo.GetByID(ctx, d.BalanceID)
		if err != nil || b == nil {
			continue
		}
		uc.deliver(ctx, alertJob{delivery: d, balance: b, code: d.TransformerCode})
		n++
	}
	return n, nil
}

// SendTest sends a test message to a recipient (botao "enviar teste").
func (uc *AlertUseCase) SendTest(ctx context.Context, id domain.UUID) error {
	rc, err := uc.alertRepo.GetRecipient(ctx, id)
	if err != nil || rc == nil {
		return ErrAlertNotFound
	}
	switch rc.Channel {
	case domain.AlertChannelWhatsapp:
		if uc.whatsapp == nil {
			return badRequest("WhatsApp não configurado no servidor (ZAPI_INSTANCE_ID/ZAPI_TOKEN)")
		}
		return uc.whatsapp.SendText(ctx, rc.Recipient, "Voltiq: mensagem de teste. Você receberá aqui os alertas CRÍTICOS de perdas.")
	default:
		if uc.emailProvider == nil {
			return badRequest("e-mail não configurado no servidor (RESEND_API_KEY)")
		}
		return uc.emailProvider.SendEmail(ctx, email.SendEmailInput{
			To: []string{rc.Recipient}, Subject: "Voltiq - teste de alerta",
			TextBody: "Mensagem de teste. Você receberá neste endereço os alertas de perdas configurados no Voltiq.",
			HTMLBody: "<p>Mensagem de teste. Você receberá neste endereço os alertas de perdas configurados no Voltiq.</p>",
		})
	}
}
