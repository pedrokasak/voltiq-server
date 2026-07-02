package middleware

import (
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// SecurityHeaders is a map of security headers
type SecurityHeaders map[string]string

// DefaultSecurityHeaders returns default security headers
func DefaultSecurityHeaders() SecurityHeaders {
	return SecurityHeaders{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"X-XSS-Protection":        "1; mode=block",
		"X-Permitted-Cross-Domain-Policies": "none",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Cache-Control":           "no-store, no-cache, must-revalidate, proxy-revalidate",
		"Pragma":                  "no-cache",
		"Expires":                 "0",
	}
}

// SecurityMiddleware applies security headers and XSS protection
type SecurityMiddleware struct {
	headers SecurityHeaders
}

// NewSecurityMiddleware creates a new SecurityMiddleware
func NewSecurityMiddleware(customHeaders ...SecurityHeaders) *SecurityMiddleware {
	headers := DefaultSecurityHeaders()
	
	for _, custom := range customHeaders {
		for k, v := range custom {
			headers[k] = v
		}
	}
	
	return &SecurityMiddleware{
		headers: headers,
	}
}

// Handler applies security headers
func (m *SecurityMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set security headers
		for header, value := range m.headers {
			w.Header().Set(header, value)
		}
		
		// Set Content-Security-Policy
		csp := m.buildCSP(r)
		w.Header().Set("Content-Security-Policy", csp)
		
		next.ServeHTTP(w, r)
	})
}

// buildCSP builds Content-Security-Policy header
func (m *SecurityMiddleware) buildCSP(r *http.Request) string {
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	
	return fmt.Sprintf(
		"default-src 'self'; "+
			"script-src 'self'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data: https:; "+
			"font-src 'self'; "+
			"connect-src 'self' https://%s; "+
			"frame-ancestors 'none'; "+
			"base-uri 'self'; "+
			"form-action 'self'",
		host,
	)
}

// XSSProtectionMiddleware sanitizes input to prevent XSS
type XSSProtectionMiddleware struct{}

// NewXSSProtectionMiddleware creates new XSS protection middleware
func NewXSSProtectionMiddleware() *XSSProtectionMiddleware {
	return &XSSProtectionMiddleware{}
}

// Handler sanitizes request input
func (m *XSSProtectionMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sanitize query parameters
		query := r.URL.Query()
		for key, values := range query {
			for i, value := range values {
				query[key][i] = SanitizeInput(value)
			}
		}
		r.URL.RawQuery = query.Encode()
		
		// Sanitize headers that might contain user input
		if r.Header.Get("X-Custom-Input") != "" {
			r.Header.Set("X-Custom-Input", SanitizeInput(r.Header.Get("X-Custom-Input")))
		}
		
		next.ServeHTTP(w, r)
	})
}

// SanitizeInput sanitizes string input to prevent XSS
func SanitizeInput(input string) string {
	// Escape HTML entities
	sanitized := html.EscapeString(input)
	
	// Remove javascript: protocol
	sanitized = regexp.MustCompile(`(?i)javascript:`).ReplaceAllString(sanitized, "")
	
	// Remove data: URIs that could contain scripts
	sanitized = regexp.MustCompile(`(?i)data\s*:`).ReplaceAllString(sanitized, "")
	
	// Remove event handlers
	eventHandlers := []string{
		`on\w+\s*=`,
		`<script`,
		`</script>`,
		`<iframe`,
		`</iframe>`,
		`<object`,
		`</object>`,
		`<embed`,
		`</embed>`,
	}
	
	for _, pattern := range eventHandlers {
		re := regexp.MustCompile(pattern)
		sanitized = re.ReplaceAllString(sanitized, "")
	}
	
	// Trim whitespace
	sanitized = strings.TrimSpace(sanitized)
	
	return sanitized
}

// RequestIDMiddleware adds unique request ID for tracking
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), r.Context().Value(&contextKey{}))
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r)
	})
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

// CORSMiddleware handles CORS with security
type CORSMiddleware struct {
	config CORSConfig
}

// NewCORSMiddleware creates new CORS middleware
func NewCORSMiddleware(config CORSConfig) *CORSMiddleware {
	if config.AllowedMethods == nil {
		config.AllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	}
	
	if config.AllowedHeaders == nil {
		config.AllowedHeaders = []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-CSRF-Token",
			"X-Request-ID",
		}
	}
	
	if config.ExposedHeaders == nil {
		config.ExposedHeaders = []string{
			"X-Request-ID",
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
		}
	}
	
	if config.MaxAge == 0 {
		config.MaxAge = 300
	}
	
	return &CORSMiddleware{
		config: config,
	}
}

// Handler applies CORS headers
func (m *CORSMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		
		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range m.config.AllowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}
		
		if !allowed && origin != "" {
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}
		
		// Set CORS headers
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(m.config.AllowedMethods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(m.config.AllowedHeaders, ", "))
		w.Header().Set("Access-Control-Expose-Headers", strings.Join(m.config.ExposedHeaders, ", "))
		w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", m.config.MaxAge))
		
		if m.config.AllowCredentials {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		
		// Handle preflight
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}
