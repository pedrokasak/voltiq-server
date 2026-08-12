package payment

import (
	"fmt"

	"github.com/voltiq/server/internal/domain"
)

// ProviderType identifies the payment provider
type ProviderType string

const (
	ProviderTypeAsaas ProviderType = "asaas"
	// Future: ProviderTypeStripe ProviderType = "stripe"
	// Future: ProviderTypeMercadoPago ProviderType = "mercadopago"
)

// ProviderConfig holds configuration for a payment provider
type ProviderConfig struct {
	Type       ProviderType
	APIKey     string
	WebhookKey string
	Sandbox    bool
	// Provider-specific config can be added here
}

// NewProvider creates a payment provider based on config
func NewProvider(config ProviderConfig) (PaymentProvider, error) {
	switch config.Type {
	case ProviderTypeAsaas:
		return NewAsaasProvider(config.APIKey, config.WebhookKey, config.Sandbox), nil
	default:
		return nil, fmt.Errorf("unsupported payment provider: %s", config.Type)
	}
}

// GetProviderFromEnv creates provider from environment variables
func GetProviderFromEnv() (PaymentProvider, error) {
	// Read from env - in real app would use a config package
	// For now, we'll expect these to be set:
	// ASAAS_API_KEY, ASAAS_WEBHOOK_KEY, ASAAS_SANDBOX
	return nil, nil // Placeholder - will be implemented in usecase init
}

// PlanConfig maps our plans to payment provider plans
type PlanConfig struct {
	ProviderPlanID string
	BillingCycle   BillingCycle
	Value          float64
	Description    string
	MaxUsers       int
}

// DefaultPlanConfigs returns the default plan configurations for Asaas
func DefaultPlanConfigs() map[domain.TenantPlan]PlanConfig {
	return map[domain.TenantPlan]PlanConfig{
		domain.TenantPlanStarter: {
			ProviderPlanID: "voltiq-starter",
			BillingCycle:   BillingCycleMonthly,
			Value:          99.00,
			Description:    "Plano Starter - Voltiq Software",
			MaxUsers:       10,
		},
		domain.TenantPlanPro: {
			ProviderPlanID: "voltiq-pro",
			BillingCycle:   BillingCycleMonthly,
			Value:          299.00,
			Description:    "Plano Pro - Voltiq Software",
			MaxUsers:       50,
		},
		domain.TenantPlanEnterprise: {
			ProviderPlanID: "voltiq-enterprise",
			BillingCycle:   BillingCycleMonthly,
			Value:          999.00,
			Description:    "Plano Enterprise - Voltiq Software",
			MaxUsers:       999999,
		},
	}
}