package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"regexp"
)

// Define Prometheus metrics
var (
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
)

func InitMetrics() {
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(dataTransferred)
	prometheus.MustRegister(activeConnections)
}

// NormalizePath normalizes dynamic paths (e.g., "/users/123" -> "/users/:id")
func NormalizePath(path string) string {
	// A simple normalization logic for paths (this can be customized or use regex)
	re := regexp.MustCompile(`\d+`)
	normalizedPath := re.ReplaceAllString(path, ":id")
	return normalizedPath
}

// RecordRequest records metrics for each request
func RecordRequest(method, path string, statusCode int, duration float64) {
	normalizedPath := NormalizePath(path)
	statusCodeStr := http.StatusText(statusCode)

	httpRequestsTotal.WithLabelValues(method, normalizedPath, statusCodeStr).Inc()
	httpRequestDuration.WithLabelValues(method, normalizedPath, statusCodeStr).Observe(duration)
}

// RecordDataTransferred records the number of bytes transferred, partitioned by direction (inbound or outbound)
func RecordDataTransferred(direction string, numBytes int) {
	dataTransferred.WithLabelValues(direction).Add(float64(numBytes))
}

// UpdateActiveConnections increments or decrements the number of active connections
func UpdateActiveConnections(increment bool) {
	if increment {
		activeConnections.Inc()
	} else {
		activeConnections.Dec()
	}
}

// ExposeMetricsHandler returns a handler that serves the metrics for Prometheus
func ExposeMetricsHandler() http.Handler {
	return promhttp.Handler()
}
