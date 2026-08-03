package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AsaasProvider implements PaymentProvider for Asaas gateway
type AsaasProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	webhookKey string
}

// NewAsaasProvider creates a new Asaas provider
func NewAsaasProvider(apiKey, webhookKey string, sandbox bool) *AsaasProvider {
	baseURL := "https://api.asaas.com/api/v3"
	if sandbox {
		baseURL = "https://sandbox.asaas.com/api/v3"
	}

	return &AsaasProvider{
		apiKey:     apiKey,
		baseURL:    baseURL,
		webhookKey: webhookKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// --- Customer operations ---

func (p *AsaasProvider) CreateCustomer(ctx context.Context, input CreateCustomerInput) (*Customer, error) {
	body := map[string]any{
		"name":            input.Name,
		"email":           input.Email,
		"cpfCnpj":         input.Document,
		"phone":           input.Phone,
		"mobilePhone":     input.Phone,
		"address":         input.Address,
		"addressNumber":   input.AddressNumber,
		"province":        input.Province,
		"postalCode":      input.PostalCode,
		"externalReference": input.ExternalID,
	}

	if input.Metadata != nil {
		for k, v := range input.Metadata {
			body[k] = v
		}
	}

	resp, err := p.doRequest(ctx, "POST", "/customers", body)
	if err != nil {
		return nil, err
	}

	return p.parseCustomer(resp), nil
}

func (p *AsaasProvider) GetCustomer(ctx context.Context, customerID string) (*Customer, error) {
	resp, err := p.doRequest(ctx, "GET", "/customers/"+customerID, nil)
	if err != nil {
		return nil, err
	}
	return p.parseCustomer(resp), nil
}

func (p *AsaasProvider) UpdateCustomer(ctx context.Context, customerID string, input UpdateCustomerInput) (*Customer, error) {
	body := make(map[string]any)
	if input.Name != nil {
		body["name"] = *input.Name
	}
	if input.Email != nil {
		body["email"] = *input.Email
	}
	if input.Phone != nil {
		body["phone"] = *input.Phone
		body["mobilePhone"] = *input.Phone
	}
	if input.Address != nil {
		body["address"] = *input.Address
	}
	if input.AddressNumber != nil {
		body["addressNumber"] = *input.AddressNumber
	}
	if input.Province != nil {
		body["province"] = *input.Province
	}
	if input.PostalCode != nil {
		body["postalCode"] = *input.PostalCode
	}
	if input.Metadata != nil {
		for k, v := range input.Metadata {
			body[k] = v
		}
	}

	resp, err := p.doRequest(ctx, "PUT", "/customers/"+customerID, body)
	if err != nil {
		return nil, err
	}
	return p.parseCustomer(resp), nil
}

// --- Subscription operations ---

func (p *AsaasProvider) CreateSubscription(ctx context.Context, input CreateSubscriptionInput) (*Subscription, error) {
	body := map[string]any{
		"customer":      input.CustomerID,
		"billingType":   string(input.BillingType),
		"cycle":         string(input.Cycle),
		"value":         input.Value,
		"description":   input.Description,
		"nextDueDate":   input.NextDueDate.Format("2006-01-02"),
		"externalReference": input.ExternalID,
	}

	if input.CreditCardToken != nil {
		body["creditCardToken"] = *input.CreditCardToken
	}

	if input.Metadata != nil {
		for k, v := range input.Metadata {
			body[k] = v
		}
	}

	resp, err := p.doRequest(ctx, "POST", "/subscriptions", body)
	if err != nil {
		return nil, err
	}

	return p.parseSubscription(resp), nil
}

func (p *AsaasProvider) GetSubscription(ctx context.Context, subscriptionID string) (*Subscription, error) {
	resp, err := p.doRequest(ctx, "GET", "/subscriptions/"+subscriptionID, nil)
	if err != nil {
		return nil, err
	}
	return p.parseSubscription(resp), nil
}

func (p *AsaasProvider) UpdateSubscription(ctx context.Context, subscriptionID string, input UpdateSubscriptionInput) (*Subscription, error) {
	body := make(map[string]any)
	if input.BillingType != nil {
		body["billingType"] = string(*input.BillingType)
	}
	if input.Cycle != nil {
		body["cycle"] = string(*input.Cycle)
	}
	if input.Value != nil {
		body["value"] = *input.Value
	}
	if input.Description != nil {
		body["description"] = *input.Description
	}
	if input.NextDueDate != nil {
		body["nextDueDate"] = input.NextDueDate.Format("2006-01-02")
	}
	if input.Metadata != nil {
		for k, v := range input.Metadata {
			body[k] = v
		}
	}

	resp, err := p.doRequest(ctx, "PUT", "/subscriptions/"+subscriptionID, body)
	if err != nil {
		return nil, err
	}
	return p.parseSubscription(resp), nil
}

func (p *AsaasProvider) CancelSubscription(ctx context.Context, subscriptionID string) error {
	_, err := p.doRequest(ctx, "DELETE", "/subscriptions/"+subscriptionID, nil)
	return err
}

func (p *AsaasProvider) ListSubscriptions(ctx context.Context, filter SubscriptionFilter) ([]*Subscription, error) {
	params := url.Values{}
	if filter.CustomerID != "" {
		params.Set("customer", filter.CustomerID)
	}
	if filter.Status != nil {
		params.Set("status", string(*filter.Status))
	}
	if filter.Limit > 0 {
		params.Set("limit", strconv.Itoa(filter.Limit))
	}
	if filter.Offset > 0 {
		params.Set("offset", strconv.Itoa(filter.Offset))
	}

	endpoint := "/subscriptions"
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	resp, err := p.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Response is { "data": [...], "totalCount": N }
	var result struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}

	subs := make([]*Subscription, 0, len(result.Data))
	for _, item := range result.Data {
		subs = append(subs, p.parseSubscription(item))
	}
	return subs, nil
}

// --- Payment operations ---

func (p *AsaasProvider) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	resp, err := p.doRequest(ctx, "GET", "/payments/"+paymentID, nil)
	if err != nil {
		return nil, err
	}
	return p.parsePayment(resp), nil
}

func (p *AsaasProvider) ListPayments(ctx context.Context, filter PaymentFilter) ([]*Payment, error) {
	params := url.Values{}
	if filter.CustomerID != "" {
		params.Set("customer", filter.CustomerID)
	}
	if filter.SubscriptionID != nil {
		params.Set("subscription", *filter.SubscriptionID)
	}
	if filter.Status != nil {
		params.Set("status", string(*filter.Status))
	}
	if filter.DateFrom != nil {
		params.Set("dateCreated[ge]", filter.DateFrom.Format("2006-01-02"))
	}
	if filter.DateTo != nil {
		params.Set("dateCreated[le]", filter.DateTo.Format("2006-01-02"))
	}
	if filter.Limit > 0 {
		params.Set("limit", strconv.Itoa(filter.Limit))
	}
	if filter.Offset > 0 {
		params.Set("offset", strconv.Itoa(filter.Offset))
	}

	endpoint := "/payments"
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	resp, err := p.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}

	payments := make([]*Payment, 0, len(result.Data))
	for _, item := range result.Data {
		payments = append(payments, p.parsePayment(item))
	}
	return payments, nil
}

func (p *AsaasProvider) RefundPayment(ctx context.Context, paymentID string, input RefundInput) (*Refund, error) {
	body := map[string]any{}
	if input.Value != nil {
		body["value"] = *input.Value
	}
	if input.Description != "" {
		body["description"] = input.Description
	}

	resp, err := p.doRequest(ctx, "POST", "/payments/"+paymentID+"/refund", body)
	if err != nil {
		return nil, err
	}

	return p.parseRefund(resp), nil
}

// CreatePayment creates a standalone payment (for proration charges)
func (p *AsaasProvider) CreatePayment(ctx context.Context, input CreatePaymentInput) (*Payment, error) {
	body := map[string]any{
		"customer":           input.CustomerID,
		"value":              input.Value,
		"billingType":        string(input.BillingType),
		"description":        input.Description,
		"dueDate":            input.DueDate.Format("2006-01-02"),
		"externalReference":  input.ExternalReference,
	}

	resp, err := p.doRequest(ctx, "POST", "/payments", body)
	if err != nil {
		return nil, err
	}

	return p.parsePayment(resp), nil
}

// --- Webhook handling ---

func (p *AsaasProvider) VerifyWebhookSignature(payload []byte, signature string) bool {
	if p.webhookKey == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(p.webhookKey))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

func (p *AsaasProvider) ParseWebhookEvent(payload []byte) (*WebhookEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, err
	}

	eventTypeStr, _ := raw["event"].(string)
	eventType := WebhookEventType(eventTypeStr)

	// Extract the main payload
	var eventPayload map[string]any
	if p, ok := raw["payment"].(map[string]any); ok {
		eventPayload = p
	} else if s, ok := raw["subscription"].(map[string]any); ok {
		eventPayload = s
	} else if c, ok := raw["customer"].(map[string]any); ok {
		eventPayload = c
	}

	timestampStr, _ := raw["dateCreated"].(string)
	timestamp, _ := time.Parse(time.RFC3339, timestampStr)

	return &WebhookEvent{
		ID:        fmt.Sprintf("%v", raw["id"]),
		Type:      eventType,
		Timestamp: timestamp,
		Payload:   eventPayload,
		Raw:       payload,
	}, nil
}

// --- HTTP helpers ---

func (p *AsaasProvider) doRequest(ctx context.Context, method, endpoint string, body any) (json.RawMessage, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+endpoint, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access_token", p.apiKey)
	req.Header.Set("User-Agent", "Voltiq/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("asaas API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	// Parse response to extract data
	var result map[string]json.RawMessage
	if err := json.Unmarshal(respBody, &result); err != nil {
		return respBody, nil // Return raw if can't parse
	}

	if data, ok := result["data"]; ok {
		return data, nil
	}

	return respBody, nil
}

// --- Parsers ---

func (p *AsaasProvider) parseCustomer(data json.RawMessage) *Customer {
	var c struct {
		ID               string `json:"id"`
		Name             string `json:"name"`
		Email            string `json:"email"`
		CPFCNPJ          string `json:"cpfCnpj"`
		Phone            string `json:"phone"`
		Address          string `json:"address"`
		AddressNumber    string `json:"addressNumber"`
		Province         string `json:"province"`
		PostalCode       string `json:"postalCode"`
		ExternalReference string `json:"externalReference"`
		DateCreated      string `json:"dateCreated"`
	}
	json.Unmarshal(data, &c)

	createdAt, _ := time.Parse(time.RFC3339, c.DateCreated)

	return &Customer{
		ID:            c.ID,
		ExternalID:    c.ExternalReference,
		Name:          c.Name,
		Email:         c.Email,
		Document:      c.CPFCNPJ,
		Phone:         c.Phone,
		Address:       c.Address,
		AddressNumber: c.AddressNumber,
		Province:      c.Province,
		PostalCode:    c.PostalCode,
		CreatedAt:     createdAt,
	}
}

func (p *AsaasProvider) parseSubscription(data json.RawMessage) *Subscription {
	var s struct {
		ID              string  `json:"id"`
		Customer        string  `json:"customer"`
		Status          string  `json:"status"`
		BillingType     string  `json:"billingType"`
		Cycle           string  `json:"cycle"`
		Value           float64 `json:"value"`
		Description     string  `json:"description"`
		NextDueDate     string  `json:"nextDueDate"`
		ExternalReference string `json:"externalReference"`
		DateCreated     string  `json:"dateCreated"`
		DateUpdated     string  `json:"dateUpdated"`
		DeletedAt       *string `json:"deletedAt"`
	}
	json.Unmarshal(data, &s)

	createdAt, _ := time.Parse(time.RFC3339, s.DateCreated)
	updatedAt, _ := time.Parse(time.RFC3339, s.DateUpdated)
	nextDueDate, _ := time.Parse("2006-01-02", s.NextDueDate)

	var cancelledAt *time.Time
	if s.DeletedAt != nil {
		t, _ := time.Parse(time.RFC3339, *s.DeletedAt)
		cancelledAt = &t
	}

	return &Subscription{
		ID:              s.ID,
		CustomerID:      s.Customer,
		ExternalID:      s.ExternalReference,
		Status:          SubscriptionStatus(s.Status),
		BillingType:     BillingType(s.BillingType),
		Cycle:           BillingCycle(s.Cycle),
		Value:           s.Value,
		Description:     s.Description,
		NextDueDate:     nextDueDate,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		CancelledAt:     cancelledAt,
	}
}

func (p *AsaasProvider) parsePayment(data json.RawMessage) *Payment {
	var pm struct {
		ID             string   `json:"id"`
		Subscription   *string  `json:"subscription"`
		Customer       string   `json:"customer"`
		Value          float64  `json:"value"`
		Status         string   `json:"status"`
		BillingType    string   `json:"billingType"`
		DueDate        string   `json:"dueDate"`
		PaidAt         *string  `json:"paidAt"`
		InvoiceURL     string   `json:"invoiceUrl"`
		BankSlipURL    string   `json:"bankSlipUrl"`
		PIXQRCode      string   `json:"pixQrCode"`
		PIXCode        string   `json:"pixCode"`
		DateCreated    string   `json:"dateCreated"`
		DateUpdated    string   `json:"dateUpdated"`
	}
	json.Unmarshal(data, &pm)

	dueDate, _ := time.Parse("2006-01-02", pm.DueDate)
	createdAt, _ := time.Parse(time.RFC3339, pm.DateCreated)
	updatedAt, _ := time.Parse(time.RFC3339, pm.DateUpdated)

	var paidAt *time.Time
	if pm.PaidAt != nil {
		t, _ := time.Parse(time.RFC3339, *pm.PaidAt)
		paidAt = &t
	}

	return &Payment{
		ID:             pm.ID,
		SubscriptionID: pm.Subscription,
		CustomerID:     pm.Customer,
		Value:          pm.Value,
		Status:         PaymentStatus(pm.Status),
		BillingType:    BillingType(pm.BillingType),
		DueDate:        dueDate,
		PaidAt:         paidAt,
		InvoiceURL:     pm.InvoiceURL,
		BankSlipURL:    pm.BankSlipURL,
		PIXQRCode:      pm.PIXQRCode,
		PIXCode:        pm.PIXCode,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}

func (p *AsaasProvider) parseRefund(data json.RawMessage) *Refund {
	var r struct {
		ID          string  `json:"id"`
		Payment     string  `json:"payment"`
		Value       float64 `json:"value"`
		Status      string  `json:"status"`
		Description string  `json:"description"`
		DateCreated string  `json:"dateCreated"`
	}
	json.Unmarshal(data, &r)

	createdAt, _ := time.Parse(time.RFC3339, r.DateCreated)

	return &Refund{
		ID:          r.ID,
		PaymentID:   r.Payment,
		Value:       r.Value,
		Status:      RefundStatus(r.Status),
		Description: r.Description,
		CreatedAt:   createdAt,
	}
}

// --- Errors ---

var (
	ErrCustomerNotFound      = errors.New("customer not found")
	ErrSubscriptionNotFound  = errors.New("subscription not found")
	ErrPaymentNotFound       = errors.New("payment not found")
	ErrInvalidWebhookSignature = errors.New("invalid webhook signature")
)

// IsNotFoundError checks if error is a not found error
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrCustomerNotFound) ||
		errors.Is(err, ErrSubscriptionNotFound) ||
		errors.Is(err, ErrPaymentNotFound)
}

// MapAsaasError maps Asaas API errors to our domain errors
func MapAsaasError(statusCode int, body string) error {
	if statusCode == 404 {
		return ErrSubscriptionNotFound // Generic not found
	}
	if statusCode == 400 {
		// Check for specific error codes
		if strings.Contains(body, "customer not found") {
			return ErrCustomerNotFound
		}
		if strings.Contains(body, "subscription not found") {
			return ErrSubscriptionNotFound
		}
	}
	return fmt.Errorf("asaas error: %d - %s", statusCode, body)
}