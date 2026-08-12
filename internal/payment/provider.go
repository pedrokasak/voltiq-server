package payment

import (
	"context"
	"time"

	"github.com/voltiq/server/internal/domain"
)

// PaymentProvider defines the interface for payment gateway operations
type PaymentProvider interface {
	// Customer operations
	CreateCustomer(ctx context.Context, input CreateCustomerInput) (*Customer, error)
	GetCustomer(ctx context.Context, customerID string) (*Customer, error)
	UpdateCustomer(ctx context.Context, customerID string, input UpdateCustomerInput) (*Customer, error)

	// Subscription operations
	CreateSubscription(ctx context.Context, input CreateSubscriptionInput) (*Subscription, error)
	GetSubscription(ctx context.Context, subscriptionID string) (*Subscription, error)
	UpdateSubscription(ctx context.Context, subscriptionID string, input UpdateSubscriptionInput) (*Subscription, error)
	CancelSubscription(ctx context.Context, subscriptionID string) error
	ListSubscriptions(ctx context.Context, filter SubscriptionFilter) ([]*Subscription, error)

	// Payment operations
	GetPayment(ctx context.Context, paymentID string) (*Payment, error)
	ListPayments(ctx context.Context, filter PaymentFilter) ([]*Payment, error)
	RefundPayment(ctx context.Context, paymentID string, input RefundInput) (*Refund, error)
	CreatePayment(ctx context.Context, input CreatePaymentInput) (*Payment, error)

	// Webhook handling
	VerifyWebhookSignature(payload []byte, signature string) bool
	ParseWebhookEvent(payload []byte) (*WebhookEvent, error)
}

// Customer represents a customer in the payment gateway
type Customer struct {
	ID           string
	ExternalID   string // Our tenant/user ID
	Name         string
	Email        string
	Document     string // CPF/CNPJ
	Phone        string
	Address      string
	AddressNumber string
	Province     string
	PostalCode   string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Metadata     map[string]string
}

// CreateCustomerInput input for creating a customer
type CreateCustomerInput struct {
	ExternalID    string
	Name          string
	Email         string
	Document      string
	Phone         string
	Address       string
	AddressNumber string
	Province     string
	PostalCode    string
	Metadata      map[string]string
}

// UpdateCustomerInput input for updating a customer
type UpdateCustomerInput struct {
	Name          *string
	Email         *string
	Phone         *string
	Address       *string
	AddressNumber *string
	Province      *string
	PostalCode    *string
	Metadata      map[string]string
}

// Subscription represents a recurring subscription
type Subscription struct {
	ID              string
	CustomerID      string
	ExternalID      string // Our plan/tenant reference
	Status          SubscriptionStatus
	BillingType     BillingType
	Cycle           BillingCycle
	Value           float64
	Description     string
	NextDueDate     time.Time
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CancelledAt     *time.Time
	Metadata        map[string]string
}

type SubscriptionStatus string

const (
	SubscriptionStatusActive    SubscriptionStatus = "ACTIVE"
	SubscriptionStatusInactive  SubscriptionStatus = "INACTIVE"
	SubscriptionStatusExpired   SubscriptionStatus = "EXPIRED"
	SubscriptionStatusOverdue   SubscriptionStatus = "OVERDUE"
	SubscriptionStatusCancelled SubscriptionStatus = "CANCELLED"
	SubscriptionStatusPending   SubscriptionStatus = "PENDING"
)

type BillingType string

const (
	BillingTypeBoleto      BillingType = "BOLETO"
	BillingTypeCreditCard  BillingType = "CREDIT_CARD"
	BillingTypePIX         BillingType = "PIX"
	BillingTypeUndefined   BillingType = "UNDEFINED"
)

type BillingCycle string

const (
	BillingCycleWeekly       BillingCycle = "WEEKLY"
	BillingCycleBiweekly     BillingCycle = "BIWEEKLY"
	BillingCycleMonthly      BillingCycle = "MONTHLY"
	BillingCycleQuarterly    BillingCycle = "QUARTERLY"
	BillingCycleSemiannually BillingCycle = "SEMIANNUALLY"
	BillingCycleYearly       BillingCycle = "YEARLY"
)

// CreateSubscriptionInput input for creating a subscription
type CreateSubscriptionInput struct {
	CustomerID      string
	ExternalID      string
	BillingType     BillingType
	Cycle           BillingCycle
	Value           float64
	Description     string
	NextDueDate     time.Time
	CreditCardToken *string // Token from frontend
	Metadata        map[string]string
}

// UpdateSubscriptionInput input for updating a subscription
type UpdateSubscriptionInput struct {
	BillingType *BillingType
	Cycle       *BillingCycle
	Value       *float64
	Description *string
	NextDueDate *time.Time
	Metadata    map[string]string
}

// SubscriptionFilter for listing subscriptions
type SubscriptionFilter struct {
	CustomerID string
	Status     *SubscriptionStatus
	Limit      int
	Offset     int
}

// Payment represents a payment transaction
type Payment struct {
	ID              string
	SubscriptionID  *string
	CustomerID      string
	Value           float64
	Status          PaymentStatus
	BillingType     BillingType
	DueDate         time.Time
	PaidAt          *time.Time
	InvoiceURL      string
	BankSlipURL     string
	PIXQRCode       string
	PIXCode         string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Metadata        map[string]string
}

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "PENDING"
	PaymentStatusReceived  PaymentStatus = "RECEIVED"
	PaymentStatusOverdue   PaymentStatus = "OVERDUE"
	PaymentStatusRefunded  PaymentStatus = "REFUNDED"
	PaymentStatusConfirmed PaymentStatus = "CONFIRMED"
	PaymentStatusDeleted   PaymentStatus = "DELETED"
)

// PaymentFilter for listing payments
type PaymentFilter struct {
	CustomerID    string
	SubscriptionID *string
	Status        *PaymentStatus
	DateFrom      *time.Time
	DateTo        *time.Time
	Limit         int
	Offset        int
}

// RefundInput input for refunding a payment
type RefundInput struct {
	Value       *float64 // nil = full refund
	Description string
}

// CreatePaymentInput input for creating a standalone payment
type CreatePaymentInput struct {
	CustomerID       string
	Value            float64
	BillingType      BillingType
	Description      string
	DueDate          time.Time
	ExternalReference string
}

// Refund represents a refund operation
type Refund struct {
	ID          string
	PaymentID   string
	Value       float64
	Status      RefundStatus
	Description string
	CreatedAt   time.Time
}

type RefundStatus string

const (
	RefundStatusPending  RefundStatus = "PENDING"
	RefundStatusApproved RefundStatus = "APPROVED"
	RefundStatusRejected RefundStatus = "REJECTED"
	RefundStatusCompleted RefundStatus = "COMPLETED"
)

// WebhookEvent represents a webhook event from the payment gateway
type WebhookEvent struct {
	ID        string
	Gateway   string
	Type      WebhookEventType
	Timestamp time.Time
	Payload   map[string]any
	Raw       []byte
}

type WebhookEventType string

const (
	WebhookEventPaymentCreated    WebhookEventType = "PAYMENT_CREATED"
	WebhookEventPaymentReceived   WebhookEventType = "PAYMENT_RECEIVED"
	WebhookEventPaymentOverdue    WebhookEventType = "PAYMENT_OVERDUE"
	WebhookEventPaymentRefunded   WebhookEventType = "PAYMENT_REFUNDED"
	WebhookEventSubscriptionCreated WebhookEventType = "SUBSCRIPTION_CREATED"
	WebhookEventSubscriptionUpdated WebhookEventType = "SUBSCRIPTION_UPDATED"
	WebhookEventSubscriptionDeleted WebhookEventType = "SUBSCRIPTION_DELETED"
	WebhookEventCustomerCreated   WebhookEventType = "CUSTOMER_CREATED"
	WebhookEventCustomerUpdated   WebhookEventType = "CUSTOMER_UPDATED"
)

// MapToDomainTenantPlan maps our internal plan to payment provider plan
func MapToDomainTenantPlan(providerPlan string) domain.TenantPlan {
	switch providerPlan {
	case "starter":
		return domain.TenantPlanStarter
	case "pro":
		return domain.TenantPlanPro
	case "enterprise":
		return domain.TenantPlanEnterprise
	default:
		return domain.TenantPlanTrial
	}
}

// MapFromDomainTenantPlan maps our internal plan to payment provider plan code
func MapFromDomainTenantPlan(plan domain.TenantPlan) string {
	switch plan {
	case domain.TenantPlanStarter:
		return "starter"
	case domain.TenantPlanPro:
		return "pro"
	case domain.TenantPlanEnterprise:
		return "enterprise"
	default:
		return "trial"
	}
}

// PlanPrices defines the price for each plan in BRL
var PlanPrices = map[domain.TenantPlan]float64{
	domain.TenantPlanStarter:    99.00,
	domain.TenantPlanPro:        299.00,
	domain.TenantPlanEnterprise: 999.00,
}