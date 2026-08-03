package email

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockEmailProvider implements EmailProvider for testing
type MockEmailProvider struct {
	SentEmails []SentEmailRecord
}

type SentEmailRecord struct {
	To       []string
	Subject  string
	HTMLBody string
	TextBody string
	From     string
	ReplyTo  string
}

func NewMockEmailProvider() *MockEmailProvider {
	return &MockEmailProvider{
		SentEmails: make([]SentEmailRecord, 0),
	}
}

func (m *MockEmailProvider) SendEmail(ctx context.Context, input SendEmailInput) error {
	m.SentEmails = append(m.SentEmails, SentEmailRecord{
		To:       input.To,
		Subject:  input.Subject,
		HTMLBody: input.HTMLBody,
		TextBody: input.TextBody,
		From:     input.From,
		ReplyTo:  input.ReplyTo,
	})
	return nil
}

func (m *MockEmailProvider) SendTemplateEmail(ctx context.Context, input SendTemplateEmailInput) error {
	return m.SendEmail(ctx, SendEmailInput{
		To:      input.To,
		Subject: input.Subject,
		HTMLBody: "template: " + input.TemplateID,
		From:    input.From,
		ReplyTo: input.ReplyTo,
	})
}

// TestGetDunningSubject tests the subject generation for each stage
func TestGetDunningSubject(t *testing.T) {
	tests := []struct {
		name       string
		stage      int
		planName   string
		expected   string
	}{
		{
			name:       "stage 1",
			stage:      1,
			planName:   "Plano Starter",
			expected:   "[Voltiq] Pagamento em atraso — Plano Starter — 1º aviso",
		},
		{
			name:       "stage 2",
			stage:      2,
			planName:   "Plano Pro",
			expected:   "[Voltiq] Pagamento em atraso — Plano Pro — 2º aviso (suspensão em 8 dias)",
		},
		{
			name:       "stage 3",
			stage:      3,
			planName:   "Plano Enterprise",
			expected:   "[Voltiq] ÚLTIMO AVISO — Plano Enterprise — Suspensão amanhã",
		},
		{
			name:       "invalid stage defaults to stage 1",
			stage:      99,
			planName:   "Plano Teste",
			expected:   "[Voltiq] Pagamento em atraso — Plano Teste",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDunningSubject(tt.stage, tt.planName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetDunningTemplateID tests the template ID mapping
func TestGetDunningTemplateID(t *testing.T) {
	tests := []struct {
		name     string
		stage    int
		expected string
	}{
		{name: "stage 1", stage: 1, expected: "dunning-stage-1"},
		{name: "stage 2", stage: 2, expected: "dunning-stage-2"},
		{name: "stage 3", stage: 3, expected: "dunning-stage-3"},
		{name: "invalid stage defaults to stage 1", stage: 99, expected: "dunning-stage-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetDunningTemplateID(tt.stage)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildDunningTemplateData tests the template data builder
func TestBuildDunningTemplateData(t *testing.T) {
	data := DunningEmailData{
		TenantName:   "Cooperativa Exemplo",
		PlanName:     "Plano Pro",
		Amount:       "R$ 299,00",
		DueDate:      "15/01/2026",
		DaysOverdue:  7,
		BillingURL:   "https://app.voltiq.com.br/billing",
		SupportEmail: "suporte@voltiq.com.br",
		CompanyName:  "Voltiq Software",
		Stage:        2,
	}

	result := BuildDunningTemplateData(data)

	require.NotNil(t, result)
	assert.Equal(t, "Cooperativa Exemplo", result["tenant_name"])
	assert.Equal(t, "Plano Pro", result["plan_name"])
	assert.Equal(t, "R$ 299,00", result["amount"])
	assert.Equal(t, "15/01/2026", result["due_date"])
	assert.Equal(t, 7, result["days_overdue"])
	assert.Equal(t, "https://app.voltiq.com.br/billing", result["billing_url"])
	assert.Equal(t, "suporte@voltiq.com.br", result["support_email"])
	assert.Equal(t, "Voltiq Software", result["company_name"])
	assert.Equal(t, 2, result["stage"])
}

// TestMockEmailProvider tests the mock provider
func TestMockEmailProvider(t *testing.T) {
	ctx := context.Background()
	mock := NewMockEmailProvider()

	err := mock.SendEmail(ctx, SendEmailInput{
		To:       []string{"test@example.com"},
		Subject:  "Test Subject",
		HTMLBody: "<p>Test</p>",
		TextBody: "Test",
	})

	require.NoError(t, err)
	assert.Len(t, mock.SentEmails, 1)
	assert.Equal(t, []string{"test@example.com"}, mock.SentEmails[0].To)
	assert.Equal(t, "Test Subject", mock.SentEmails[0].Subject)
	assert.Equal(t, "<p>Test</p>", mock.SentEmails[0].HTMLBody)
}