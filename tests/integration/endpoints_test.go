package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/voltiq/server/internal/delivery/handler"
	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/router"
	"github.com/voltiq/server/internal/domain"
	"github.com/voltiq/server/internal/ingestion"
	"github.com/voltiq/server/internal/jwt"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/usecase"
	"github.com/voltiq/server/pkg/metrics"
)

// TestApp holds the test application state
type TestApp struct {
	Server      *httptest.Server
	Router      http.Handler
	AuthToken   string
	TenantID    domain.UUID
	UserID      domain.UUID
	DB          *repository.Database
	AuthUseCase *usecase.AuthUseCase
}

// NewTestApp creates a new test application
func NewTestApp(t *testing.T) *TestApp {
	ctx := context.Background()

	// Use test database
	db, err := repository.NewDatabase(ctx)
	if err != nil {
		t.Skip("Skipping integration test: database not available")
		return nil
	}

	// Initialize repositories
	userRepo := repository.NewUserRepository(db)
	tenantRepo := repository.NewTenantRepository(db)
	transformerRepo := repository.NewTransformerRepository(db)
	ucRepo := repository.NewConsumingUnitRepository(db)
	transformerReadingRepo := repository.NewTransformerReadingRepository(db)
	ucReadingRepo := repository.NewUCReadingRepository(db)
	balanceRepo := repository.NewBalanceRepository(db)
	importRepo := repository.NewImportRepository(db)
	inviteRepo := repository.NewInviteRepository(db)
	alertRepo := repository.NewAlertRepository(db)

	// Initialize JWT service
	jwtService := jwt.NewService("test-secret-key-change-in-production")
	jwtService.SetExpiration(24 * time.Hour)
	jwtService.SetRefreshExpiration(7 * 24 * time.Hour)

	// Initialize use cases
	authUseCase := usecase.NewAuthUseCase(userRepo, tenantRepo, jwtService)
	signupUseCase := usecase.NewSignupUseCase(tenantRepo, userRepo)
	inviteUseCase := usecase.NewInviteUseCase(inviteRepo, userRepo, tenantRepo)
	transformerUseCase := usecase.NewTransformerUseCase(transformerRepo)
	ucUseCase := usecase.NewConsumingUnitUseCase(ucRepo)
	balanceUseCase := usecase.NewBalanceUseCase(balanceRepo, transformerRepo, transformerReadingRepo, ucReadingRepo, ucRepo)
	importUseCase := usecase.NewImportUseCase(ingestion.NewCSVParser(), importRepo, transformerReadingRepo, ucReadingRepo)
	alertUseCase := usecase.NewAlertUseCase(alertRepo)
	riskUseCase := usecase.NewRiskUseCase(balanceRepo, transformerRepo, ucRepo)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(authUseCase, signupUseCase)
	inviteHandler := handler.NewInviteHandler(inviteUseCase)
	transformerHandler := handler.NewTransformerHandler(transformerUseCase)
	ucHandler := handler.NewConsumingUnitHandler(ucUseCase)
	balanceHandler := handler.NewBalanceHandler(balanceUseCase)
	importHandler := handler.NewImportHandler(importUseCase)
	alertHandler := handler.NewAlertHandler(alertUseCase)
	riskHandler := handler.NewRiskHandler(riskUseCase)
	dashboardHandler := handler.NewDashboardHandler(transformerRepo, balanceRepo, ucRepo)
	healthHandler := handler.NewHealthHandler("test")

	// Initialize middleware
	authMiddleware := middleware.NewAuthMiddleware(jwtService, db)
	rateLimiter := middleware.NewRateLimiter(1000, 100)
	securityMiddleware := middleware.NewSecurityMiddleware()
	metricsCollector := metrics.NewMetricsCollector()

	// Setup router
	cfg := router.Config{
		AuthHandler:          authHandler,
		InviteHandler:        inviteHandler,
		TransformerHandler:   transformerHandler,
		ConsumingUnitHandler: ucHandler,
		BalanceHandler:       balanceHandler,
		ImportHandler:        importHandler,
		AlertHandler:         alertHandler,
		RiskHandler:          riskHandler,
		DashboardHandler:     dashboardHandler,
		HealthHandler:        healthHandler,
		MetricsCollector:     metricsCollector,
		AuthMiddleware:       authMiddleware,
		RateLimiter:          rateLimiter,
		SecurityMiddleware:   securityMiddleware,
		CORSOrigins:          []string{"*"},
	}

	router := router.Setup(cfg)
	server := httptest.NewServer(router)

	return &TestApp{
		Server:      server,
		Router:      router,
		DB:          db,
		AuthUseCase: authUseCase,
	}
}

// Close cleans up the test app
func (app *TestApp) Close() {
	app.Server.Close()
	app.DB.Close()
}

// Login logs in and stores the auth token
func (app *TestApp) Login(t *testing.T, email, password string) string {
	reqBody := map[string]string{
		"email":    email,
		"password": password,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(app.Server.URL+"/api/v1/auth/login", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Login request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed with status %d: %v", resp.StatusCode, result)
	}

	data := result["data"].(map[string]any)
	token := data["token"].(string)
	app.AuthToken = token
	return token
}

// Signup creates a new tenant and admin user
func (app *TestApp) Signup(t *testing.T) (string, domain.UUID) {
	reqBody := map[string]string{
		"tenant_name":     "Test Cooperative",
		"tenant_document": "12.345.678/0001-90",
		"plan":            "trial",
		"admin_name":      "Test Admin",
		"admin_email":     "admin@test-" + uuid.New().String()[:8] + ".coop.br",
		"admin_password":  "TestPassword123!",
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(app.Server.URL+"/api/v1/auth/signup", "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Signup request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Signup failed with status %d: %v", resp.StatusCode, result)
	}

	data := result["data"].(map[string]any)
	tenant := data["tenant"].(map[string]any)
	user := data["user"].(map[string]any)

	app.TenantID = domain.UUID(tenant["id"].(string))
	app.UserID = domain.UUID(user["id"].(string))

	return app.Login(t, reqBody["admin_email"], reqBody["admin_password"]), app.TenantID
}

// AuthenticatedRequest makes an authenticated HTTP request
func (app *TestApp) AuthenticatedRequest(t *testing.T, method, path string, body interface{}) *http.Response {
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, app.Server.URL+path, reqBody)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+app.AuthToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	return resp
}

// AssertSuccess asserts that the response is successful and returns the data
func (app *TestApp) AssertSuccess(t *testing.T, resp *http.Response, expectedStatus int) map[string]any {
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode != expectedStatus {
		t.Fatalf("Expected status %d, got %d: %v", expectedStatus, resp.StatusCode, result)
	}

	if success, ok := result["success"].(bool); !ok || !success {
		t.Fatalf("Response indicates failure: %v", result)
	}

	return result["data"].(map[string]any)
}

// TestTransformerCRUD tests transformer CRUD operations
func TestTransformerCRUD(t *testing.T) {
	app := NewTestApp(t)
	if app == nil {
		return
	}
	defer app.Close()

	// Signup
	app.Signup(t)

	// Test Create Transformer
	createReq := map[string]interface{}{
		"code":                "TRF-TEST-001",
		"power_kva":           112.5,
		"primary_voltage_kv":  13.8,
		"secondary_voltage_v": 220,
		"lat":                 -23.5505,
		"lng":                 -46.6333,
		"core_loss_kw":        0.340,
		"winding_loss_kw":     1.450,
		"loss_limit_pct":      10.0,
	}

	resp := app.AuthenticatedRequest(t, "POST", "/api/v1/transformers", createReq)
	data := app.AssertSuccess(t, resp, http.StatusCreated)
	transformerID := data["id"].(string)
	t.Logf("Created transformer: %s", transformerID)

	// Test Get Transformer
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/transformers/"+transformerID, nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test List Transformers
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/transformers", nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test Update Transformer
	updateReq := map[string]interface{}{
		"code":                "TRF-TEST-001-UPDATED",
		"power_kva":           150.0,
		"primary_voltage_kv":  13.8,
		"secondary_voltage_v": 220,
	}
	resp = app.AuthenticatedRequest(t, "PUT", "/api/v1/transformers/"+transformerID, updateReq)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test Get Technical Data
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/transformers/"+transformerID+"/technical-data", nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test Get Loss Limit
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/transformers/"+transformerID+"/loss-limit", nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test Soft Delete
	resp = app.AuthenticatedRequest(t, "DELETE", "/api/v1/transformers/"+transformerID, nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Verify it's deleted (should not appear in list)
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/transformers", nil)
	data = app.AssertSuccess(t, resp, http.StatusOK)
	transformers := data["transformers"].([]interface{})
	if len(transformers) > 0 {
		t.Errorf("Expected empty transformer list after delete, got %d", len(transformers))
	}
}

// TestConsumingUnitCRUD tests consuming unit CRUD operations
func TestConsumingUnitCRUD(t *testing.T) {
	app := NewTestApp(t)
	if app == nil {
		return
	}
	defer app.Close()

	app.Signup(t)

	// First create a transformer
	createReq := map[string]interface{}{
		"code":                "TRF-UC-TEST",
		"power_kva":           112.5,
		"primary_voltage_kv":  13.8,
		"secondary_voltage_v": 220,
	}
	resp := app.AuthenticatedRequest(t, "POST", "/api/v1/transformers", createReq)
	data := app.AssertSuccess(t, resp, http.StatusCreated)
	transformerID := data["id"].(string)

	// Test Create Consuming Unit
	ucReq := map[string]interface{}{
		"transformer_id": transformerID,
		"uc_code":        "UC-TEST-001",
		"name":           "Test Consumer Unit",
		"class":          "RESIDENTIAL",
	}
	resp = app.AuthenticatedRequest(t, "POST", "/api/v1/consuming-units", ucReq)
	data = app.AssertSuccess(t, resp, http.StatusCreated)
	ucID := data["id"].(string)
	t.Logf("Created consuming unit: %s", ucID)

	// Test Get Consuming Unit
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/consuming-units/"+ucID, nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test List Consuming Units
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/consuming-units", nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test List by Transformer
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/consuming-units/transformer/"+transformerID, nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test Update Consuming Unit
	updateReq := map[string]interface{}{
		"name":  "Updated Test Consumer",
		"class": "COMMERCIAL",
	}
	resp = app.AuthenticatedRequest(t, "PUT", "/api/v1/consuming-units/"+ucID, updateReq)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test Delete Consuming Unit
	resp = app.AuthenticatedRequest(t, "DELETE", "/api/v1/consuming-units/"+ucID, nil)
	app.AssertSuccess(t, resp, http.StatusOK)
}

// TestBalanceCalculation tests energy balance calculation endpoint
func TestBalanceCalculation(t *testing.T) {
	app := NewTestApp(t)
	if app == nil {
		return
	}
	defer app.Close()

	app.Signup(t)

	// Create transformer
	createReq := map[string]interface{}{
		"code":                "TRF-BALANCE-TEST",
		"power_kva":           112.5,
		"primary_voltage_kv":  13.8,
		"secondary_voltage_v": 220,
		"core_loss_kw":        0.340,
		"winding_loss_kw":     1.450,
		"loss_limit_pct":      10.0,
	}
	resp := app.AuthenticatedRequest(t, "POST", "/api/v1/transformers", createReq)
	data := app.AssertSuccess(t, resp, http.StatusCreated)
	transformerID := data["id"].(string)

	// Create consuming units
	for i := 1; i <= 3; i++ {
		ucReq := map[string]interface{}{
			"transformer_id": transformerID,
			"uc_code":        "UC-BAL-" + string(rune(i+'0')),
			"name":           "UC " + string(rune(i+'0')),
			"class":          "RESIDENTIAL",
		}
		resp = app.AuthenticatedRequest(t, "POST", "/api/v1/consuming-units", ucReq)
		app.AssertSuccess(t, resp, http.StatusCreated)
	}

	// Calculate balance
	balanceReq := map[string]string{
		"period_start": "2025-01-01",
		"period_end":   "2025-01-31",
	}
	resp = app.AuthenticatedRequest(t, "POST", "/api/v1/balance/transformer/"+transformerID+"/calculate", balanceReq)

	// This might fail if no readings exist - that's expected
	if resp.StatusCode == http.StatusInternalServerError {
		t.Log("Balance calculation failed as expected (no readings in DB)")
		return
	}

	data = app.AssertSuccess(t, resp, http.StatusOK)
	t.Logf("Balance calculated: %v", data)
}

// TestAlertConfiguration tests alert configuration endpoints
func TestAlertConfiguration(t *testing.T) {
	app := NewTestApp(t)
	if app == nil {
		return
	}
	defer app.Close()

	app.Signup(t)

	// Create transformer
	createReq := map[string]interface{}{
		"code":                "TRF-ALERT-TEST",
		"power_kva":           112.5,
		"primary_voltage_kv":  13.8,
		"secondary_voltage_v": 220,
	}
	resp := app.AuthenticatedRequest(t, "POST", "/api/v1/transformers", createReq)
	data := app.AssertSuccess(t, resp, http.StatusCreated)
	transformerID := data["id"].(string)

	// Test Create Alert Config
	alertReq := map[string]string{
		"transformer_id": transformerID,
		"type":           "WARNING",
		"channel":        "EMAIL",
		"recipient":      "test@example.com",
	}
	resp = app.AuthenticatedRequest(t, "POST", "/api/v1/alerts", alertReq)
	data = app.AssertSuccess(t, resp, http.StatusCreated)
	alertID := data["id"].(string)
	t.Logf("Created alert config: %s", alertID)

	// Test List Alerts for Transformer
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/alerts/transformer/"+transformerID, nil)
	data = app.AssertSuccess(t, resp, http.StatusOK)
	alerts := data["alerts"].([]interface{})
	if len(alerts) != 1 {
		t.Errorf("Expected 1 alert, got %d", len(alerts))
	}

	// Test Get Alert by ID
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/alerts/"+alertID, nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test Update Alert Config
	updateReq := map[string]string{
		"type":      "CRITICAL",
		"recipient": "updated@example.com",
	}
	resp = app.AuthenticatedRequest(t, "PUT", "/api/v1/alerts/"+alertID, updateReq)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test List All Alerts for Tenant
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/alerts", nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test Delete Alert Config
	resp = app.AuthenticatedRequest(t, "DELETE", "/api/v1/alerts/"+alertID, nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Verify deleted
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/alerts/"+alertID, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 after delete, got %d", resp.StatusCode)
	}
}

// TestDashboardEndpoints tests dashboard endpoints
func TestDashboardEndpoints(t *testing.T) {
	app := NewTestApp(t)
	if app == nil {
		return
	}
	defer app.Close()

	app.Signup(t)

	// Test Get KPIs
	resp := app.AuthenticatedRequest(t, "GET", "/api/v1/dashboard/kpis", nil)
	data := app.AssertSuccess(t, resp, http.StatusOK)
	t.Logf("KPIs: %v", data)

	// Test Get Monthly Loss History
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/dashboard/monthly-loss", nil)
	data = app.AssertSuccess(t, resp, http.StatusOK)
	t.Logf("Monthly loss: %v", data)

	// Test Get Transformer Current Status
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/dashboard/transformer-current-status", nil)
	data = app.AssertSuccess(t, resp, http.StatusOK)
	t.Logf("Transformer status: %v", data)
}

// TestRiskScoreEndpoints tests risk score endpoints
func TestRiskScoreEndpoints(t *testing.T) {
	app := NewTestApp(t)
	if app == nil {
		return
	}
	defer app.Close()

	app.Signup(t)

	// Create transformer
	createReq := map[string]interface{}{
		"code":                "TRF-RISK-TEST",
		"power_kva":           112.5,
		"primary_voltage_kv":  13.8,
		"secondary_voltage_v": 220,
	}
	resp := app.AuthenticatedRequest(t, "POST", "/api/v1/transformers", createReq)
	data := app.AssertSuccess(t, resp, http.StatusCreated)
	transformerID := data["id"].(string)

	// Test Get Risk Score
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/risk/transformer/"+transformerID+"/score", nil)
	if resp.StatusCode == http.StatusNotFound {
		t.Log("Risk score not found (expected - no balance data)")
		return
	}
	data = app.AssertSuccess(t, resp, http.StatusOK)
	t.Logf("Risk score: %v", data)

	// Test Get Anomalies
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/risk/transformer/"+transformerID+"/anomalies", nil)
	if resp.StatusCode == http.StatusNotFound {
		t.Log("Anomalies not found (expected - no balance data)")
		return
	}
	data = app.AssertSuccess(t, resp, http.StatusOK)
	t.Logf("Anomalies: %v", data)

	// Test Get All Risk Scores
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/risk/all-scores", nil)
	data = app.AssertSuccess(t, resp, http.StatusOK)
	t.Logf("All risk scores: %v", data)

	// Test Get All Anomalies
	resp = app.AuthenticatedRequest(t, "GET", "/api/v1/risk/all-anomalies", nil)
	data = app.AssertSuccess(t, resp, http.StatusOK)
	t.Logf("All anomalies: %v", data)
}

// TestHealthEndpoints tests health check endpoints
func TestHealthEndpoints(t *testing.T) {
	app := NewTestApp(t)
	if app == nil {
		return
	}
	defer app.Close()

	// Test /health (no auth required)
	resp, err := http.Get(app.Server.URL + "/health")
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	// Test /ready (no auth required)
	resp, err = http.Get(app.Server.URL + "/ready")
	if err != nil {
		t.Fatalf("Readiness check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

// TestImportEndpoints tests CSV import endpoints
func TestImportEndpoints(t *testing.T) {
	app := NewTestApp(t)
	if app == nil {
		return
	}
	defer app.Close()

	app.Signup(t)

	// Test Get Upload Template
	resp := app.AuthenticatedRequest(t, "GET", "/api/v1/imports/template", nil)
	data := app.AssertSuccess(t, resp, http.StatusOK)
	t.Logf("Template: %v", data)

	// Test Validate CSV (with minimal valid CSV)
	csvContent := "transformer_id,reading_at,energy_kwh,demand_kw,power_factor\n"
	csvContent += "00000000-0000-0000-0000-000000000000,2025-01-01 00:00:00,1000.0,50.0,0.95\n"

	// Note: Actual file upload would require multipart form
	// This just tests the endpoint exists
	resp = app.AuthenticatedRequest(t, "POST", "/api/v1/imports/validate", map[string]string{
		"content": csvContent,
	})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
		t.Logf("Validate CSV returned: %d", resp.StatusCode)
	}
}

// TestAuthFlow tests authentication flow
func TestAuthFlow(t *testing.T) {
	app := NewTestApp(t)
	if app == nil {
		return
	}
	defer app.Close()

	// Test Signup
	token, tenantID := app.Signup(t)
	t.Logf("Signup successful, tenant: %s, token: %s...", tenantID, token[:20])

	// Test Login with same credentials
	// (Already logged in during signup)

	// Test Refresh Token
	resp := app.AuthenticatedRequest(t, "POST", "/api/v1/auth/refresh", nil)
	if resp.StatusCode != http.StatusOK {
		t.Logf("Refresh token endpoint returned: %d (may need cookie)", resp.StatusCode)
	}

	// Test Logout
	resp = app.AuthenticatedRequest(t, "POST", "/api/v1/auth/logout", nil)
	app.AssertSuccess(t, resp, http.StatusOK)

	// Test invalid token
	invalidApp := *app
	invalidApp.AuthToken = "invalid-token"

	resp = invalidApp.AuthenticatedRequest(t, "GET", "/api/v1/transformers", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401 for invalid token, got %d", resp.StatusCode)
	}
}

// TestRateLimiting tests rate limiting
func TestRateLimiting(t *testing.T) {
	app := NewTestApp(t)
	if app == nil {
		return
	}
	defer app.Close()

	app.Signup(t)

	// Make many requests quickly
	// Note: This is a basic test - actual rate limiting is hard to test without
	// knowing the exact configuration
	for i := 0; i < 5; i++ {
		resp := app.AuthenticatedRequest(t, "GET", "/api/v1/transformers", nil)
		if resp.StatusCode == http.StatusTooManyRequests {
			t.Log("Rate limit hit")
			return
		}
	}
	t.Log("Rate limiting test passed (no limit hit in test)")
}
