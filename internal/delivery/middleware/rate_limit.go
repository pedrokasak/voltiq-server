package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/voltiq/server/internal/delivery/request"
)

// RateLimiterConfig holds rate limiting configuration
type RateLimiterConfig struct {
	RequestsPerMinute int
	Burst             int
}

// RateLimiter implements token bucket rate limiting with fingerprinting
type RateLimiter struct {
	mu       sync.RWMutex
	buckets  map[string]*tokenBucket
	config   RateLimiterConfig
	stopChan chan struct{}
}

type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerMinute, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		config:   RateLimiterConfig{RequestsPerMinute: requestsPerMinute, Burst: burst},
		stopChan: make(chan struct{}),
	}

	// Cleanup old buckets periodically
	go rl.cleanup()

	return rl
}

// Allow checks if a request should be allowed based on fingerprint
func (rl *RateLimiter) Allow(fingerprint string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	bucket, exists := rl.buckets[fingerprint]

	if !exists {
		rl.buckets[fingerprint] = &tokenBucket{
			tokens:     float64(rl.config.Burst),
			lastUpdate: now,
		}
		return true
	}

	// Add tokens based on time elapsed
	elapsed := now.Sub(bucket.lastUpdate).Seconds()
	bucket.tokens += elapsed * (float64(rl.config.RequestsPerMinute) / 60.0)

	if bucket.tokens > float64(rl.config.Burst) {
		bucket.tokens = float64(rl.config.Burst)
	}

	bucket.lastUpdate = now

	if bucket.tokens >= 1 {
		bucket.tokens--
		return true
	}

	return false
}

// Middleware creates HTTP middleware for rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fingerprint := rl.generateFingerprint(r)

		if !rl.Allow(fingerprint) {
			w.Header().Set("Retry-After", "60")
			request.WriteJSON(w, http.StatusTooManyRequests, request.Fail(
				"RATE_LIMIT_EXCEEDED",
				"Too many requests. Please try again later.",
				nil,
			))
			return
		}

		next.ServeHTTP(w, r)
	})
}

// generateFingerprint creates a unique fingerprint for the request
func (rl *RateLimiter) generateFingerprint(r *http.Request) string {
	// Combine IP, User-Agent, and Tenant-ID for fingerprinting
	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = forwarded
	}

	userAgent := r.Header.Get("User-Agent")
	tenantID := r.Header.Get("X-Tenant-ID")
	if tenantID == "" {
		// Try to get from context if available
		if tid := r.Context().Value(TenantIDKey); tid != nil {
			tenantID = string(tid.(string))
		}
	}

	// Create hash from combined values
	hash := sha256.Sum256([]byte(ip + userAgent + tenantID))
	return hex.EncodeToString(hash[:])
}

// cleanup removes old buckets to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for fingerprint, bucket := range rl.buckets {
				if now.Sub(bucket.lastUpdate) > 30*time.Minute {
					delete(rl.buckets, fingerprint)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopChan:
			return
		}
	}
}

// Stop stops the cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopChan)
}

// GetFingerprint extracts or generates fingerprint from request
func GetFingerprint(r *http.Request) string {
	ip := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = forwarded
	}

	userAgent := r.Header.Get("User-Agent")
	tenantID := r.Header.Get("X-Tenant-ID")

	hash := sha256.Sum256([]byte(ip + userAgent + tenantID))
	return hex.EncodeToString(hash[:])
}

// contextKey for tenant ID
type contextKey string

const (
	TenantIDKey contextKey = "tenant_id"
)
