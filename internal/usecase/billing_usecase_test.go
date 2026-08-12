package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/voltiq/server/internal/email"
	"github.com/voltiq/server/internal/payment"
	"github.com/voltiq/server/internal/repository"
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

func (m *MockEmailProvider) SendEmail(ctx context.Context, input email.SendEmailInput) error {
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

func (m *MockEmailProvider) SendTemplateEmail(ctx context.Context, input email.SendTemplateEmailInput) error {
	return nil
}

// TestBillingUseCase_SendDunningEmail tests the dunning email sending
func TestBillingUseCase_SendDunningEmail(t *testing.T) {
	ctx := context.Background()

	// Setup mocks
	mockEmailProvider := NewMockEmailProvider()
	emailTemplates, _ := email.NewTemplateLoader()

	// Create billing use case with mocks
	uc := &BillingUseCase{
		emailProvider:  mockEmailProvider,
		emailTemplates: emailTemplates,
		planConfigs:    payment.DefaultPlanConfigs(),
	}

	// Create test pending dunning
	pending := &repository.DunningPending{
		TenantID:              "test-tenant-id",
		PaymentGatewayID:     "sub_test123",
		Stage:                1,
		TenantEmail:          stringPtr("test@example.com"),
		TenantName:           stringPtr("Test Tenant"),
		PaymentCustomerID:    stringPtr("cus_test"),
		PaymentSubscriptionID: stringPtr("sub_test123"),
		CreatedAt:            time.Now(),
	}

	// Execute
	err := uc.sendDunningEmail(context.Background(), pending, 1)

	// Verify
	assert.NoError(t, err)
	assert.Len(t, mockEmailProvider.SentEmails, 1)

	sent := mockEmailProvider.SentEmails[0]
	assert.Equal(t, []string{"test@example.com"}, sent.To)
	assert.Contains(t, sent.Subject, "1º aviso")
	assert.Contains(t, sent.HTMLBody, "Test Tenant")
	assert.Contains(t, sent.HTMLBody, "1º aviso")
}

func TestBillingUseCase_SendDunningEmail_InvalidEmail(t *testing.T) {
	ctx := context.Background()

	mockEmailProvider := NewMockEmailProvider()
	emailTemplates, _ := email.NewTemplateLoader()

	uc := &BillingUseCase{
		emailProvider:  mockEmailProvider,
		emailTemplates: email.NewTemplateLoader(),
		planConfigs:    payment.DefaultPlanConfigs(),
	}

	// Test with missing email
	pending := &repository.DunningPending{
		TenantID:              "test-tenant-id",
		PaymentGatewayID:     "sub_test123",
		Stage:                1,
		TenantEmail:          nil, // Missing email
		TenantName:           stringPtr("Test Tenant"),
		PaymentCustomerID:    stringPtr("cus_test"),
		PaymentSubscriptionID: stringPtr("sub_test123"),
		CreatedAt:            time.Now(),
	}

	err := uc.sendDunningEmail(context.Background(), pending, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tenant email not available")
}

func TestBillingUseCase_SendDunningEmail_NoEmailProvider(t *testing.T) {
	ctx := context.Background()

	uc := &BillingUseCase{
		emailProvider:  nil, // No email provider
		emailTemplates: email.NewTemplateLoader(),
		planConfigs:    payment.DefaultPlanConfigs(),
	}

	pending := &repository.DunningPending{
		TenantID:              "test-tenant-id",
		PaymentGatewayID:     "sub_test123",
		Stage:                1,
		TenantEmail:          stringPtr("test@example.com"),
		TenantName:           stringPtr("Test Tenant"),
		PaymentCustomerID:    stringPtr("cus_test"),
		PaymentSubscriptionID: stringPtr("sub_test123"),
		CreatedAt:            time.Now(),
	}

	err := uc.sendDunningEmail(context.Background(), pending, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "email provider or templates not configured")
}

func stringPtr(s string) *string {
	return &s
}