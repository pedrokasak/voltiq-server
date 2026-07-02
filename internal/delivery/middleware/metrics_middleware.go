package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// MetricsMiddleware tracks request metrics
type MetricsMiddleware struct {
	requestCount   map[string]map[string]int64
	requestLatency map[string]map[string][]time.Duration
}

// NewMetricsMiddleware creates a new MetricsMiddleware
func NewMetricsMiddleware() *MetricsMiddleware {
	return &MetricsMiddleware{
		requestCount:   make(map[string]map[string]int64),
		requestLatency: make(map[string]map[string][]time.Duration),
	}
}

// Handler wraps a http.Handler with metrics tracking
func (m *MetricsMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Use chi's route pattern if available
		routePattern := chi.RouteContext(r.Context()).RoutePattern()
		if routePattern == "" {
			routePattern = r.URL.Path
		}

		method := r.Method

		next.ServeHTTP(w, r)

		// Track metrics
		duration := time.Since(start)
		
		if m.requestCount[method] == nil {
			m.requestCount[method] = make(map[string]int64)
			m.requestLatency[method] = make(map[string][]time.Duration)
		}
		
		m.requestCount[method][routePattern]++
		m.requestLatency[method][routePattern] = append(m.requestLatency[method][routePattern], duration)
	})
}

// GetRequestCount returns the count of requests for a method and route
func (m *MetricsMiddleware) GetRequestCount(method, route string) int64 {
	if counts, ok := m.requestCount[method]; ok {
		return counts[route]
	}
	return 0
}

// GetMetrics returns all metrics in Prometheus format
func (m *MetricsMiddleware) GetMetrics() string {
	var result string
	
	result += "# HELP api_requests_total Total number of API requests\n"
	result += "# TYPE api_requests_total counter\n"
	
	for method, routes := range m.requestCount {
		for route, count := range routes {
			result += "api_requests_total{method=\"" + method + "\",endpoint=\"" + route + "\"} " + strconv.FormatInt(count, 10) + "\n"
		}
	}
	
	return result
}
