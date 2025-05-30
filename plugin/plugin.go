package plugin

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"dito/config"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"plugin"

	"log/slog"

	"gopkg.in/yaml.v3"
)

// AppAccessor defines an interface for plugins to access application-level components.
type AppAccessor interface {
	GetLogger() *slog.Logger
}

// Plugin defines the interface that all plugins must implement.
type Plugin interface {
	// Name returns the unique name of the plugin.
	Name() string
	// Init initializes the plugin with a given context, configuration, and AppAccessor.
	Init(ctx context.Context, config map[string]interface{}, appAccessor AppAccessor) error
	// MiddlewareFunc returns the middleware function if applicable.
	MiddlewareFunc() func(http.Handler) http.Handler
}

// validatePublicKeyIntegrity checks that the public key file has not been altered.
func validatePublicKeyIntegrity(publicKeyPath, expectedHash string) error {
	data, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != expectedHash {
		return errors.New("public key integrity check failed")
	}

	return nil
}

// verifyPluginSignature ensures that the plugin file has a valid signature.
func verifyPluginSignature(pluginPath string, publicKey ed25519.PublicKey) error {
	data, err := os.ReadFile(pluginPath)
	if err != nil {
		return fmt.Errorf("failed to read plugin file: %w", err)
	}

	hash := sha256.Sum256(data)
	sigData, err := os.ReadFile(pluginPath + ".sig")
	if err != nil {
		return fmt.Errorf("failed to read signature file: %w", err)
	}

	signature, err := hex.DecodeString(string(sigData))
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	if !ed25519.Verify(publicKey, hash[:], signature) {
		return errors.New("plugin signature verification failed")
	}

	return nil
}

// loadPluginConfig loads the configuration file for a specific plugin.
func loadPluginConfig(configPath string) (map[string]interface{}, error) {
	pluginCfg := make(map[string]interface{}) // Renamed to avoid conflict with 'config' package

	// Check if the configuration file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		slog.Default().Warn("Plugin config file not found, creating empty config.", slog.String("path", configPath))
		return pluginCfg, nil // Return an empty configuration if not found
	}

	// Read the YAML configuration file
	data, err := os.ReadFile(configPath)
	if err != nil {
		slog.Default().Error("Failed to read plugin config file", slog.String("path", configPath), slog.Any("error", err))
		return nil, fmt.Errorf("failed to read plugin config file (%s): %w", configPath, err)
	}

	// Log raw YAML for debugging
	slog.Default().Debug("Raw plugin configuration file content", slog.String("path", configPath), slog.String("data", string(data)))

	// Parse the YAML into a map structure
	if err := yaml.Unmarshal(data, &pluginCfg); err != nil {
		slog.Default().Error("Failed to parse YAML in plugin config", slog.String("path", configPath), slog.Any("error", err))
		return nil, fmt.Errorf("failed to parse plugin config YAML for %s: %w", configPath, err)
	}

	// Log parsed config for debugging
	slog.Default().Debug("Parsed plugin configuration", slog.String("path", configPath), slog.Any("config_data", pluginCfg))

	return pluginCfg, nil
}

// LoadPlugin loads a plugin dynamically from a given path after verifying its signature.
func LoadPlugin(pluginDir, pluginName string, publicKey ed25519.PublicKey) (Plugin, map[string]interface{}, error) {
	pluginPath := filepath.Join(pluginDir, pluginName, pluginName+".so")
	configPath := filepath.Join(pluginDir, pluginName, "config.yaml")

	// Verify the plugin's digital signature
	if err := verifyPluginSignature(pluginPath, publicKey); err != nil {
		return nil, nil, fmt.Errorf("plugin signature verification failed: %w", err)
	}

	// Open the plugin file
	p, err := plugin.Open(pluginPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open plugin: %w", err)
	}

	// Lookup for the exported symbol "NewPlugin"
	symbol, err := p.Lookup("NewPlugin")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find symbol 'NewPlugin': %w", err)
	}

	// Ensure the symbol is of the correct type
	newPlugin, ok := symbol.(func() Plugin)
	if !ok {
		return nil, nil, fmt.Errorf("symbol 'NewPlugin' is not of the expected type")
	}

	// Load the plugin-specific configuration
	pluginConfig, err := loadPluginConfig(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config for plugin %s: %w", pluginName, err)
	}

	// Return the plugin instance and its configuration (without initializing it)
	return newPlugin(), pluginConfig, nil
}

// LoadAndVerifyPlugins scans the plugin directory, verifies signatures, and loads plugins dynamically.
func LoadAndVerifyPlugins() ([]Plugin, map[string]map[string]interface{}, error) {
	cfg := config.GetCurrentProxyConfig()
	pluginDir := cfg.Plugins.Directory
	publicKeyPath := cfg.Plugins.PublicKeyPath
	expectedHash := cfg.Plugins.PublicKeyHash

	// Validate the integrity of the public key
	if err := validatePublicKeyIntegrity(publicKeyPath, expectedHash); err != nil {
		return nil, nil, fmt.Errorf("public key integrity validation failed: %w", err)
	}

	// Read the public key file
	publicKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read public key: %w", err)
	}

	// Convert public key bytes to ed25519.PublicKey
	publicKey := ed25519.PublicKey(publicKeyData)

	var plugins []Plugin
	pluginConfigs := make(map[string]map[string]interface{})

	// Iterate over directories inside the plugin directory
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read plugin directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			pluginName := entry.Name()
			p, loadedPluginConfig, err := LoadPlugin(pluginDir, pluginName, publicKey) // Renamed 'config' to 'loadedPluginConfig'
			if err != nil {
				slog.Default().Error("Failed to load plugin", slog.String("plugin_name", pluginName), slog.Any("error", err))
				continue
			}

			// DEBUG: Check if the configuration is loaded correctly
			slog.Default().Debug("Plugin configuration loaded, before Init",
				slog.String("plugin_name", pluginName),
				slog.Any("config_data", loadedPluginConfig))

			// Plugins are now loaded but not initialized
			slog.Default().Info("Plugin loaded, pending initialization", slog.String("plugin_name", p.Name()))

			// Store plugin instance and its configuration
			plugins = append(plugins, p)
			pluginConfigs[pluginName] = loadedPluginConfig
		}
	}

	return plugins, pluginConfigs, nil
}
