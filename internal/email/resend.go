package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/voltiq/server/internal/config"
)

// ResendProvider implements EmailProvider for Resend
type ResendProvider struct {
	apiKey     string
	fromEmail  string
	fromName   string
	replyTo    string
	baseURL    string
	httpClient *http.Client
}

// NewResendProvider creates a new Resend provider
func NewResendProvider(cfg *config.EmailConfig) *ResendProvider {
	return &ResendProvider{
		apiKey:     cfg.ResendAPIKey,
		fromEmail:  cfg.FromEmail,
		fromName:   cfg.FromName,
		replyTo:    cfg.ReplyTo,
		baseURL:    cfg.BaseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// SendEmail sends a simple email via Resend API
func (p *ResendProvider) SendEmail(ctx context.Context, input SendEmailInput) error {
	if p.apiKey == "" {
		return fmt.Errorf("resend API key not configured")
	}

	from := input.From
	if from == "" {
		from = fmt.Sprintf("%s <%s>", p.fromName, p.fromEmail)
	}

	replyTo := input.ReplyTo
	if replyTo == "" {
		replyTo = p.replyTo
	}

	payload := map[string]any{
		"from":     from,
		"to":       input.To,
		"subject":  input.Subject,
		"html":     input.HTMLBody,
		"text":     input.TextBody,
		"reply_to": replyTo,
	}

	return p.sendRequest(ctx, "/emails", payload)
}

// SendTemplateEmail sends a templated email via Resend API
func (p *ResendProvider) SendTemplateEmail(ctx context.Context, input SendTemplateEmailInput) error {
	if p.apiKey == "" {
		return fmt.Errorf("resend API key not configured")
	}

	from := input.From
	if from == "" {
		from = fmt.Sprintf("%s <%s>", p.fromName, p.fromEmail)
	}

	replyTo := input.ReplyTo
	if replyTo == "" {
		replyTo = p.replyTo
	}

	payload := map[string]any{
		"from":       from,
		"to":         input.To,
		"subject":    input.Subject,
		"template_id": input.TemplateID,
		"data":       input.TemplateData,
		"reply_to":   replyTo,
	}

	return p.sendRequest(ctx, "/emails", payload)
}

// sendRequest sends HTTP request to Resend API
func (p *ResendProvider) sendRequest(ctx context.Context, endpoint string, payload map[string]any) error {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := "https://api.resend.com" + endpoint
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp map[string]any
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("resend API error: status=%d, body=%v", resp.StatusCode, errResp)
	}

	return nil
}

// DunningEmailData represents data for dunning email templates
type DunningEmailData struct {
	TenantName       string
	PlanName         string
	Amount           string
	DueDate          string
	DaysOverdue      int
	BillingURL       string
	SupportEmail     string
	CompanyName      string
	Stage            int // 1, 2, or 3
}

// GetDunningSubject returns subject based on stage
func GetDunningSubject(stage int, planName string) string {
	switch stage {
	case 1:
		return fmt.Sprintf("[Voltiq] Pagamento em atraso — %s — 1º aviso", planName)
	case 2:
		return fmt.Sprintf("[Voltiq] Pagamento em atraso — %s — 2º aviso (suspensão em 8 dias)", planName)
	case 3:
		return fmt.Sprintf("[Voltiq] ÚLTIMO AVISO — %s — Suspensão amanhã", planName)
	default:
		return fmt.Sprintf("[Voltiq] Pagamento em atraso — %s", planName)
	}
}

// GetDunningTemplateID returns Resend template ID for stage
func GetDunningTemplateID(stage int) string {
	switch stage {
	case 1:
		return "dunning-stage-1"
	case 2:
		return "dunning-stage-2"
	case 3:
		return "dunning-stage-3"
	default:
		return "dunning-stage-1"
	}
}

// BuildDunningTemplateData builds template data for dunning email
func BuildDunningTemplateData(data DunningEmailData) map[string]any {
	return map[string]any{
		"tenant_name":   data.TenantName,
		"plan_name":     data.PlanName,
		"amount":        data.Amount,
		"due_date":      data.DueDate,
		"days_overdue":  data.DaysOverdue,
		"billing_url":   data.BillingURL,
		"support_email": data.SupportEmail,
		"company_name":  data.CompanyName,
		"stage":         data.Stage,
		"current_year":  time.Now().Year(),
	}
}