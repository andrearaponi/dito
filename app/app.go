package app

import (
	credis "dito/client/redis"
	"dito/config"
	"dito/logging"
	"dito/transport"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"sync"
)

// Dito is the main application structure that holds the configuration, Redis client, logger, and transport cache.
type Dito struct {
	Config         *config.ProxyConfig       // Config is the current proxy configuration.
	configMutex    sync.RWMutex              // configMutex is used to safely update the configuration.
	RedisClient    *redis.Client             // RedisClient is the client instance for Redis operations.
	Logger         *slog.Logger              // Logger is used for logging within the application.
	TransportCache *transport.TransportCache // TransportCache is a cache for storing custom HTTP transports.
}

// NewDito creates a new instance of the Dito application structure.
//
// Parameters:
// - redisClient: The Redis client instance for Redis operations.
// - logger: The logger instance for logging within the application.
//
// Returns:
// - *Dito: A pointer to the newly created Dito instance.
func NewDito(redisClient *redis.Client, logger *slog.Logger) *Dito {
	return &Dito{
		Config:         config.GetCurrentProxyConfig(),
		RedisClient:    redisClient,
		Logger:         logger,
		TransportCache: transport.NewTransportCache(),
	}
}

// UpdateConfig safely updates the configuration in the Dito instance.
//
// Parameters:
// - newConfig: The new configuration to be set.
func (d *Dito) UpdateConfig(newConfig *config.ProxyConfig) {
	d.configMutex.Lock()
	d.Config = newConfig
	d.TransportCache.Clear()
	d.configMutex.Unlock()
	d.Logger.Warn("Configuration updated in Dito")
}

// GetCurrentConfig retrieves the current proxy configuration from the Dito instance.
//
// This method acquires a read lock to ensure thread-safe access to the configuration.
//
// Returns:
// - *config.ProxyConfig: The current proxy configuration.
func (d *Dito) GetCurrentConfig() *config.ProxyConfig {
	d.configMutex.RLock()
	proxyConfig := d.Config
	d.configMutex.RUnlock()
	return proxyConfig
}

// UpdateComponents updates the logger and Redis client when configuration changes.
//
// Parameters:
// - newConfig: The new configuration to be set.
func (d *Dito) UpdateComponents(newConfig *config.ProxyConfig) {
	d.configMutex.Lock()
	defer d.configMutex.Unlock()

	if newConfig.Logging.Level != d.Config.Logging.Level {
		d.Logger = logging.InitializeLogger(newConfig.Logging.Level)
	}

	if newConfig.Redis != d.Config.Redis {
		if d.RedisClient != nil {
			d.RedisClient.Close()
		}
		var err error
		d.RedisClient, err = credis.InitRedis(d.Logger, newConfig.Redis)
		if err != nil {
			d.Logger.Error("Failed to initialize Redis client", "error", err)
		}
	}

	d.Config = newConfig
}
