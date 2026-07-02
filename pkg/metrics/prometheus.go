package metrics

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// MetricsCollector collects and exposes metrics in Prometheus format
type MetricsCollector struct {
	mu             sync.RWMutex
	requestCount   map[string]map[string]int64
	requestLatency map[string]map[string][]float64
	startTime      time.Time
}

// NewMetricsCollector creates a new MetricsCollector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		requestCount:   make(map[string]map[string]int64),
		requestLatency: make(map[string]map[string][]float64),
		startTime:      time.Now(),
	}
}

// RecordRequest records a request metric
func (m *MetricsCollector) RecordRequest(method, route string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.requestCount[method] == nil {
		m.requestCount[method] = make(map[string]int64)
		m.requestLatency[method] = make(map[string][]float64)
	}

	m.requestCount[method][route]++
	m.requestLatency[method][route] = append(m.requestLatency[method][route], duration.Seconds())
}

// GetMetrics returns metrics in Prometheus format
func (m *MetricsCollector) GetMetrics() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result string

	// API requests total
	result += "# HELP api_requests_total Total number of API requests\n"
	result += "# TYPE api_requests_total counter\n"
	for method, routes := range m.requestCount {
		for route, count := range routes {
			result += fmt.Sprintf("api_requests_total{method=\"%s\",endpoint=\"%s\"} %d\n", method, route, count)
		}
	}

	// API request latency
	result += "# HELP api_request_latency_seconds API request latency in seconds\n"
	result += "# TYPE api_request_latency_seconds histogram\n"
	for method, routes := range m.requestLatency {
		for route, latencies := range routes {
			if len(latencies) > 0 {
				avg := 0.0
				for _, l := range latencies {
					avg += l
				}
				avg /= float64(len(latencies))
				result += fmt.Sprintf("api_request_latency_seconds{method=\"%s\",endpoint=\"%s\"} %.6f\n", method, route, avg)
			}
		}
	}

	// Uptime
	result += "# HELP api_uptime_seconds API uptime in seconds\n"
	result += "# TYPE api_uptime_seconds gauge\n"
	result += fmt.Sprintf("api_uptime_seconds %.0f\n", time.Since(m.startTime).Seconds())

	return result
}

// ServeHTTP implements http.Handler for metrics endpoint
func (m *MetricsCollector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Write([]byte(m.GetMetrics()))
}

// GetRequestCount returns the count of requests for a method and route
func (m *MetricsCollector) GetRequestCount(method, route string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if counts, ok := m.requestCount[method]; ok {
		return counts[route]
	}
	return 0
}

// GetAverageLatency returns the average latency for a method and route
func (m *MetricsCollector) GetAverageLatency(method, route string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if latencies, ok := m.requestLatency[method]; ok {
		if lats, ok := latencies[route]; ok && len(lats) > 0 {
			sum := 0.0
			for _, l := range lats {
				sum += l
			}
			return sum / float64(len(lats))
		}
	}
	return 0
}

// Reset resets all metrics
func (m *MetricsCollector) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requestCount = make(map[string]map[string]int64)
	m.requestLatency = make(map[string]map[string][]float64)
	m.startTime = time.Now()
}
