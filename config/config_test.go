package config_test

import (
	"dito/config"
	"github.com/stretchr/testify/assert"
	"log/slog"
	"os"
	"testing"
	"time"
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
redis:
  host: "localhost"
  port: "6379"
  password: "secret"
locations:
  - path: "/api"
    target_url: "http://backend:8000"
    replace_path: true
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
	assert.Equal(t, 1, len(loadedConfig.Locations))
	assert.Equal(t, "/api", loadedConfig.Locations[0].Path)
}

// TestUpdateConfig verifies that UpdateConfig correctly updates the configuration.
func TestUpdateConfig(t *testing.T) {
	initialConfig := &config.ProxyConfig{
		Port: "8080",
		Logging: config.Logging{
			Enabled: false,
		},
	}
	config.UpdateConfig(initialConfig)
	assert.Equal(t, initialConfig, config.GetCurrentProxyConfig())

	updatedConfig := &config.ProxyConfig{
		Port: "9090",
		Logging: config.Logging{
			Enabled: true,
		},
	}
	config.UpdateConfig(updatedConfig)
	assert.Equal(t, updatedConfig, config.GetCurrentProxyConfig())
}

// TestIsConfigDifferent verifies the behavior of isConfigDifferent.
func TestIsConfigDifferent(t *testing.T) {
	config1 := &config.ProxyConfig{Port: "8080"}
	config2 := &config.ProxyConfig{Port: "8081"}
	assert.True(t, config.IsConfigDifferent(config1, config2))

	config2.Port = "8080"
	assert.False(t, config.IsConfigDifferent(config1, config2))
}

// TestWatchConfig verifies that WatchConfig invokes the callback on configuration changes.
func TestWatchConfig(t *testing.T) {
	content := `
port: "8080"
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
`
	err = os.WriteFile(file.Name(), []byte(updatedContent), 0644)
	assert.NoError(t, err)

	time.Sleep(3 * time.Second)
	assert.True(t, callbackInvoked)
}
