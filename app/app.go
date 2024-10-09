package app

import (
	credis "dito/client/redis"
	"dito/config"
	"dito/logging"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"sync"
)

// Dito represents the main application structure.
// It holds the configuration, Redis client, and logger.
type Dito struct {
	Config      *config.ProxyConfig
	configMutex sync.RWMutex
	RedisClient *redis.Client
	Logger      *slog.Logger
}

// NewDito creates a new instance of Dito with the provided Redis client and logger.
//
// Parameters:
// - redisClient: The Redis client instance.
// - logger: The logger instance.
//
// Returns:
// - *Dito: A pointer to the newly created Dito instance.
func NewDito(redisClient *redis.Client, logger *slog.Logger) *Dito {
	return &Dito{
		Config:      config.GetCurrentProxyConfig(),
		RedisClient: redisClient,
		Logger:      logger,
	}
}

// UpdateConfig safely updates the configuration in the Dito instance.
//
// Parameters:
// - newConfig: The new configuration to be set.
func (d *Dito) UpdateConfig(newConfig *config.ProxyConfig) {
	d.configMutex.Lock()
	d.Config = newConfig
	d.configMutex.Unlock()
	d.Logger.Warn("Configuration updated in Dito")
}

// GetCurrentConfig returns a pointer to the current configuration, safely.
//
// Returns:
// - *config.ProxyConfig: The current proxy configuration.
func (d *Dito) GetCurrentConfig() *config.ProxyConfig {
	d.configMutex.RLock()
	config := d.Config
	d.configMutex.RUnlock()
	return config
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
		//fmt.Printf("New logger level is set to: %s\n", newConfig.Logging.Level)
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
