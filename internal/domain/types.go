package domain

import (
	"math"
	"time"
)

// UUID represents a unique identifier
type UUID string

// TenantPlan represents the subscription plan of a tenant
type TenantPlan string

const (
	TenantPlanTrial      TenantPlan = "trial"
	TenantPlanStarter    TenantPlan = "starter"
	TenantPlanPro        TenantPlan = "pro"
	TenantPlanEnterprise TenantPlan = "enterprise"
)

// UserRole represents the role of a user in the system
type UserRole string

const (
	UserRoleAdmin    UserRole = "ADMIN"
	UserRoleEngineer UserRole = "ENGINEER"
	UserRoleViewer   UserRole = "VIEWER"
)

// UCClass represents the class of a consuming unit
type UCClass string

const (
	UCClassResidential UCClass = "RESIDENTIAL"
	UCClassCommercial  UCClass = "COMMERCIAL"
	UCClassIndustrial  UCClass = "INDUSTRIAL"
	UCClassRural       UCClass = "RURAL"
	UCClassPublicPower UCClass = "PUBLIC_POWER"
)

// BalanceStatus represents the status of a transformer balance
type BalanceStatus string

const (
	BalanceStatusNormal   BalanceStatus = "NORMAL"
	BalanceStatusWarning  BalanceStatus = "WARNING"
	BalanceStatusCritical BalanceStatus = "CRITICAL"
)

// ImportStatus represents the status of a CSV import
type ImportStatus string

const (
	ImportStatusProcessing ImportStatus = "PROCESSING"
	ImportStatusCompleted  ImportStatus = "COMPLETED"
	ImportStatusError      ImportStatus = "ERROR"
)

// AlertType represents the type of alert
type AlertType string

const (
	AlertTypeWarning  AlertType = "WARNING"
	AlertTypeCritical AlertType = "CRITICAL"
)

// AlertChannel represents the channel for sending alerts
type AlertChannel string

const (
	AlertChannelEmail    AlertChannel = "EMAIL"
	AlertChannelWhatsapp AlertChannel = "WHATSAPP"
)

// AlertDeliveryStatus represents the delivery status of an alert
type AlertDeliveryStatus string

const (
	AlertDeliveryPending AlertDeliveryStatus = "PENDING"
	AlertDeliverySent    AlertDeliveryStatus = "SENT"
	AlertDeliveryError   AlertDeliveryStatus = "ERROR"
)

// BalanceCalculationStatus represents the status of balance calculation per PRODIST M7
type BalanceCalculationStatus string

const (
	BalanceCalculationStatusNormal   BalanceCalculationStatus = "NORMAL"
	BalanceCalculationStatusWarning  BalanceCalculationStatus = "WARNING"
	BalanceCalculationStatusCritical BalanceCalculationStatus = "CRITICAL"
)

// CalculationInput groups data for the calculation engine to process a transformer in a period
type CalculationInput struct {
	Transformer    Transformer
	EnergyInjected float64   // kWh measured at primary
	UCConsumptions []float64 // kWh of each linked UC
	PeriodHours    float64   // period duration in hours
	AverageCurrent float64   // Ic in A (optional - uses 50% if absent)
}

// CalculationResult contains all values calculated by the PRODIST M7 engine
type CalculationResult struct {
	TransformerID UUID
	TenantID      UUID

	EnergyInjectedKWh   float64
	TotalConsumptionKWh float64
	LossKWh             float64
	LossPct             float64

	TechnicalLossTrafoKWh float64 // PT_trafo = P0×T + Pcc×(Ic/In)²×T
	NonTechnicalLossKWh   float64 // PNT = perda_total - perda_tecnica

	Status   BalanceCalculationStatus
	LimitPct float64
	UCCount  int

	CalculatedAt time.Time
}

// Tenant represents a company/cooperative client of the SaaS
type Tenant struct {
	ID         UUID       `json:"id"`
	Name       string     `json:"name"`
	Document   string     `json:"document"`
	Plan       TenantPlan `json:"plan"`
	TrialUntil time.Time  `json:"trial_until"`
	Active     bool       `json:"active"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// User represents system users
type User struct {
	ID           UUID       `json:"id"`
	TenantID     UUID       `json:"tenant_id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	PasswordHash string     `json:"-"`
	Role         UserRole   `json:"role"`
	Active       bool       `json:"active"`
	LastLogin    *time.Time `json:"last_login"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at"`
}

// Substation represents an electrical substation
type Substation struct {
	ID        UUID       `json:"id"`
	TenantID  UUID       `json:"tenant_id"`
	Code      string     `json:"code"`
	Name      string     `json:"name"`
	Lat       *float64   `json:"lat"`
	Lng       *float64   `json:"lng"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

// Transformer represents a distribution transformer
// P0 and Pcc are nameplate data required for PRODIST M7 calculation
type Transformer struct {
	ID                UUID       `json:"id"`
	TenantID          UUID       `json:"tenant_id"`
	SubstationID      *UUID      `json:"substation_id"`
	Code              string     `json:"code"`
	PowerKVA          float64    `json:"power_kva"`
	PrimaryVoltageKV  float64    `json:"primary_voltage_kv"`
	SecondaryVoltageV float64    `json:"secondary_voltage_v"`
	Lat               *float64   `json:"lat"`
	Lng               *float64   `json:"lng"`
	CoreLossKW        *float64   `json:"core_loss_kw"`    // P0: no-load losses (core), kW
	WindingLossKW     *float64   `json:"winding_loss_kw"` // Pcc: load losses (winding), kW
	LossLimitPct      *float64   `json:"loss_limit_pct"`  // ANEEL regulatory limit (%)
	Active            bool       `json:"active"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `json:"deleted_at"`
}

// NominalCurrent calculates the nominal current of the transformer (A) - three-phase
// In = (kVA × 1000) / (√3 × V)
func (t Transformer) NominalCurrent() float64 {
	if t.SecondaryVoltageV == 0 {
		return 0
	}
	return (t.PowerKVA * 1000) / (math.Sqrt(3) * t.SecondaryVoltageV)
}

// ConsumingUnit represents a consumer unit linked to a transformer
type ConsumingUnit struct {
	ID            UUID       `json:"id"`
	TenantID      UUID       `json:"tenant_id"`
	TransformerID UUID       `json:"transformer_id"`
	UCCode        string     `json:"uc_code"`
	Name          string     `json:"name"`
	Class         *UCClass   `json:"class"`
	Active        bool       `json:"active"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeletedAt     *time.Time `json:"deleted_at"`
}

// TransformerReading represents energy injected at transformer (primary meter)
type TransformerReading struct {
	ID            UUID      `json:"id"`
	TenantID      UUID      `json:"tenant_id"`
	TransformerID UUID      `json:"transformer_id"`
	ReadingAt     time.Time `json:"reading_at"`
	EnergyKWh     float64   `json:"energy_kwh"`
	DemandKW      *float64  `json:"demand_kw"`
	PowerFactor   *float64  `json:"power_factor"`
	ImportID      *UUID     `json:"import_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// UCReading represents consumption reading per UC
type UCReading struct {
	ID             UUID      `json:"id"`
	TenantID       UUID      `json:"tenant_id"`
	UCID           UUID      `json:"uc_id"`
	TransformerID  UUID      `json:"transformer_id"`
	ReadingAt      time.Time `json:"reading_at"`
	ConsumptionKWh float64   `json:"consumption_kwh"`
	ImportID       *UUID     `json:"import_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// TransformerBalance represents the balance result per transformer per period
type TransformerBalance struct {
	ID                  UUID          `json:"id"`
	TenantID            UUID          `json:"tenant_id"`
	TransformerID       UUID          `json:"transformer_id"`
	PeriodStart         time.Time     `json:"period_start"`
	PeriodEnd           time.Time     `json:"period_end"`
	EnergyInjectedKWh   float64       `json:"energy_injected_kwh"`
	TotalConsumptionKWh float64       `json:"total_consumption_kwh"`
	LossKWh             float64       `json:"loss_kwh"`
	LossPct             float64       `json:"loss_pct"`
	TechnicalLossKWh    *float64      `json:"technical_loss_kwh"`
	NonTechnicalLossKWh *float64      `json:"non_technical_loss_kwh"`
	Status              BalanceStatus `json:"status"`
	UCCount             int           `json:"uc_count"`
	CalculatedAt        time.Time     `json:"calculated_at"`
}

// Import represents a CSV import history
type Import struct {
	ID          UUID           `json:"id"`
	TenantID    UUID           `json:"tenant_id"`
	UserID      *UUID          `json:"user_id"`
	FileName    string         `json:"file_name"`
	TotalRows   *int           `json:"total_rows"`
	RowsOK      *int           `json:"rows_ok"`
	RowsError   *int           `json:"rows_error"`
	Status      ImportStatus   `json:"status"`
	ErrorsJSON  map[string]any `json:"errors_json"`
	CreatedAt   time.Time      `json:"created_at"`
	CompletedAt *time.Time     `json:"completed_at"`
}

// Alert represents a triggered alert
type Alert struct {
	ID             UUID                `json:"id"`
	TenantID       UUID                `json:"tenant_id"`
	TransformerID  UUID                `json:"transformer_id"`
	BalanceID      UUID                `json:"balance_id"`
	Type           AlertType           `json:"type"`
	Channel        AlertChannel        `json:"channel"`
	Recipient      string              `json:"recipient"`
	DeliveryStatus AlertDeliveryStatus `json:"delivery_status"`
	SentAt         *time.Time          `json:"sent_at"`
	CreatedAt      time.Time           `json:"created_at"`
}

// InviteStatus represents the status of a user invite
type InviteStatus string

const (
	InviteStatusPending   InviteStatus = "PENDING"
	InviteStatusAccepted  InviteStatus = "ACCEPTED"
	InviteStatusCancelled InviteStatus = "CANCELLED"
)

// Invite represents a user invitation
type Invite struct {
	ID          UUID         `json:"id"`
	TenantID    UUID         `json:"tenant_id"`
	Email       string       `json:"email"`
	Role        UserRole     `json:"role"`
	Token       string       `json:"token"`
	Status      InviteStatus `json:"status"`
	InvitedBy   UUID         `json:"invited_by"`
	AcceptedAt  *time.Time   `json:"accepted_at"`
	ExpiresAt   time.Time    `json:"expires_at"`
	CreatedAt   time.Time    `json:"created_at"`
	DeletedAt   *time.Time   `json:"deleted_at"`
}
