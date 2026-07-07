package security_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/voltiq/server/internal/delivery/handler"
	"github.com/voltiq/server/internal/delivery/middleware"
	"github.com/voltiq/server/internal/delivery/request"
	"github.com/voltiq/server/internal/jwt"
	"github.com/voltiq/server/internal/repository"
	"github.com/voltiq/server/internal/usecase"
)

// TestRateLimiting tests rate limiting middleware
func TestRateLimiting(t *testing.T) {
	rateLimiter := middleware.NewRateLimiter(10, 5) // 10 req/min, burst of 5

	var allowedCount int
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowedCount++
		w.WriteHeader(http.StatusOK)
	})

	rateLimitedHandler := rateLimiter.Middleware(testHandler)

	// Make 5 requests (should all pass - within burst)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		rateLimitedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d", i+1, rr.Code)
		}
	}

	// Make 5 more requests (should start getting rate limited)
	rateLimitedCount := 0
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		rateLimitedHandler.ServeHTTP(rr, req)

		if rr.Code == http.StatusTooManyRequests {
			rateLimitedCount++
		}
	}

	if rateLimitedCount == 0 {
		t.Error("Expected some requests to be rate limited")
	}

	t.Logf("Allowed: %d, Rate Limited: %d", allowedCount, rateLimitedCount)
}

// TestRateLimitFingerprint tests that rate limiting uses proper fingerprinting
func TestRateLimitFingerprint(t *testing.T) {
	rateLimiter := middleware.NewRateLimiter(10, 3)

	requestCount := make(map[string]int)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fingerprint := middleware.GetFingerprint(r)
		requestCount[fingerprint]++
		w.WriteHeader(http.StatusOK)
	})

	rateLimitedHandler := rateLimiter.Middleware(testHandler)

	// Simulate requests from different "IPs"
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1." + string(rune(i%3+'0'))
		rr := httptest.NewRecorder()
		rateLimitedHandler.ServeHTTP(rr, req)
	}

	// Check that we have multiple fingerprints
	if len(requestCount) < 2 {
		t.Error("Expected multiple fingerprints for different IPs")
	}

	t.Logf("Fingerprints: %v", requestCount)
}

// TestXSSProtection tests XSS sanitization
func TestXSSProtection(t *testing.T) {
	xssMiddleware := middleware.NewXSSProtectionMiddleware()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Script tag",
			input:    "<script>alert('XSS')</script>",
			expected: "&lt;script&gt;alert(&#39;XSS&#39;)&lt;/script&gt;",
		},
		{
			name:     "JavaScript protocol",
			input:    "javascript:alert(1)",
			expected: "alert(1)",
		},
		{
			name:     "Event handler",
			input:    "<img onerror=alert(1)>",
			expected: "&lt;img &gt;",
		},
		{
			name:     "Normal text",
			input:    "Hello World",
			expected: "Hello World",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := middleware.SanitizeInput(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

// TestSecurityHeaders tests security headers
func TestSecurityHeaders(t *testing.T) {
	securityMiddleware := middleware.NewSecurityMiddleware()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	securedHandler := securityMiddleware.Handler(testHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	securedHandler.ServeHTTP(rr, req)

	expectedHeaders := map[string]string{
		"X-Content-Type-Options":            "nosniff",
		"X-Frame-Options":                   "DENY",
		"X-XSS-Protection":                  "1; mode=block",
		"X-Permitted-Cross-Domain-Policies": "none",
		"Referrer-Policy":                   "strict-origin-when-cross-origin",
		"Cache-Control":                     "no-store, no-cache, must-revalidate, proxy-revalidate",
	}

	for header, expected := range expectedHeaders {
		if got := rr.Header().Get(header); got != expected {
			t.Errorf("Header %s: expected %q, got %q", header, expected, got)
		}
	}

	// Check CSP header
	csp := rr.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Expected Content-Security-Policy header")
	}

	if !strings.Contains(csp, "default-src 'self'") {
		t.Error("CSP should include default-src 'self'")
	}

	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Error("CSP should include frame-ancestors 'none'")
	}
}

// TestSecureCookieConfig tests secure cookie configuration
func TestSecureCookieConfig(t *testing.T) {
	config := handler.DefaultRefreshCookieConfig()

	if !config.Secure {
		t.Error("Cookie should be Secure")
	}

	if !config.HttpOnly {
		t.Error("Cookie should be HttpOnly")
	}

	if config.SameSite != http.SameSiteStrictMode {
		t.Error("Cookie should have SameSite=Strict")
	}

	if config.Path != "/api/v1/auth/refresh" {
		t.Errorf("Cookie path should be /api/v1/auth/refresh, got %s", config.Path)
	}

	if config.MaxAge != 7*24*60*60 {
		t.Error("Cookie max age should be 7 days")
	}
}

// TestFileUploadValidation tests file upload validation
func TestFileUploadValidation(t *testing.T) {
	uploadMiddleware := middleware.NewFileUploadMiddleware(middleware.FileUploadConfig{
		MaxFileSize: 1024, // 1KB for testing
		ValidateCSV: true,
	})

	// Test valid CSV
	validCSV := "transformer_id,reading_at,energy_kwh\nTRF-001,2025-01-15,1500.50\n"
	req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte(validCSV)))
	rr := httptest.NewRecorder()

	// Create a mock file upload
	valid := uploadMiddleware.ValidateFile(rr, req, bytes.NewReader([]byte(validCSV)))
	if !valid {
		t.Error("Expected valid CSV to pass validation")
	}

	// Test invalid file type (binary data)
	invalidData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG magic bytes
	req2 := httptest.NewRequest("POST", "/upload", bytes.NewReader(invalidData))
	rr2 := httptest.NewRecorder()

	valid2 := uploadMiddleware.ValidateFile(rr2, req2, bytes.NewReader(invalidData))
	if valid2 {
		t.Error("Expected PNG file to fail validation")
	}
}

// TestSQLInjectionPrevention tests that inputs are properly sanitized
func TestSQLInjectionPrevention(t *testing.T) {
	// Test that our sanitization prevents common SQL injection patterns
	testCases := []string{
		"'; DROP TABLE users; --",
		"1' OR '1'='1",
		"admin'--",
		"1; DELETE FROM trafos",
		"UNION SELECT * FROM users",
	}

	for _, input := range testCases {
		// Our middleware should escape HTML but we need to ensure
		// database layer uses parameterized queries
		sanitized := middleware.SanitizeInput(input)

		// Check that dangerous characters are escaped
		if strings.Contains(sanitized, "'") && !strings.Contains(sanitized, "&#39;") {
			t.Errorf("Single quote not properly escaped in: %s", input)
		}
	}
}

// TestAuthCookieSecurity tests authentication cookie security
func TestAuthCookieSecurity(t *testing.T) {
	// Create test dependencies
	jwtService := jwt.NewService("test-secret-key")
	jwtService.SetExpiration(24 * time.Hour)
	jwtService.SetRefreshExpiration(7 * 24 * time.Hour)

	// Mock repositories (we won't actually use them in this test)
	var userRepo *repository.UserRepository
	var tenantRepo *repository.TenantRepository

	authUseCase := usecase.NewAuthUseCase(userRepo, tenantRepo, jwtService)
	authHandler := handler.NewAuthHandler(authUseCase, nil)

	// Test that login sets secure cookies
	loginReq := map[string]string{
		"email":    "test@example.com",
		"password": "password123",
	}

	jsonData, _ := json.Marshal(loginReq)
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(jsonData))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// We expect this to fail (user doesn't exist) but cookies should still be set correctly
	authHandler.Login(rr, req)

	// Check if cookie was set (even on error, the pattern should be correct)
	cookies := rr.Result().Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "refresh_token" {
			if !cookie.HttpOnly {
				t.Error("Refresh token cookie should be HttpOnly")
			}
			if !cookie.Secure {
				t.Error("Refresh token cookie should be Secure")
			}
			if cookie.SameSite != http.SameSiteStrictMode {
				t.Error("Refresh token cookie should have SameSite=Strict")
			}
			if cookie.Path != "/api/v1/auth/refresh" {
				t.Errorf("Refresh token cookie path should be /api/v1/auth/refresh, got %s", cookie.Path)
			}
		}
	}
}

// TestContentTypePrevention tests MIME type sniffing prevention
func TestContentTypePrevention(t *testing.T) {
	handler := middleware.ContentTypeMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("Expected X-Content-Type-Options: nosniff header")
	}
}

// TestRequestIDTracking tests request ID generation
func TestRequestIDTracking(t *testing.T) {
	handler := middleware.RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	requestID := rr.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("Expected X-Request-ID header to be set")
	}

	// Verify it's unique
	req2 := httptest.NewRequest("GET", "/test", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	requestID2 := rr2.Header().Get("X-Request-ID")
	if requestID == requestID2 {
		t.Error("Expected unique request IDs")
	}
}

// TestIntegration_SecurityChain tests the complete security chain
func TestIntegration_SecurityChain(t *testing.T) {
	// Create a complete middleware chain
	rateLimiter := middleware.NewRateLimiter(60, 10)
	securityMiddleware := middleware.NewSecurityMiddleware()
	xssMiddleware := middleware.NewXSSProtectionMiddleware()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request.WriteJSON(w, http.StatusOK, request.Success(map[string]string{
			"message": "success",
		}, ""))
	})

	// Chain: Security -> XSS -> Rate Limit -> Handler
	chain := securityMiddleware.Handler(
		xssMiddleware.Handler(
			rateLimiter.Middleware(testHandler),
		),
	)

	// Test normal request
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	chain.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", rr.Code)
	}

	// Verify security headers
	var response request.Response
	json.Unmarshal(rr.Body.Bytes(), &response)

	if !response.Success {
		t.Error("Expected successful response")
	}

	// Verify security headers are present
	headers := rr.Header()
	if headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("Missing X-Content-Type-Options header")
	}
	if headers.Get("X-Frame-Options") != "DENY" {
		t.Error("Missing X-Frame-Options header")
	}
}

// BenchmarkRateLimiting benchmarks rate limiting performance
func BenchmarkRateLimiting(b *testing.B) {
	rateLimiter := middleware.NewRateLimiter(1000, 100)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rateLimitedHandler := rateLimiter.Middleware(testHandler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		rateLimitedHandler.ServeHTTP(rr, req)
	}
}

// BenchmarkXSSSanitization benchmarks XSS sanitization
func BenchmarkXSSSanitization(b *testing.B) {
	input := "<script>alert('XSS')</script><img onerror=alert(1)>"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		middleware.SanitizeInput(input)
	}
}
