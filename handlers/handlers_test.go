package handlers_test

import (
	"bytes"
	"dito/app"
	"dito/config"
	"dito/handlers"
	"dito/logging"
	"dito/plugin"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// setupTestConfig initializes a sample configuration that proxies /test to targetURL.
func setupTestConfig(targetURL string) *config.ProxyConfig {
	cfg := &config.ProxyConfig{
		Port: "8080",
		Logging: config.Logging{
			Enabled: true,
			Verbose: false,
			Level:   "info",
		},
		Locations: []config.LocationConfig{
			{
				Path:      "/test",
				TargetURL: targetURL,
			},
		},
	}

	// Compile regular expressions for each location.
	for i, location := range cfg.Locations {
		regex, err := regexp.Compile(location.Path)
		if err != nil {
			panic(err)
		}
		cfg.Locations[i].CompiledRegex = regex
	}

	return cfg
}

// setupDito creates an instance of Dito for testing purposes.
func setupDito() *app.Dito {
	// Initialize the logger.
	logger := logging.InitializeLogger("info")

	// Create a sample HTTPTransportConfig.
	httpTransportConfig := &config.HTTPTransportConfig{
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       100,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
		ForceHTTP2:            true,
		DialTimeout:           30 * time.Second,
		KeepAlive:             30 * time.Second,
		CertFile:              "testdata/test_cert.pem",
		KeyFile:               "testdata/test_key.pem",
		CaFile:                "testdata/test_ca.pem",
	}

	// Create a new Dito instance.
	dito := app.NewDito(httpTransportConfig, logger)
	if dito == nil {
		panic("Failed to initialize Dito instance")
	}
	return dito
}

func TestDynamicProxyHandler(t *testing.T) {
	// Loopback backend owned by the test: the suite must not depend on external hosts.
	type receivedRequest struct {
		method string
		path   string
	}
	var (
		mu       sync.Mutex
		received []receivedRequest
	)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, receivedRequest{method: r.Method, path: r.URL.Path})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	// Set up the configuration and Dito instance.
	config.UpdateConfig(setupTestConfig(backend.URL))
	dito := setupDito()

	// Create a request to test the handler.
	req, err := http.NewRequest("GET", "/test", nil)
	assert.NoError(t, err)

	// Create a ResponseRecorder to capture the response.
	rr := httptest.NewRecorder()

	if dito == nil {
		t.Fatal("Dito instance is nil")
	}
	req.Body = io.NopCloser(bytes.NewBufferString("Test body"))

	// Call the handler with an empty slice of plugins.
	handlers.DynamicProxyHandler(dito, rr, req, []plugin.Plugin{})

	// Check that the status code is what you expect.
	assert.Equal(t, http.StatusOK, rr.Code)

	// The proxied request must have reached the loopback backend, exactly once.
	mu.Lock()
	defer mu.Unlock()
	if assert.Len(t, received, 1, "the loopback backend must receive exactly one proxied request") {
		assert.Equal(t, http.MethodGet, received[0].method)
		assert.Equal(t, "/", received[0].path)
	}
}

// TestNewHandler checks the handler shared by the binary and the e2e harness:
// a request goes through it to the backend and the client gets its body.
func TestNewHandler(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello from backend")
	}))
	t.Cleanup(backend.Close)

	config.UpdateConfig(setupTestConfig(backend.URL))
	server := httptest.NewServer(handlers.NewHandler(setupDito(), nil))
	t.Cleanup(server.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/test", nil)
	if !assert.NoError(t, err) {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "hello from backend", string(body))
}
