package config_test

import (
	"dito/config"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestLoadConfiguration verifies that a valid configuration file is correctly loaded.
func TestLoadConfiguration(t *testing.T) {
	content := `
port: "8080"
hot_reload: true
logging:
  enabled: true
  verbose: false
  level: "INFO"
response_limits:
  max_response_body_size: 52428800
redis:
  host: "localhost"
  port: "6379"
  password: "secret"
locations:
  - path: "/api"
    target_url: "http://backend:8000"
    replace_path: true
    max_response_body_size: 10485760
    middlewares: ["auth", "rate-limiter"]
    rate_limiting:
      enabled: true
      requests_per_second: 5
      burst: 10
`

	file, err := os.CreateTemp("", "config_test_*.yaml")
	assert.NoError(t, err)
	defer os.Remove(file.Name())

	_, err = file.Write([]byte(content))
	assert.NoError(t, err)

	loadedConfig, err := config.LoadConfiguration(file.Name())
	assert.NoError(t, err)
	assert.Equal(t, "8080", loadedConfig.Port)
	assert.True(t, loadedConfig.HotReload)
	assert.Equal(t, int64(52428800), loadedConfig.ResponseLimits.MaxResponseBodySize)
	assert.Equal(t, 1, len(loadedConfig.Locations))
	assert.Equal(t, "/api", loadedConfig.Locations[0].Path)
	assert.Equal(t, int64(10485760), loadedConfig.Locations[0].MaxResponseBodySize)
}

// TestLoadConfigurationWithDefaults verifies that default values are set when not specified.
func TestLoadConfigurationWithDefaults(t *testing.T) {
	content := `
port: "8080"
hot_reload: true
logging:
  enabled: true
  verbose: false
  level: "INFO"
locations:
  - path: "/api"
    target_url: "http://backend:8000"
    replace_path: true
`

	file, err := os.CreateTemp("", "config_test_defaults_*.yaml")
	assert.NoError(t, err)
	defer os.Remove(file.Name())

	_, err = file.Write([]byte(content))
	assert.NoError(t, err)

	loadedConfig, err := config.LoadConfiguration(file.Name())
	assert.NoError(t, err)

	// Should have default 100MB limit
	assert.Equal(t, int64(100*1024*1024), loadedConfig.ResponseLimits.MaxResponseBodySize)

	// Location should not have specific limit (should be 0, meaning use global)
	assert.Equal(t, int64(0), loadedConfig.Locations[0].MaxResponseBodySize)
}

// TestValidateResponseBodySizeLimits tests validation of response body size limits.
func TestValidateResponseBodySizeLimits(t *testing.T) {
	testCases := []struct {
		name        string
		content     string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid global limit",
			content: `
port: "8080"
response_limits:
  max_response_body_size: 1048576
locations:
  - path: "/api"
    target_url: "http://backend:8000"
`,
			expectError: false,
		},
		{
			name: "valid location limit",
			content: `
port: "8080"
response_limits:
  max_response_body_size: 1048576
locations:
  - path: "/api"
    target_url: "http://backend:8000"
    max_response_body_size: 2097152
`,
			expectError: false,
		},
		{
			name: "unlimited location limit (0)",
			content: `
port: "8080"
response_limits:
  max_response_body_size: 1048576
locations:
  - path: "/api"
    target_url: "http://backend:8000"
    max_response_body_size: 0
`,
			expectError: false,
		},
		{
			name: "negative global limit",
			content: `
port: "8080"
response_limits:
  max_response_body_size: -1
locations:
  - path: "/api"
    target_url: "http://backend:8000"
`,
			expectError: true,
			errorMsg:    "global max_response_body_size cannot be negative",
		},
		{
			name: "negative location limit",
			content: `
port: "8080"
response_limits:
  max_response_body_size: 1048576
locations:
  - path: "/api"
    target_url: "http://backend:8000"
    max_response_body_size: -1
`,
			expectError: true,
			errorMsg:    "location '/api': max_response_body_size cannot be negative",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := os.CreateTemp("", "config_validation_*.yaml")
			assert.NoError(t, err)
			defer os.Remove(file.Name())

			_, err = file.Write([]byte(tc.content))
			assert.NoError(t, err)

			_, err = config.LoadConfiguration(file.Name())
			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGetEffectiveMaxResponseBodySize tests the method to get effective response body size limits.
func TestGetEffectiveMaxResponseBodySize(t *testing.T) {
	globalLimit := int64(1048576) // 1MB

	testCases := []struct {
		name           string
		locationLimit  int64 // Change to int64
		expectedResult int64
	}{
		{
			name:           "location limit 0 - use global", // Renamed for clarity
			locationLimit:  0,                               // Use 0 for location limit
			expectedResult: globalLimit,                     // Expected result is global limit based on current logic
		},
		{
			name:           "location limit set (positive) - use location", // Renamed for clarity
			locationLimit:  2097152,                                        // 2MB
			expectedResult: 2097152,
		},
		// Note: The current GetEffectiveMaxResponseBodySize treats negative limits
		// the same as 0 (uses global). While validation should prevent negative
		// limits, the method's behavior for negative input would also be globalLimit.
		// We won't add a test for negative here as validation is tested elsewhere.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			location := config.LocationConfig{}
			location.MaxResponseBodySize = tc.locationLimit // Assignment now works with int64

			result := location.GetEffectiveMaxResponseBodySize(globalLimit)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

// TestUpdateConfig verifies that UpdateConfig correctly updates the configuration.
func TestUpdateConfig(t *testing.T) {
	initialConfig := &config.ProxyConfig{
		Port: "8080",
		Logging: config.Logging{
			Enabled: false,
		},
		ResponseLimits: config.ResponseLimits{
			MaxResponseBodySize: 1048576,
		},
	}
	config.UpdateConfig(initialConfig)
	assert.Equal(t, initialConfig, config.GetCurrentProxyConfig())

	updatedConfig := &config.ProxyConfig{
		Port: "9090",
		Logging: config.Logging{
			Enabled: true,
		},
		ResponseLimits: config.ResponseLimits{
			MaxResponseBodySize: 2097152,
		},
	}
	config.UpdateConfig(updatedConfig)
	assert.Equal(t, updatedConfig, config.GetCurrentProxyConfig())
}

// TestIsConfigDifferent verifies the behavior of isConfigDifferent.
func TestIsConfigDifferent(t *testing.T) {
	config1 := &config.ProxyConfig{
		Port: "8080",
		ResponseLimits: config.ResponseLimits{
			MaxResponseBodySize: 1048576,
		},
	}
	config2 := &config.ProxyConfig{
		Port: "8081",
		ResponseLimits: config.ResponseLimits{
			MaxResponseBodySize: 1048576,
		},
	}
	assert.True(t, config.IsConfigDifferent(config1, config2))

	config2.Port = "8080"
	assert.False(t, config.IsConfigDifferent(config1, config2))

	// Test difference in response limits
	config2.ResponseLimits.MaxResponseBodySize = 2097152
	assert.True(t, config.IsConfigDifferent(config1, config2))
}

// TestWatchConfig verifies that WatchConfig invokes the callback on configuration changes.
func TestWatchConfig(t *testing.T) {
	content := `
port: "8080"
response_limits:
  max_response_body_size: 1048576
`
	file, err := os.CreateTemp("", "config_watch_test_*.yaml")
	assert.NoError(t, err)
	defer os.Remove(file.Name())

	_, err = file.Write([]byte(content))
	assert.NoError(t, err)

	initialConfig, err := config.LoadConfiguration(file.Name())
	assert.NoError(t, err)
	config.UpdateConfig(initialConfig)

	callbackInvoked := false
	callback := func(newConfig *config.ProxyConfig) {
		callbackInvoked = true
	}

	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	go config.WatchConfig(file.Name(), callback, testLogger)

	time.Sleep(3 * time.Second)

	updatedContent := `
port: "9090"
response_limits:
  max_response_body_size: 2097152
`
	err = os.WriteFile(file.Name(), []byte(updatedContent), 0644)
	assert.NoError(t, err)

	time.Sleep(3 * time.Second)
	assert.True(t, callbackInvoked)
}
