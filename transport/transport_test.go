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
// TestCreateCustomTransport verifies that the custom transport is correctly created with certificates.
func TestCreateCustomTransport(t *testing.T) {
	// Creiamo una LocationConfig di esempio con i percorsi dei certificati.
	location := &config.LocationConfig{
		CertFile: "testdata/test_cert.pem",
		KeyFile:  "testdata/test_key.pem",
		CaFile:   "testdata/test_ca.pem",
	}

	// Creiamo un'istanza di Caronte con una configurazione fittizia.
	caronte := &transport.Caronte{
		RT:       http.DefaultTransport,
		Location: location,
	}

	// Simuliamo la creazione del trasporto.
	customTransport, err := caronte.CreateCustomTransport()
	assert.NoError(t, err)
	assert.NotNil(t, customTransport)

	// Verifichiamo che il TLSClientConfig sia impostato correttamente.
	// Qui non facciamo una type assertion, ma accediamo direttamente.
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

	// Creiamo un'istanza di Caronte.
	caronte := &transport.Caronte{
		Location: location,
	}

	// Simuliamo una richiesta HTTP.
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	req.Header.Set("X-Remove-Header", "RemoveMe")

	// Applichiamo le modifiche agli header.
	caronte.AddHeaders(req)

	// Verifichiamo che l'header "X-Custom-Header" sia stato aggiunto.
	assert.Equal(t, "CustomValue", req.Header.Get("X-Custom-Header"))

	// Verifichiamo che l'header "X-Remove-Header" sia stato rimosso.
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
