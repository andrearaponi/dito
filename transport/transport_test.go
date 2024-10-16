package transport_test

import (
	"dito/config"
	"dito/transport"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

func setupTestConfig() {
	// Define a sample configuration.
	testConfig := &config.ProxyConfig{
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
	// Update the global configuration with this test configuration.
	config.UpdateConfig(testConfig)
}

// TestCreateCustomTransport verifies that the custom transport is correctly created with certificates.
func TestCreateCustomTransport(t *testing.T) {
	location := &config.LocationConfig{
		CertFile: "testdata/test_cert.pem",
		KeyFile:  "testdata/test_key.pem",
		CaFile:   "testdata/test_ca.pem",
	}

	caronte := &transport.Caronte{
		RT:       http.DefaultTransport,
		Location: location,
	}

	customTransport, err := caronte.CreateCustomTransport()
	assert.NoError(t, err)
	assert.NotNil(t, customTransport)

	tlsTransport := customTransport
	assert.NotNil(t, tlsTransport.TLSClientConfig)
	assert.Len(t, tlsTransport.TLSClientConfig.Certificates, 1)
}

// TestAddHeaders verifies that headers are correctly added to the request.
func TestAddHeaders(t *testing.T) {
	location := &config.LocationConfig{
		AdditionalHeaders: map[string]string{
			"X-Custom-Header": "CustomValue",
		},
		ExcludedHeaders: []string{"X-Remove-Header"},
	}

	caronte := &transport.Caronte{
		Location: location,
	}

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	req.Header.Set("X-Remove-Header", "RemoveMe")

	caronte.AddHeaders(req)

	assert.Equal(t, "CustomValue", req.Header.Get("X-Custom-Header"))

	assert.Empty(t, req.Header.Get("X-Remove-Header"))
}

// TestRoundTrip simulates a RoundTrip and checks if headers and certificates are handled correctly.
func TestRoundTrip(t *testing.T) {
	setupTestConfig()

	location := &config.LocationConfig{
		Path: "/test",
		AdditionalHeaders: map[string]string{
			"X-Test-Header": "test-value",
		},
	}

	caronte := &transport.Caronte{
		RT:       http.DefaultTransport,
		Location: location,
	}

	req, err := http.NewRequest("GET", "http://localhost/test", nil)
	assert.NoError(t, err)

	resp, err := caronte.RoundTrip(req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}
