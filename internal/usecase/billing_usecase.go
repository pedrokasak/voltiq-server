package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/email"
	"github.com/voltiq/server/internal/payment"
	"github.com/voltiq/server/internal/repository"
)

// BillingUseCase handles billing operations
type BillingUseCase struct {
	tenantRepo       *repository.TenantRepository
	userRepo         *repository.UserRepository
	paymentProvider  payment.PaymentProvider
	webhookRepo      *repository.WebhookEventRepository
	dunningRepo      *repository.DunningRepository
	metricsRepo      *repository.MetricsRepository
	prorationRepo    *repository.ProrationRepository
	emailProvider    email.EmailProvider
	emailTemplates   *email.TemplateLoader
	planConfigs      map[domain.TenantPlan]payment.PlanConfig
}

// NewBillingUseCase creates a new BillingUseCase
func NewBillingUseCase(
	tenantRepo *repository.TenantRepository,
	userRepo *repository.UserRepository,
	paymentProvider payment.PaymentProvider,
	webhookRepo *repository.WebhookEventRepository,
	dunningRepo *repository.DunningRepository,
	metricsRepo *repository.MetricsRepository,
	prorationRepo *repository.ProrationRepository,
	emailProvider email.EmailProvider,
	emailTemplates *email.TemplateLoader,
) *BillingUseCase {
	return &BillingUseCase{
		tenantRepo:      tenantRepo,
		userRepo:        userRepo,
		paymentProvider: paymentProvider,
		webhookRepo:     webhookRepo,
		dunningRepo:     dunningRepo,
		metricsRepo:     metricsRepo,
		prorationRepo:   prorationRepo,
		emailProvider:   emailProvider,
		emailTemplates:  emailTemplates,
		planConfigs:     payment.DefaultPlanConfigs(),
	}
}

// GetBillingInfo returns billing information for a tenant
func (uc *BillingUseCase) GetBillingInfo(ctx context.Context, tenantID domain.UUID) (*BillingInfo, error) {
	tenant, err := uc.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, ErrTenantNotFound
	}

	// Get customer from payment provider if exists
	var customer *payment.Customer
	if tenant.PaymentCustomerID != nil && *tenant.PaymentCustomerID != "" {
		customer, err = uc.paymentProvider.GetCustomer(ctx, *tenant.PaymentCustomerID)
		if err != nil && !payment.IsNotFoundError(err) {
			return nil, err
		}
	}

	// Get subscription if exists
	var subscription *payment.Subscription
	if tenant.PaymentSubscriptionID != nil && *tenant.PaymentSubscriptionID != "" {
		subscription, err = uc.paymentProvider.GetSubscription(ctx, *tenant.PaymentSubscriptionID)
		if err != nil && !payment.IsNotFoundError(err) {
			return nil, err
		}
	}

	return &BillingInfo{
		Tenant:       tenant,
		Customer:     customer,
		Subscription: subscription,
	}, nil
}

// BillingInfo holds billing information for a tenant
type BillingInfo struct {
	Tenant       *domain.Tenant
	Customer     *payment.Customer
	Subscription *payment.Subscription
}

// ActivatePlan activates a plan for a tenant (creates customer + subscription)
func (uc *BillingUseCase) ActivatePlan(ctx context.Context, tenantID domain.UUID, plan domain.TenantPlan, billingType payment.BillingType, creditCardToken *string) (*BillingInfo, error) {
	tenant, err := uc.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, ErrTenantNotFound
	}

	planConfig, ok := uc.planConfigs[plan]
	if !ok {
		return nil, ErrInvalidPlan
	}

	// Get tenant owner for customer info
	owner, err := uc.userRepo.GetByTenantAndRole(ctx, tenantID, domain.UserRoleOwner)
	if err != nil {
		return nil, err
	}
	if owner == nil {
		// Fallback to first user if no owner found
		users, err := uc.userRepo.GetByTenant(ctx, tenantID)
		if err != nil || len(users) == 0 {
			return nil, errors.New("no users found for tenant")
		}
		owner = users[0]
	}

	// Create or get customer
	var customer *payment.Customer
	if tenant.PaymentCustomerID != nil && *tenant.PaymentCustomerID != "" {
		customer, err = uc.paymentProvider.GetCustomer(ctx, *tenant.PaymentCustomerID)
		if err != nil && !payment.IsNotFoundError(err) {
			return nil, err
		}
	}

	if customer == nil {
		// Create new customer
		customerInput := payment.CreateCustomerInput{
			ExternalID:     string(tenantID),
			Name:           owner.Name,
			Email:          owner.Email,
			Document:       tenant.Document,
			Phone:          "", // User doesn't have phone field yet
			Address:        tenant.Address,
			AddressNumber:  tenant.AddressNumber,
			Province:       tenant.Province,
			PostalCode:     tenant.PostalCode,
			Metadata: map[string]string{
				"tenant_id": string(tenantID),
			},
		}

		customer, err = uc.paymentProvider.CreateCustomer(ctx, customerInput)
		if err != nil {
			return nil, err
		}

		// Save customer ID to tenant
		tenant.PaymentCustomerID = &customer.ID
		tenant.UpdatedAt = time.Now()
		if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
			return nil, err
		}
	}

	// Check if already has active subscription
	if tenant.PaymentSubscriptionID != nil && *tenant.PaymentSubscriptionID != "" {
		existingSub, err := uc.paymentProvider.GetSubscription(ctx, *tenant.PaymentSubscriptionID)
		if err != nil && !payment.IsNotFoundError(err) {
			return nil, err
		}
		if existingSub != nil && existingSub.Status == payment.SubscriptionStatusActive {
			return nil, ErrSubscriptionAlreadyActive
		}
	}

	// Create subscription
	nextDueDate := time.Now().AddDate(0, 1, 0) // Next month
	subInput := payment.CreateSubscriptionInput{
		CustomerID:      customer.ID,
		ExternalID:      planConfig.ProviderPlanID,
		BillingType:     billingType,
		Cycle:           planConfig.BillingCycle,
		Value:           planConfig.Value,
		Description:     planConfig.Description,
		NextDueDate:     nextDueDate,
		CreditCardToken: creditCardToken,
		Metadata: map[string]string{
			"tenant_id": string(tenantID),
			"plan":      string(plan),
		},
	}

	subscription, err := uc.paymentProvider.CreateSubscription(ctx, subInput)
	if err != nil {
		return nil, err
	}

	// Update tenant with subscription info
	tenant.PaymentSubscriptionID = &subscription.ID
	tenant.Plan = plan
	tenant.Status = domain.TenantStatusActive
	tenant.MaxUsers = planConfig.MaxUsers
	tenant.ActivatedAt = &subscription.CreatedAt
	tenant.UpdatedAt = time.Now()

	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}

	return &BillingInfo{
		Tenant:       tenant,
		Customer:     customer,
		Subscription: subscription,
	}, nil
}

// ChangePlan changes the tenant's plan with proration support
func (uc *BillingUseCase) ChangePlan(ctx context.Context, tenantID domain.UUID, newPlan domain.TenantPlan) (*BillingInfo, error) {
	tenant, err := uc.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, ErrTenantNotFound
	}

	if tenant.PaymentSubscriptionID == nil || *tenant.PaymentSubscriptionID == "" {
		return nil, ErrSubscriptionNotFound
	}

	planConfig, ok := uc.planConfigs[newPlan]
	if !ok {
		return nil, ErrInvalidPlan
	}

	// Check seat limit
	if tenant.SeatCount > planConfig.MaxUsers {
		return nil, ErrSeatLimitExceeded
	}

	// Get current subscription for proration calculation
	currentSub, err := uc.paymentProvider.GetSubscription(ctx, *tenant.PaymentSubscriptionID)
	if err != nil {
		return nil, err
	}
	if currentSub == nil {
		return nil, ErrSubscriptionNotFound
	}

	// Calculate proration
	oldPlanPriceCents := int64(currentSub.Value * 100)
	newPlanPriceCents := int64(planConfig.Value * 100)

	// Use subscription period for calculation
	periodStart := currentSub.CurrentPeriodStart
	periodEnd := currentSub.CurrentPeriodEnd
	if periodStart.IsZero() || periodEnd.IsZero() {
		// Fallback to monthly cycle
		periodStart = time.Now().Truncate(24 * time.Hour)
		periodEnd = periodStart.AddDate(0, 1, 0)
	}

	amountCents, daysCalculated, dailyRateOld, dailyRateNew := repository.CalculateProration(
		oldPlanPriceCents,
		newPlanPriceCents,
		periodStart,
		periodEnd,
		time.Now(),
	)

	// Determine reason and handle payment/credit
	reason := "upgrade"
	if amountCents > 0 {
		reason = "downgrade" // positive = credit to tenant
	} else if amountCents < 0 {
		reason = "upgrade" // negative = charge to tenant
	}

	// Create proration record
	proration := &repository.ProrationCredit{
		TenantID:              string(tenantID),
		SubscriptionGatewayID: currentSub.ID,
		AmountCents:           amountCents,
		Reason:                reason,
		PeriodStart:           periodStart,
		PeriodEnd:             periodEnd,
		DaysCalculated:        daysCalculated,
		DailyRateOldCents:     dailyRateOld,
		DailyRateNewCents:     dailyRateNew,
	}

	if err := uc.prorationRepo.Create(ctx, proration); err != nil {
		return nil, err
	}

	// Handle immediate payment for upgrade (TODO: implement when Asaas CreatePayment is ready)
	// if reason == "upgrade" && amountCents < 0 {
	// 	_ = uc.paymentProvider.CreatePayment(ctx, payment.CreatePaymentInput{
	// 		CustomerID:       currentSub.CustomerID,
	// 		Value:            float64(-amountCents) / 100,
	// 		BillingType:      currentSub.BillingType,
	// 		Description:      fmt.Sprintf("Proration charge for upgrade from %s to %s", currentSub.Description, planConfig.Description),
	// 		DueDate:          time.Now(),
	// 		ExternalReference: proration.ID,
	// 	})
	// }

	// Update subscription in payment provider
	subInput := payment.UpdateSubscriptionInput{
		BillingType: nil, // Keep current
		Cycle:       &planConfig.BillingCycle,
		Value:       &planConfig.Value,
		Description: &planConfig.Description,
	}

	subscription, err := uc.paymentProvider.UpdateSubscription(ctx, *tenant.PaymentSubscriptionID, subInput)
	if err != nil {
		return nil, err
	}

	// Update tenant
	tenant.Plan = newPlan
	tenant.MaxUsers = planConfig.MaxUsers
	tenant.UpdatedAt = time.Now()

	if err := uc.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}

	return &BillingInfo{
		Tenant:       tenant,
		Subscription: subscription,
	}, nil
}

// CancelSubscription cancels the tenant's subscription
func (uc *BillingUseCase) CancelSubscription(ctx context.Context, tenantID domain.UUID) error {
	tenant, err := uc.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return err
	}
	if tenant == nil {
		return ErrTenantNotFound
	}

	if tenant.PaymentSubscriptionID == nil || *tenant.PaymentSubscriptionID == "" {
		return ErrSubscriptionNotFound
	}

	if err := uc.paymentProvider.CancelSubscription(ctx, *tenant.PaymentSubscriptionID); err != nil {
		return err
	}

	// Update tenant status
	tenant.Status = domain.TenantStatusCancelled
	tenant.CancelledAt = newTimePtr(time.Now())
	tenant.UpdatedAt = time.Now()

	return uc.tenantRepo.Update(ctx, tenant)
}

// GetPaymentHistory returns payment history for a tenant
func (uc *BillingUseCase) GetPaymentHistory(ctx context.Context, tenantID domain.UUID, limit, offset int) ([]*payment.Payment, error) {
	tenant, err := uc.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, ErrTenantNotFound
	}

	if tenant.PaymentCustomerID == nil || *tenant.PaymentCustomerID == "" {
		return []*payment.Payment{}, nil
	}

	filter := payment.PaymentFilter{
		CustomerID: *tenant.PaymentCustomerID,
		Limit:      limit,
		Offset:     offset,
	}

	return uc.paymentProvider.ListPayments(ctx, filter)
}

// GetProrationHistory returns proration credits for a tenant
func (uc *BillingUseCase) GetProrationHistory(ctx context.Context, tenantID domain.UUID) ([]*repository.ProrationCredit, error) {
	tenant, err := uc.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if tenant == nil {
		return nil, ErrTenantNotFound
	}

	if tenant.PaymentSubscriptionID == nil || *tenant.PaymentSubscriptionID == "" {
		return []*repository.ProrationCredit{}, nil
	}

	return uc.prorationRepo.GetTenantCredits(ctx, string(tenantID))
}

// HandleWebhook processes a webhook event from the payment provider with idempotency
func (uc *BillingUseCase) HandleWebhook(ctx context.Context, event *payment.WebhookEvent) error {
	// 1. Check idempotency
	exists, err := uc.webhookRepo.Exists(ctx, "asaas", event.ID)
	if err != nil {
		return err
	}
	if exists {
		// Already processed, return success to avoid retry
		return nil
	}

	// 2. Save event as pending
	if err := uc.webhookRepo.CreatePending(ctx, event); err != nil {
		return err
	}

	// 3. Process event
	processErr := uc.processWebhookEvent(ctx, event)

	// 4. Update status
	if processErr != nil {
		uc.webhookRepo.MarkFailed(ctx, "asaas", event.ID, processErr.Error())
		return processErr // Return error for Asaas retry
	}

	uc.webhookRepo.MarkProcessed(ctx, "asaas", event.ID)
	return nil
}

func (uc *BillingUseCase) processWebhookEvent(ctx context.Context, event *payment.WebhookEvent) error {
	switch event.Type {
	case payment.WebhookEventPaymentReceived:
		return uc.handlePaymentReceived(ctx, event)
	case payment.WebhookEventPaymentOverdue:
		return uc.handlePaymentOverdue(ctx, event)
	case payment.WebhookEventSubscriptionDeleted:
		return uc.handleSubscriptionDeleted(ctx, event)
	case payment.WebhookEventSubscriptionUpdated:
		return uc.handleSubscriptionUpdated(ctx, event)
	}
	return nil
}

func (uc *BillingUseCase) handlePaymentReceived(ctx context.Context, event *payment.WebhookEvent) error {
	// Extract subscription ID from event
	subID, ok := event.Payload["subscription"].(string)
	if !ok {
		return nil
	}

	// Find tenant by subscription ID
	tenant, err := uc.tenantRepo.GetByPaymentSubscriptionID(ctx, subID)
	if err != nil {
		return err
	}
	if tenant == nil {
		return nil // Not our tenant
	}

	// Reactivate if suspended
	if tenant.Status == domain.TenantStatusSuspended || tenant.Status == domain.TenantStatusPendingPayment {
		tenant.Status = domain.TenantStatusActive
		tenant.UpdatedAt = time.Now()
		return uc.tenantRepo.Update(ctx, tenant)
	}

	return nil
}

func (uc *BillingUseCase) handlePaymentOverdue(ctx context.Context, event *payment.WebhookEvent) error {
	subID, ok := event.Payload["subscription"].(string)
	if !ok {
		return nil
	}

	tenant, err := uc.tenantRepo.GetByPaymentSubscriptionID(ctx, subID)
	if err != nil {
		return err
	}
	if tenant == nil {
		return nil
	}

	tenant.Status = domain.TenantStatusPendingPayment
	tenant.UpdatedAt = time.Now()

	// Create dunning event for stage 1 (D+1)
	if err := uc.dunningRepo.Create(ctx, string(tenant.ID), subID, 1); err != nil {
		return err
	}

	return uc.tenantRepo.Update(ctx, tenant)
}

func (uc *BillingUseCase) handleSubscriptionDeleted(ctx context.Context, event *payment.WebhookEvent) error {
	subID, ok := event.Payload["subscription"].(string)
	if !ok {
		return nil
	}

	tenant, err := uc.tenantRepo.GetByPaymentSubscriptionID(ctx, subID)
	if err != nil {
		return err
	}
	if tenant == nil {
		return nil
	}

	tenant.Status = domain.TenantStatusCancelled
	tenant.CancelledAt = newTimePtr(time.Now())
	tenant.PaymentSubscriptionID = nil
	tenant.UpdatedAt = time.Now()
	return uc.tenantRepo.Update(ctx, tenant)
}

func (uc *BillingUseCase) handleSubscriptionUpdated(ctx context.Context, event *payment.WebhookEvent) error {
	subID, ok := event.Payload["subscription"].(string)
	if !ok {
		return nil
	}

	tenant, err := uc.tenantRepo.GetByPaymentSubscriptionID(ctx, subID)
	if err != nil {
		return err
	}
	if tenant == nil {
		return nil
	}

	// Sync status
	if status, ok := event.Payload["status"].(string); ok {
		switch payment.SubscriptionStatus(status) {
		case payment.SubscriptionStatusActive:
			tenant.Status = domain.TenantStatusActive
		case payment.SubscriptionStatusOverdue:
			tenant.Status = domain.TenantStatusPendingPayment
		case payment.SubscriptionStatusCancelled, payment.SubscriptionStatusExpired:
			tenant.Status = domain.TenantStatusCancelled
		}
		tenant.UpdatedAt = time.Now()
		return uc.tenantRepo.Update(ctx, tenant)
	}

	return nil
}

// RunDunningJob executes the daily dunning process
func (uc *BillingUseCase) RunDunningJob(ctx context.Context) error {
	now := time.Now()

	// Stage 1: D+1 (first notice)
	if err := uc.processDunningStage(ctx, 1, now.AddDate(0, 0, -1)); err != nil {
		return err
	}

	// Stage 2: D+7 (second notice)
	if err := uc.processDunningStage(ctx, 2, now.AddDate(0, 0, -7)); err != nil {
		return err
	}

	// Stage 3: D+15 (final notice)
	if err := uc.processDunningStage(ctx, 3, now.AddDate(0, 0, -15)); err != nil {
		return err
	}

	return nil
}

func (uc *BillingUseCase) processDunningStage(ctx context.Context, stage int, cutoffTime time.Time) error {
	pending, err := uc.dunningRepo.GetPendingDunning(ctx, stage, cutoffTime)
	if err != nil {
		return err
	}

	for _, p := range pending {
		// Send dunning email
		if err := uc.sendDunningEmail(ctx, p, stage); err != nil {
			// Log error but don't fail the entire job
			fmt.Printf("Failed to send dunning email for tenant %s stage %d: %v\n", p.TenantID, stage, err)
		} else {
			// Mark email as sent
			if err := uc.dunningRepo.MarkEmailSent(ctx, p.TenantID, p.PaymentGatewayID, stage, fmt.Sprintf("dunning_stage_%d", stage)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (uc *BillingUseCase) sendDunningEmail(ctx context.Context, pending *repository.DunningPending, stage int) error {
	if uc.emailProvider == nil || uc.emailTemplates == nil {
		return fmt.Errorf("email provider or templates not configured")
	}

	if pending.TenantEmail == nil || *pending.TenantEmail == "" {
		return fmt.Errorf("tenant email not available")
	}

	if pending.TenantName == nil {
		return fmt.Errorf("tenant name not available")
	}

	// Get tenant for plan info
	tenant, err := uc.tenantRepo.GetByID(ctx, domain.UUID(pending.TenantID))
	if err != nil {
		return err
	}
	if tenant == nil {
		return fmt.Errorf("tenant not found")
	}

	planConfig, ok := uc.planConfigs[tenant.Plan]
	if !ok {
		planConfig = payment.PlanConfig{
			ProviderPlanID: string(tenant.Plan),
			Value:          99.00,
			Description:    string(tenant.Plan),
		}
	}

	// Build template data
	data := email.BuildDunningTemplateData(email.DunningEmailData{
		TenantName:   *pending.TenantName,
		PlanName:     planConfig.Description,
		Amount:       fmt.Sprintf("R$ %.2f", planConfig.Value),
		DueDate:      time.Now().AddDate(0, 0, -int(stage)*7).Format("02/01/2006"), // Approximate
		DaysOverdue:  stage * 7, // Approximate
		BillingURL:   "https://app.voltiq.com.br/billing",
		SupportEmail: "suporte@voltiq.com.br",
		CompanyName:  "Voltiq Software",
		Stage:        stage,
	})

	// Render template
	templateName := email.GetDunningTemplateName(stage)
	htmlBody, err := uc.emailTemplates.Render(templateName, data)
	if err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	subject := email.GetDunningSubject(stage, planConfig.Description)

	// Send email
	return uc.emailProvider.SendEmail(ctx, email.SendEmailInput{
		To:       []string{*pending.TenantEmail},
		Subject:  subject,
		HTMLBody: htmlBody,
		TextBody: fmt.Sprintf("%s\n\nAcesse: https://app.voltiq.com.br/billing para regularizar.", subject),
	})
}

// RunMetricsJob computes and stores daily billing metrics
func (uc *BillingUseCase) RunMetricsJob(ctx context.Context, date time.Time) error {
	metrics, err := uc.metricsRepo.ComputeDailyMetrics(ctx, date)
	if err != nil {
		return err
	}

	return uc.metricsRepo.UpsertDailyMetrics(ctx, metrics)
}

// GetBillingMetrics returns billing metrics for a date range
func (uc *BillingUseCase) GetBillingMetrics(ctx context.Context, from, to time.Time) ([]*repository.DailyBillingMetrics, error) {
	return uc.metricsRepo.GetMetricsRange(ctx, from, to)
}

// GetLatestBillingMetrics returns the most recent billing metrics
func (uc *BillingUseCase) GetLatestBillingMetrics(ctx context.Context) (*repository.DailyBillingMetrics, error) {
	return uc.metricsRepo.GetLatestMetrics(ctx)
}

// GetDunningHistory returns dunning history for a tenant
func (uc *BillingUseCase) GetDunningHistory(ctx context.Context, tenantID domain.UUID) ([]*repository.DunningEvent, error) {
	return uc.dunningRepo.GetDunningHistory(ctx, string(tenantID))
}

// Helper
func newTimePtr(t time.Time) *time.Time {
	return &t
}