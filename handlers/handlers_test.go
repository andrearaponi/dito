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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// setupTestConfig initializes a sample configuration for testing.
func setupTestConfig() *config.ProxyConfig {
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
				TargetURL: "http://example.com",
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
	// Set up the configuration and Dito instance.
	config.UpdateConfig(setupTestConfig())
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
}
