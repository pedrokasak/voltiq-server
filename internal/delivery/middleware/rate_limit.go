package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"strings"
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
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	config   RateLimiterConfig
	name     string
	stopChan chan struct{}
	stopOnce sync.Once
}

type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerMinute, burst int) *RateLimiter {
	return NewNamedRateLimiter("default", requestsPerMinute, burst)
}

// NewNamedRateLimiter creates a rate limiter for one endpoint category; the
// name is part of the fingerprint so categories never share buckets.
func NewNamedRateLimiter(name string, requestsPerMinute, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*tokenBucket),
		config:   RateLimiterConfig{RequestsPerMinute: requestsPerMinute, Burst: burst},
		name:     name,
		stopChan: make(chan struct{}),
	}
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
		rl.buckets[fingerprint] = &tokenBucket{tokens: float64(rl.config.Burst) - 1, lastUpdate: now}
		return true
	}

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

// retryAfter is the number of seconds until one token is available.
func (rl *RateLimiter) retryAfter() int {
	if rl.config.RequestsPerMinute <= 0 {
		return 60
	}
	return max(1, 60/rl.config.RequestsPerMinute)
}

// Middleware creates HTTP middleware for rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(rl.name + ":" + GetFingerprint(r)) {
			w.Header().Set("Retry-After", strconv.Itoa(rl.retryAfter()))
			request.WriteJSON(w, http.StatusTooManyRequests, request.Fail(
				"RATE_LIMIT_EXCEEDED",
				"muitas requisições; aguarde alguns instantes e tente novamente",
				nil,
			))
			return
		}
		next.ServeHTTP(w, r)
	})
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
	rl.stopOnce.Do(func() { close(rl.stopChan) })
}

// ClientIP returns the client address. Behind Fly.io the edge sets
// Fly-Client-IP; X-Forwarded-For is only trusted for its first hop when
// TRUST_PROXY is enabled by the caller (see trustProxy). The port is removed
// so that every connection of a client shares the bucket.
func ClientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("Fly-Client-IP")); ip != "" {
		return ip
	}
	if trustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			return strings.TrimSpace(strings.Split(fwd, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// trustProxy enables X-Forwarded-For (set by SetTrustProxy at startup).
var trustProxy bool

// SetTrustProxy configures whether X-Forwarded-For is trusted.
func SetTrustProxy(v bool) { trustProxy = v }

// GetFingerprint returns SHA256(IP + User-Agent + tenant) for the request;
// the tenant comes from the authenticated context when available.
func GetFingerprint(r *http.Request) string {
	tenantID := string(GetTenantID(r.Context()))
	hash := sha256.Sum256([]byte(ClientIP(r) + "|" + r.Header.Get("User-Agent") + "|" + tenantID))
	return hex.EncodeToString(hash[:])
}

// RateLimits groups the per-category limiters (SDD seção de segurança):
// login 5/min, refresh 10/min, imports 20/min, balanco 30/min, demais 60/min.
type RateLimits struct {
	Login   *RateLimiter
	Refresh *RateLimiter
	Import  *RateLimiter
	Balance *RateLimiter
	Default *RateLimiter
}

// NewRateLimits creates the standard limiters.
func NewRateLimits() *RateLimits {
	return &RateLimits{
		Login:   NewNamedRateLimiter("login", 5, 3),
		Refresh: NewNamedRateLimiter("refresh", 10, 5),
		Import:  NewNamedRateLimiter("import", 20, 10),
		Balance: NewNamedRateLimiter("balance", 30, 15),
		Default: NewNamedRateLimiter("default", 60, 30),
	}
}

// Stop stops all limiters.
func (l *RateLimits) Stop() {
	for _, rl := range []*RateLimiter{l.Login, l.Refresh, l.Import, l.Balance, l.Default} {
		rl.Stop()
	}
}
