package app

import (
	"context"
	"dito/config"
	"dito/logging"
	"log/slog"
	"testing"
)

// TestLoggerUpdate tests if the logger updates correctly when the log level changes.
func TestLoggerUpdate(t *testing.T) {
	// Initial configuration with log level set to "info"
	initialConfig := &config.ProxyConfig{
		Logging: config.Logging{
			Level: "info",
		},
	}

	// Update the configuration
	config.UpdateConfig(initialConfig)

	// Create a new Dito instance with the initial logger
	// Create a mock HTTP transport config for testing
	mockHTTPTransportConfig := &config.HTTPTransportConfig{}

	dito := NewDito(mockHTTPTransportConfig, logging.InitializeLogger(initialConfig.Logging.Level))

	// Check if the initial logger level is set to "info"
	if dito.Logger.Handler().Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Expected initial logger level to be info, but it is set to debug")
	}

	// New configuration with log level set to "debug"
	newConfig := &config.ProxyConfig{
		Logging: config.Logging{
			Level: "debug",
		},
	}
	// Update the components with the new configuration
	dito.UpdateComponents(newConfig)

	// Check if the logger level is updated to "debug"
	if !dito.Logger.Handler().Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Expected logger level to be updated to debug, but it is not")
	}

	// Log messages at different levels to verify the logger behavior
	dito.Logger.Debug("This should be visible at debug level")
	dito.Logger.Info("This should always be visible")
}
