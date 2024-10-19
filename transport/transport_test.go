package transport_test

import (
	"dito/config"
	"dito/transport"
	"github.com/stretchr/testify/assert"
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

func TestGetTransport_ExistingTransport(t *testing.T) {
	setupTestConfig()

	location := &config.LocationConfig{
		Path: "/test",
	}

	cache := transport.NewTransportCache(config.GetCurrentProxyConfig().Transport.HTTP)
	customTransport, err := cache.GetTransport(location, config.GetCurrentProxyConfig().Transport.HTTP)
	assert.NoError(t, err)
	assert.NotNil(t, customTransport)

	cachedTransport, err := cache.GetTransport(location, config.GetCurrentProxyConfig().Transport.HTTP)
	assert.NoError(t, err)
	assert.Equal(t, customTransport, cachedTransport)
}

func TestGetTransport_NewTransport(t *testing.T) {
	setupTestConfig()

	location := &config.LocationConfig{
		Path: "/new",
	}

	cache := transport.NewTransportCache(config.GetCurrentProxyConfig().Transport.HTTP)
	customTransport, err := cache.GetTransport(location, config.GetCurrentProxyConfig().Transport.HTTP)
	assert.NoError(t, err)
	assert.NotNil(t, customTransport)
}

func TestInvalidateTransport(t *testing.T) {
	setupTestConfig()

	location := &config.LocationConfig{
		Path: "/invalidate",
	}

	cache := transport.NewTransportCache(config.GetCurrentProxyConfig().Transport.HTTP)
	customTransport, err := cache.GetTransport(location, config.GetCurrentProxyConfig().Transport.HTTP)
	assert.NoError(t, err)
	assert.NotNil(t, customTransport)

	cache.InvalidateTransport(config.GetCurrentProxyConfig().Transport.HTTP)
	invalidatedTransport, err := cache.GetTransport(location, config.GetCurrentProxyConfig().Transport.HTTP)
	assert.NoError(t, err)
	assert.NotEqual(t, customTransport, invalidatedTransport)
}

func TestClearTransports(t *testing.T) {
	setupTestConfig()

	location := &config.LocationConfig{
		Path: "/clear",
	}

	cache := transport.NewTransportCache(config.GetCurrentProxyConfig().Transport.HTTP)
	customTransport, err := cache.GetTransport(location, config.GetCurrentProxyConfig().Transport.HTTP)
	assert.NoError(t, err)
	assert.NotNil(t, customTransport)

	cache.Clear()
	clearedTransport, err := cache.GetTransport(location, config.GetCurrentProxyConfig().Transport.HTTP)
	assert.NoError(t, err)
	assert.NotEqual(t, customTransport, clearedTransport)
}
