package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"log/slog"
	"os"
	"regexp"
	"sync/atomic"
	"time"
)

// RedisConfig holds the configuration for connecting to a Redis server.
type RedisConfig struct {
	Enabled  bool   `yaml:"enabled"`  // Enables/disables Redis.
	Host     string `yaml:"host"`     // Redis server host.
	Port     string `yaml:"port"`     // Redis server port.
	Password string `yaml:"password"` // Redis server password.
}

// ProxyConfig holds the configuration for the proxy server.
type ProxyConfig struct {
	Port      string           `yaml:"port"`       // Port the proxy will listen on.
	HotReload bool             `yaml:"hot_reload"` // Enables/disables hot reloading.
	Logging   Logging          `yaml:"logging"`    // Logging configuration.
	Redis     RedisConfig      `yaml:"redis"`      // Redis configuration.
	Locations []LocationConfig `yaml:"locations"`  // List of configurations for each location.
}

// RateLimiting holds the configuration for rate limiting.
type RateLimiting struct {
	Enabled           bool    `yaml:"enabled"`             // Enables/disables rate limiting globally.
	RequestsPerSecond float64 `yaml:"requests_per_second"` // Number of requests allowed per second.
	Burst             int     `yaml:"burst"`               // Maximum burst of requests.
}

type Cache struct {
	Enabled bool `yaml:"enabled"` // Enables/disables caching.
	TTL     int  `yaml:"ttl"`     // Time to live for cache entries in seconds.
}

// Logging holds the configuration for logging.
type Logging struct {
	Enabled bool   `yaml:"enabled"` // Enables/disables logging.
	Verbose bool   `yaml:"verbose"` // Enables/disables verbose logging.
	Level   string `yaml:"level"`   // Log level (e.g., debug, info, warn, error).
}

// LocationConfig holds the configuration for a specific location.
type LocationConfig struct {
	Path              string            `yaml:"path"` // Path the proxy will respond to.
	CompiledRegex     *regexp.Regexp    // Compiled regular expression for the path.
	TargetURL         string            `yaml:"target_url"`         // Destination URL for this location.
	ReplacePath       bool              `yaml:"replace_path"`       // Whether to replace the path entirely.
	AdditionalHeaders map[string]string `yaml:"additional_headers"` // Additional headers to add for this location.
	ExcludedHeaders   []string          `yaml:"excluded_headers"`   // Headers to exclude for this location.
	Middlewares       []string          `yaml:"middlewares"`        // List of middlewares to apply for this location.
	RateLimiting      RateLimiting      `yaml:"rate_limiting"`      // Rate Limiting configuration.
	EnableCompression bool              `yaml:"enable_compression"` // Flag to enable Gzip Compression.
	Cache             Cache             `yaml:"cache"`              // Cache configuration.engin
	CertFile          string            `yaml:"cert_file"`          // Path to the certificate file.
	KeyFile           string            `yaml:"key_file"`           // Path to the key file.
	CaFile            string            `yaml:"ca_file"`            // Path to the CA file.
}

var currentConfig atomic.Value

// LoadConfiguration loads the proxy configuration from a YAML file.
//
// Parameters:
// - file: The path to the configuration file.
//
// Returns:
// - *ProxyConfig: A pointer to the loaded ProxyConfig.
// - error: An error if the configuration could not be loaded.
func LoadConfiguration(file string) (*ProxyConfig, error) {
	var config ProxyConfig
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	if err = yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	for i, location := range config.Locations {
		regex, err := regexp.Compile(location.Path)
		if err != nil {
			return nil, fmt.Errorf("error compiling regex for path %s: %v", location.Path, err)
		}
		config.Locations[i].CompiledRegex = regex
	}

	return &config, nil
}

// UpdateConfig updates the current configuration with a new configuration.
//
// Parameters:
// - newConfig: A pointer to the new ProxyConfig.
func UpdateConfig(newConfig *ProxyConfig) {
	currentConfig.Store(newConfig)
	if !newConfig.Logging.Enabled {
		log.SetOutput(io.Discard)
	} else {
		log.SetOutput(os.Stdout)
	}
}

// GetCurrentProxyConfig returns the current proxy configuration.
//
// Returns:
// - *ProxyConfig: A pointer to the current ProxyConfig.
func GetCurrentProxyConfig() *ProxyConfig {
	return currentConfig.Load().(*ProxyConfig)
}

// LoadAndSetConfig loads the configuration from a file and sets it as the current configuration.
//
// Parameters:
// - configFile: The path to the configuration file.
func LoadAndSetConfig(configFile string) {
	config, err := LoadConfiguration(configFile)
	if err != nil {
		log.Fatal(err)
	}
	UpdateConfig(config)
}

// IsConfigDifferent compares two configurations to determine if they are different.
//
// Parameters:
// - config1: A pointer to the first ProxyConfig.
// - config2: A pointer to the second ProxyConfig.
//
// Returns:
// - bool: True if the configurations are different, false otherwise.
func IsConfigDifferent(config1, config2 *ProxyConfig) bool {
	if config1.Port != config2.Port {
		return true
	}
	if config1.HotReload != config2.HotReload {
		return true
	}
	if config1.Logging.Enabled != config2.Logging.Enabled ||
		config1.Logging.Level != config2.Logging.Level ||
		config1.Logging.Verbose != config2.Logging.Verbose {
		return true
	}
	if config1.Redis.Enabled != config2.Redis.Enabled ||
		config1.Redis.Host != config2.Redis.Host ||
		config1.Redis.Port != config2.Redis.Port ||
		config1.Redis.Password != config2.Redis.Password {
		return true
	}
	if len(config1.Locations) != len(config2.Locations) {
		return true
	}
	for i, loc1 := range config1.Locations {
		loc2 := config2.Locations[i]
		if loc1.Path != loc2.Path || loc1.TargetURL != loc2.TargetURL ||
			loc1.ReplacePath != loc2.ReplacePath ||
			!areHeadersEqual(loc1.AdditionalHeaders, loc2.AdditionalHeaders) ||
			!areHeadersEqualList(loc1.ExcludedHeaders, loc2.ExcludedHeaders) ||
			!areMiddlewareListsEqual(loc1.Middlewares, loc2.Middlewares) ||
			!compareRateLimiting(&loc1.RateLimiting, &loc2.RateLimiting) ||
			loc1.CertFile != loc2.CertFile ||
			loc1.KeyFile != loc2.KeyFile ||
			loc1.CaFile != loc2.CaFile ||
			loc1.Cache.Enabled != loc2.Cache.Enabled ||
			loc1.Cache.TTL != loc2.Cache.TTL {
			return true
		}
	}
	return false
}

func areMiddlewareListsEqual(list1, list2 []string) bool {
	if len(list1) != len(list2) {
		return false
	}
	for i := range list1 {
		if list1[i] != list2[i] {
			return false
		}
	}
	return true
}

// compareRateLimiting compares two RateLimiting configurations.
//
// Parameters:
// - rl1: A pointer to the first RateLimiting configuration.
// - rl2: A pointer to the second RateLimiting configuration.
//
// Returns:
// - bool: True if the RateLimiting configurations are different, false otherwise.
func compareRateLimiting(rl1, rl2 *RateLimiting) bool {
	return rl1.Enabled != rl2.Enabled || rl1.RequestsPerSecond != rl2.RequestsPerSecond || rl1.Burst != rl2.Burst
}

// areHeadersEqual compares two maps of headers.
//
// Parameters:
// - headers1: The first map of headers.
// - headers2: The second map of headers.
//
// Returns:
// - bool: True if the headers are equal, false otherwise.
func areHeadersEqual(headers1, headers2 map[string]string) bool {
	if len(headers1) != len(headers2) {
		return false
	}
	for k, v := range headers1 {
		if headers2[k] != v {
			return false
		}
	}
	return true
}

// areHeadersEqualList compares two slices of headers.
//
// Parameters:
// - headers1: The first slice of headers.
// - headers2: The second slice of headers.
//
// Returns:
// - bool: True if the headers are equal, false otherwise.
func areHeadersEqualList(headers1, headers2 []string) bool {
	if len(headers1) != len(headers2) {
		return false
	}

	headerMap := make(map[string]struct{})
	for _, header := range headers1 {
		headerMap[header] = struct{}{}
	}

	for _, header := range headers2 {
		if _, exists := headerMap[header]; !exists {
			return false
		}
	}

	return true
}

// WatchConfig watches the configuration file for changes and invokes a callback when changes are detected.
//
// Parameters:
// - configFile: The path to the configuration file.
// - onChange: A callback function to invoke when the configuration changes.
// - logger: A logger to log messages.
func WatchConfig(configFile string, onChange func(*ProxyConfig), logger *slog.Logger) {
	var lastModified time.Time
	isFirstCheck := true

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fileInfo, err := os.Stat(configFile)
			if err != nil {
				logger.Error(fmt.Sprintf("Error statting configuration file: %v", err))
				continue
			}

			if fileInfo.ModTime().After(lastModified) {
				time.Sleep(1 * time.Second)

				newConfig, err := LoadConfiguration(configFile)
				if err != nil {
					logger.Error(fmt.Sprintf("Error loading configuration: %v", err))
					continue
				}

				if isFirstCheck {
					isFirstCheck = false
				} else if IsConfigDifferent(GetCurrentProxyConfig(), newConfig) {
					onChange(newConfig)
				}

				lastModified = fileInfo.ModTime()
			}
		}
	}
}
