package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"log/slog"
	"os"
	"reflect"
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

// HTTPTransportConfig holds the configuration settings for the HTTP transport.
//
// Fields:
// - IdleConnTimeout: The maximum amount of time an idle (keep-alive) connection will remain idle before closing.
// - MaxIdleConns: The maximum number of idle (keep-alive) connections across all hosts.
// - MaxIdleConnsPerHost: The maximum number of idle (keep-alive) connections to keep per-host.
// - TLSHandshakeTimeout: The maximum amount of time allowed for the TLS handshake.
// - ResponseHeaderTimeout: The maximum amount of time to wait for a server's response headers after fully writing the request.
// - ExpectContinueTimeout: The maximum amount of time to wait for a server's first response headers after fully writing the request headers if the request has an "Expect: 100-continue" header.
// - DisableCompression: Whether to disable compression (gzip) for requests.
// - DialTimeout: The maximum amount of time to wait for a dial to complete.
// - KeepAlive: The interval between keep-alive probes for an active network connection.
type HTTPTransportConfig struct {
	IdleConnTimeout       time.Duration `yaml:"idle_conn_timeout"`
	MaxIdleConns          int           `yaml:"max_idle_conns"`
	MaxIdleConnsPerHost   int           `yaml:"max_idle_conns_per_host"`
	MaxConnsPerHost       int           `yaml:"max_conns_per_host"`
	TLSHandshakeTimeout   time.Duration `yaml:"tls_handshake_timeout"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
	ExpectContinueTimeout time.Duration `yaml:"expect_continue_timeout"`
	DisableCompression    bool          `yaml:"disable_compression"`
	ForceHTTP2            bool          `yaml:"force_http2"`
	DialTimeout           time.Duration `yaml:"dial_timeout"`
	KeepAlive             time.Duration `yaml:"keep_alive"`
	CertFile              string        `yaml:"cert_file"` // Path to the certificate file.
	KeyFile               string        `yaml:"key_file"`  // Path to the key file.
	CaFile                string        `yaml:"ca_file"`   // Path to the CA file.
}

type TransportConfig struct {
	HTTP HTTPTransportConfig `yaml:"http"`
}

// MetricsConfig holds the configuration for the metrics server.
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"` // Enables/disables the metrics server.
	Path    string `yaml:"path"`    // Path the metrics server will respond to.
}

// ProxyConfig holds the configuration for the proxy server.
type ProxyConfig struct {
	Port      string           `yaml:"port"`       // Port the proxy will listen on.
	HotReload bool             `yaml:"hot_reload"` // Enables/disables hot reloading.
	Logging   Logging          `yaml:"logging"`    // Logging configuration.
	Redis     RedisConfig      `yaml:"redis"`      // Redis configuration.
	Metrics   MetricsConfig    `yaml:"metrics"`    // Metrics configuration.
	Locations []LocationConfig `yaml:"locations"`  // List of configurations for each location.
	Transport TransportConfig  `yaml:"transport"`  // Transport configuration.
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
	Transport         *TransportConfig  `yaml:"transport"`          // Optional Transport configuration for this location.
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

		if location.Transport == nil {
			config.Locations[i].Transport = &config.Transport
		}
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

// IsConfigDifferent compares two configurations using reflect.DeepEqual to determine if they are different.
//
// Parameters:
// - config1: A pointer to the first ProxyConfig.
// - config2: A pointer to the second ProxyConfig.
//
// Returns:
// - bool: True if the configurations are different, false otherwise.
func IsConfigDifferent(config1, config2 *ProxyConfig) bool {
	return !reflect.DeepEqual(config1, config2)
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
