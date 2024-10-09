package redis

import (
	"context"
	"dito/config"
	"fmt"
	"github.com/redis/go-redis/v9"
	"log"
	"log/slog"
	"time"
)

// InitRedis initializes a Redis client with the provided logger and Redis configuration.
// It attempts to connect to the Redis server and logs the connection status.
//
// Parameters:
// - logger: A pointer to the slog.Logger instance for logging messages.
// - redisConfig: The Redis configuration containing host, port, and password.
//
// Returns:
// - *redis.Client: A pointer to the initialized Redis client, or nil if the connection fails.
func InitRedis(logger *slog.Logger, redisConfig config.RedisConfig) (*redis.Client, error) {
	addr := fmt.Sprintf("%s:%s", redisConfig.Host, redisConfig.Port)
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: redisConfig.Password,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := client.Ping(ctx).Result()
	if err != nil {
		//logger.Error("Failed to connect to Redis", "error", err)
		return nil, err
	}

	logger.Info("Successfully connected to Redis")
	return client, nil
}

// RedisHealthCheck performs a health check on the provided Redis client.
// It pings the Redis server and logs a fatal error if the server is down.
//
// Parameters:
// - client: A pointer to the Redis client to be checked.
//
// Returns:
// - map[string]string: A map containing the health check message.
func RedisHealthCheck(client *redis.Client) map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := client.Ping(ctx).Result()
	if err != nil {
		log.Fatalf(fmt.Sprintf("Redis down: %v", err))
	}

	return map[string]string{
		"message": "It's healthy",
	}
}
