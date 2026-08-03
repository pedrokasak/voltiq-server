package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/payment"
	"github.com/voltiq/server/internal/usecase"
)

// BillingHandler handles billing HTTP requests
type BillingHandler struct {
	billingUseCase *usecase.BillingUseCase
}

// NewBillingHandler creates a new BillingHandler
func NewBillingHandler(billingUseCase *usecase.BillingUseCase) *BillingHandler {
	return &BillingHandler{
		billingUseCase: billingUseCase,
	}
}

// GetBillingInfo handles GET /api/v1/billing
func (h *BillingHandler) GetBillingInfo(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == "" {
		request.WriteJSON(w, http.StatusUnauthorized,
			request.Fail("UNAUTHORIZED", "tenant not found", nil))
		return
	}

	info, err := h.billingUseCase.GetBillingInfo(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, usecase.ErrTenantNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "tenant not found", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("BILLING_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(info, ""))
}

// ActivatePlan handles POST /api/v1/billing/activate
func (h *BillingHandler) ActivatePlan(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == "" {
		request.WriteJSON(w, http.StatusUnauthorized,
			request.Fail("UNAUTHORIZED", "tenant not found", nil))
		return
	}

	type ActivateRequest struct {
		Plan           string `json:"plan"`             // starter, pro, enterprise
		BillingType    string `json:"billing_type"`     // BOLETO, CREDIT_CARD, PIX
		CreditCardToken string `json:"credit_card_token,omitempty"`
	}

	var req ActivateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.Plan == "" {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "plan is required", nil))
		return
	}

	plan := domain.TenantPlan(req.Plan)
	billingType := payment.BillingType(req.BillingType)
	if billingType == "" {
		billingType = payment.BillingTypePIX // Default to PIX
	}

	var creditCardToken *string
	if req.CreditCardToken != "" {
		creditCardToken = &req.CreditCardToken
	}

	info, err := h.billingUseCase.ActivatePlan(r.Context(), tenantID, plan, billingType, creditCardToken)
	if err != nil {
		if errors.Is(err, usecase.ErrTenantNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "tenant not found", nil))
			return
		}
		if errors.Is(err, usecase.ErrInvalidPlan) {
			request.WriteJSON(w, http.StatusBadRequest,
				request.Fail("VALIDATION_ERROR", "invalid plan", nil))
			return
		}
		if errors.Is(err, usecase.ErrSubscriptionAlreadyActive) {
			request.WriteJSON(w, http.StatusBadRequest,
				request.Fail("ALREADY_ACTIVE", "tenant already has active subscription", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("ACTIVATE_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(info, "plan activated successfully"))
}

// ChangePlan handles PATCH /api/v1/billing/plan
func (h *BillingHandler) ChangePlan(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == "" {
		request.WriteJSON(w, http.StatusUnauthorized,
			request.Fail("UNAUTHORIZED", "tenant not found", nil))
		return
	}

	type ChangePlanRequest struct {
		Plan string `json:"plan"`
	}

	var req ChangePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "invalid request body", nil))
		return
	}

	if req.Plan == "" {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("VALIDATION_ERROR", "plan is required", nil))
		return
	}

	plan := domain.TenantPlan(req.Plan)

	info, err := h.billingUseCase.ChangePlan(r.Context(), tenantID, plan)
	if err != nil {
		if errors.Is(err, usecase.ErrTenantNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "tenant not found", nil))
			return
		}
		if errors.Is(err, usecase.ErrInvalidPlan) {
			request.WriteJSON(w, http.StatusBadRequest,
				request.Fail("VALIDATION_ERROR", "invalid plan", nil))
			return
		}
		if errors.Is(err, usecase.ErrSubscriptionNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "no active subscription", nil))
			return
		}
		if errors.Is(err, usecase.ErrSeatLimitExceeded) {
			request.WriteJSON(w, http.StatusBadRequest,
				request.Fail("SEAT_LIMIT_EXCEEDED", "plan downgrade would exceed current seat count", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("CHANGE_PLAN_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(info, "plan changed successfully"))
}

// CancelSubscription handles POST /api/v1/billing/cancel
func (h *BillingHandler) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == "" {
		request.WriteJSON(w, http.StatusUnauthorized,
			request.Fail("UNAUTHORIZED", "tenant not found", nil))
		return
	}

	err := h.billingUseCase.CancelSubscription(r.Context(), tenantID)
	if err != nil {
		if errors.Is(err, usecase.ErrTenantNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "tenant not found", nil))
			return
		}
		if errors.Is(err, usecase.ErrSubscriptionNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "no active subscription", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("CANCEL_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(nil, "subscription cancelled successfully"))
}

// GetPaymentHistory handles GET /api/v1/billing/payments
func (h *BillingHandler) GetPaymentHistory(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == "" {
		request.WriteJSON(w, http.StatusUnauthorized,
			request.Fail("UNAUTHORIZED", "tenant not found", nil))
		return
	}

	limit := 50
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	payments, err := h.billingUseCase.GetPaymentHistory(r.Context(), tenantID, limit, offset)
	if err != nil {
		if errors.Is(err, usecase.ErrTenantNotFound) {
			request.WriteJSON(w, http.StatusNotFound,
				request.Fail("NOT_FOUND", "tenant not found", nil))
			return
		}
		request.WriteJSON(w, http.StatusInternalServerError,
			request.Fail("PAYMENTS_ERROR", err.Error(), nil))
		return
	}

	request.WriteJSON(w, http.StatusOK, request.Success(payments, ""))
}

// WebhookHandler handles payment provider webhooks
type WebhookHandler struct {
	billingUseCase *usecase.BillingUseCase
	paymentProvider payment.PaymentProvider
}

// NewWebhookHandler creates a new WebhookHandler
func NewWebhookHandler(billingUseCase *usecase.BillingUseCase, paymentProvider payment.PaymentProvider) *WebhookHandler {
	return &WebhookHandler{
		billingUseCase:  billingUseCase,
		paymentProvider: paymentProvider,
	}
}

// HandleAsaasWebhook handles POST /api/v1/webhooks/asaas
func (h *WebhookHandler) HandleAsaasWebhook(w http.ResponseWriter, r *http.Request) {
	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("INVALID_PAYLOAD", "could not read body", nil))
		return
	}

	// Verify signature
	signature := r.Header.Get("asaas-signature")
	if signature == "" {
		signature = r.Header.Get("X-Asaas-Signature")
	}

	if !h.paymentProvider.VerifyWebhookSignature(body, signature) {
		request.WriteJSON(w, http.StatusUnauthorized,
			request.Fail("INVALID_SIGNATURE", "webhook signature verification failed", nil))
		return
	}

	// Parse event
	event, err := h.paymentProvider.ParseWebhookEvent(body)
	if err != nil {
		request.WriteJSON(w, http.StatusBadRequest,
			request.Fail("INVALID_PAYLOAD", "could not parse webhook", nil))
		return
	}

	// Process event
	if err := h.billingUseCase.HandleWebhook(r.Context(), event); err != nil {
		// Log error but return 200 to avoid retries for processing errors
		log.Printf("Webhook processing error: %v", err)
	}

	request.WriteJSON(w, http.StatusOK, request.Success(map[string]string{"status": "ok"}, ""))
}