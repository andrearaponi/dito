package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

// TestMain initializes metrics and runs the tests.
func TestMain(m *testing.M) {
	InitMetrics()
	m.Run()
}

// TestNormalizePath tests the NormalizePath function with various inputs.
func TestNormalizePath(t *testing.T) {
	assert.Equal(t, "/users/:id", NormalizePath("/users/123"))
	assert.Equal(t, "/orders/:id/items/:id", NormalizePath("/orders/456/items/789"))
	assert.Equal(t, "/static/path", NormalizePath("/static/path"))
}

// TestRecordRequest tests the RecordRequest function for recording HTTP requests.
func TestRecordRequest(t *testing.T) {
	RecordRequest("GET", "/users/123", http.StatusOK, 0.123)
	metric := &io_prometheus_client.Metric{}
	if err := httpRequestsTotal.WithLabelValues("GET", "/users/:id", "OK").Write(metric); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	assert.Equal(t, 1, int(metric.GetCounter().GetValue()))
}

// TestRecordDataTransferred tests the RecordDataTransferred function for recording data transfer.
func TestRecordDataTransferred(t *testing.T) {
	RecordDataTransferred("inbound", 1024)
	metric := &io_prometheus_client.Metric{}
	if err := dataTransferred.WithLabelValues("inbound").Write(metric); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	assert.Equal(t, 1024, int(metric.GetCounter().GetValue()))
}

// TestUpdateActiveConnections tests the UpdateActiveConnections function for updating active connections.
func TestUpdateActiveConnections(t *testing.T) {
	UpdateActiveConnections(true)
	metric := &io_prometheus_client.Metric{}
	if err := activeConnections.Write(metric); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	assert.Equal(t, 1, int(metric.GetGauge().GetValue()))

	UpdateActiveConnections(false)
	if err := activeConnections.Write(metric); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	assert.Equal(t, 0, int(metric.GetGauge().GetValue()))
}

// TestExposeMetricsHandler tests the ExposeMetricsHandler function for exposing metrics via HTTP.
func TestExposeMetricsHandler(t *testing.T) {
	req, _ := http.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	handler := ExposeMetricsHandler()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "http_requests_total")
}
