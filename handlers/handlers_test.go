package handlers_test

import (
	"bytes"
	"dito/app"
	"dito/config"
	"dito/handlers"
	"dito/logging"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
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

	// Compila le espressioni regolari per ogni location.
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
	// Setup Redis client (mocked or real as needed).
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})

	// Initialize the logger.
	logger := logging.InitializeLogger("info")

	// Create a new Dito instance.
	return app.NewDito(redisClient, logger)
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

	// Create a dummy request body.
	req.Body = io.NopCloser(bytes.NewBufferString("Test body"))

	// Call the handler.
	handlers.DynamicProxyHandler(dito, rr, req)

	// Check that the status code is what you expect.
	assert.Equal(t, http.StatusOK, rr.Code)
}
