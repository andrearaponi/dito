package metrics

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Define Prometheus metrics
var (
	// Existing metrics
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed, partitioned by method, path, and status code.",
		},
		[]string{"method", "normalized_path", "status_code"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "normalized_path", "status_code"},
	)

	dataTransferred = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "data_transferred_bytes_total",
			Help: "Total amount of data transferred in bytes, partitioned by direction (inbound or outbound).",
		},
		[]string{"direction"},
	)

	activeConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_connections",
			Help: "Number of active connections currently being handled by the proxy.",
		},
	)

	// New metrics for production-ready handler
	proxyErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxy_errors_total",
			Help: "Total number of proxy errors, partitioned by path and error type.",
		},
		[]string{"normalized_path", "error_type"},
	)

	responseLimitExceeded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "response_limit_exceeded_total",
			Help: "Total number of responses that exceeded size limits.",
		},
		[]string{"normalized_path", "limit_bytes"},
	)

	panicsRecovered = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "panics_recovered_total",
			Help: "Total number of panics recovered in handlers.",
		},
		[]string{"normalized_path"},
	)

	securityBlocks = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "security_blocks_total",
			Help: "Total number of requests blocked for security reasons.",
		},
		[]string{"normalized_path", "reason"},
	)

	activeRequests = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_requests",
			Help: "Number of requests currently being processed.",
		},
	)

	requestBodySize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "request_body_bytes",
			Help:    "Size of request bodies in bytes.",
			Buckets: []float64{100, 1000, 10000, 100000, 1000000, 10000000}, // 100B to 10MB
		},
		[]string{"method", "normalized_path"},
	)

	responseBodySize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "response_body_bytes",
			Help:    "Size of response bodies in bytes.",
			Buckets: []float64{100, 1000, 10000, 100000, 1000000, 10000000, 100000000}, // 100B to 100MB
		},
		[]string{"method", "normalized_path", "status_code"},
	)

	upstreamResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "upstream_response_seconds",
			Help:    "Time taken by upstream servers to respond.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"upstream_host", "normalized_path"},
	)

	rateLimitHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_hits_total",
			Help: "Total number of requests that hit rate limits.",
		},
		[]string{"normalized_path", "limit_type"},
	)

	websocketConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "websocket_connections_active",
			Help: "Number of active WebSocket connections.",
		},
		[]string{"normalized_path"},
	)

	middlewareExecutionTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "middleware_execution_seconds",
			Help:    "Time taken by middleware execution.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"middleware_name", "normalized_path"},
	)
)

// InitMetrics registers all metrics with Prometheus
func InitMetrics() {
	// Register existing metrics
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(dataTransferred)
	prometheus.MustRegister(activeConnections)

	// Register new metrics
	prometheus.MustRegister(proxyErrors)
	prometheus.MustRegister(responseLimitExceeded)
	prometheus.MustRegister(panicsRecovered)
	prometheus.MustRegister(securityBlocks)
	prometheus.MustRegister(activeRequests)
	prometheus.MustRegister(requestBodySize)
	prometheus.MustRegister(responseBodySize)
	prometheus.MustRegister(upstreamResponseTime)
	prometheus.MustRegister(rateLimitHits)
	prometheus.MustRegister(websocketConnections)
	prometheus.MustRegister(middlewareExecutionTime)
}

// NormalizePath normalizes dynamic paths (e.g., "/users/123" -> "/users/:id")
func NormalizePath(path string) string {
	// UUID pattern
	uuidRegex := regexp.MustCompile(`[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`)
	path = uuidRegex.ReplaceAllString(path, ":uuid")

	// Numeric ID pattern
	numericRegex := regexp.MustCompile(`\b\d+\b`)
	path = numericRegex.ReplaceAllString(path, ":id")

	// Hex strings (like MongoDB ObjectIds)
	hexRegex := regexp.MustCompile(`\b[a-fA-F0-9]{24}\b`)
	path = hexRegex.ReplaceAllString(path, ":hex_id")

	return path
}

// RecordRequest records metrics for each request
func RecordRequest(method, path string, statusCode int, duration float64) {
	normalizedPath := NormalizePath(path)
	statusCodeStr := http.StatusText(statusCode)

	httpRequestsTotal.WithLabelValues(method, normalizedPath, statusCodeStr).Inc()
	httpRequestDuration.WithLabelValues(method, normalizedPath, statusCodeStr).Observe(duration)
}

// RecordDataTransferred records the number of bytes transferred
func RecordDataTransferred(direction string, numBytes int) {
	if numBytes > 0 {
		dataTransferred.WithLabelValues(direction).Add(float64(numBytes))
	}
}

// UpdateActiveConnections increments or decrements the number of active connections
func UpdateActiveConnections(increment bool) {
	if increment {
		activeConnections.Inc()
	} else {
		activeConnections.Dec()
	}
}

// UpdateActiveRequests increments or decrements the number of active requests
func UpdateActiveRequests(increment bool) {
	if increment {
		activeRequests.Inc()
	} else {
		activeRequests.Dec()
	}
}

// RecordProxyError records proxy errors with categorization
func RecordProxyError(path string, err error) {
	normalizedPath := NormalizePath(path)
	errorType := categorizeError(err)
	proxyErrors.WithLabelValues(normalizedPath, errorType).Inc()
}

// RecordResponseLimitExceeded records when response size limits are exceeded
func RecordResponseLimitExceeded(path string, limitBytes int64) {
	normalizedPath := NormalizePath(path)
	limitStr := formatBytes(limitBytes)
	responseLimitExceeded.WithLabelValues(normalizedPath, limitStr).Inc()
}

// RecordPanic records recovered panics
func RecordPanic(path string) {
	normalizedPath := NormalizePath(path)
	panicsRecovered.WithLabelValues(normalizedPath).Inc()
}

// RecordSecurityBlock records security-related blocks
func RecordSecurityBlock(path string, reason string) {
	normalizedPath := NormalizePath(path)
	securityBlocks.WithLabelValues(normalizedPath, reason).Inc()
}

// RecordRequestBodySize records the size of request bodies
func RecordRequestBodySize(method, path string, size int64) {
	if size > 0 {
		normalizedPath := NormalizePath(path)
		requestBodySize.WithLabelValues(method, normalizedPath).Observe(float64(size))
	}
}

// RecordResponseBodySize records the size of response bodies
func RecordResponseBodySize(method, path string, statusCode int, size int64) {
	if size > 0 {
		normalizedPath := NormalizePath(path)
		statusCodeStr := http.StatusText(statusCode)
		responseBodySize.WithLabelValues(method, normalizedPath, statusCodeStr).Observe(float64(size))
	}
}

// RecordUpstreamResponseTime records the time taken by upstream servers
func RecordUpstreamResponseTime(upstreamHost, path string, duration float64) {
	normalizedPath := NormalizePath(path)
	upstreamResponseTime.WithLabelValues(upstreamHost, normalizedPath).Observe(duration)
}

// RecordRateLimitHit records when rate limits are hit
func RecordRateLimitHit(path string, limitType string) {
	normalizedPath := NormalizePath(path)
	rateLimitHits.WithLabelValues(normalizedPath, limitType).Inc()
}

// UpdateWebSocketConnections updates the count of active WebSocket connections
func UpdateWebSocketConnections(path string, increment bool) {
	normalizedPath := NormalizePath(path)
	if increment {
		websocketConnections.WithLabelValues(normalizedPath).Inc()
	} else {
		websocketConnections.WithLabelValues(normalizedPath).Dec()
	}
}

// RecordMiddlewareExecutionTime records the time taken by middleware
func RecordMiddlewareExecutionTime(middlewareName, path string, duration float64) {
	normalizedPath := NormalizePath(path)
	middlewareExecutionTime.WithLabelValues(middlewareName, normalizedPath).Observe(duration)
}

// ExposeMetricsHandler returns a handler that serves the metrics for Prometheus
func ExposeMetricsHandler() http.Handler {
	return promhttp.Handler()
}

// Helper functions

// categorizeError categorizes errors for better metrics
func categorizeError(err error) string {
	if err == nil {
		return "unknown"
	}

	errStr := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errStr, "timeout"):
		return "timeout"
	case strings.Contains(errStr, "connection refused"):
		return "connection_refused"
	case strings.Contains(errStr, "no such host"):
		return "dns_error"
	case strings.Contains(errStr, "tls"):
		return "tls_error"
	case strings.Contains(errStr, "context canceled"):
		return "cancelled"
	case strings.Contains(errStr, "eof"):
		return "eof"
	default:
		return "other"
	}
}

// formatBytes formats bytes into human-readable buckets for metrics
func formatBytes(bytes int64) string {
	switch {
	case bytes < 1024:
		return "< 1KB"
	case bytes < 10*1024:
		return "1KB-10KB"
	case bytes < 100*1024:
		return "10KB-100KB"
	case bytes < 1024*1024:
		return "100KB-1MB"
	case bytes < 10*1024*1024:
		return "1MB-10MB"
	case bytes < 100*1024*1024:
		return "10MB-100MB"
	default:
		return "> 100MB"
	}
}
