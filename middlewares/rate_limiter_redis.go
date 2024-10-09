package middlewares

import (
	"context"
	"dito/config"
	"fmt"
	"github.com/redis/go-redis/v9"
	"log/slog"
	"net/http"
	"time"
)

const rateLimiterKeyPrefix = "rate_limiter:"

// RateLimiterMiddlewareWithRedis is an HTTP middleware that applies rate limiting using Redis.
// It checks if rate limiting is enabled and uses Redis to track request counts for each client IP.
//
// Parameters:
// - next: The next http.Handler to be called if the request is allowed.
// - rateLimitingConfig: The configuration for rate limiting.
// - redisClient: The Redis client used to store and retrieve rate limiting data.
// - logger: The logger used to log messages.
//
// Returns:
// - http.Handler: A handler that applies rate limiting based on the provided configuration.
func RateLimiterMiddlewareWithRedis(next http.Handler, rateLimitingConfig config.RateLimiting, redisClient *redis.Client, logger *slog.Logger) http.Handler {
	middlewareType := "RateLimiterMiddlewareWithRedis"
	logger.Debug(fmt.Sprintf("[%s] Rate limiting is enabled with %v requests per second", middlewareType, rateLimitingConfig.RequestsPerSecond))
	if !rateLimitingConfig.Enabled {
		logger.Debug(fmt.Sprintf("[%s] Rate limiting is disabled", middlewareType))
		return next // No rate limiting if disabled
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r, logger, middlewareType)

		// Debug: Log the client IP and request
		logger.Debug(fmt.Sprintf("[%s] Handling request from IP: %s, Path: %s", middlewareType, ip, r.URL.Path))

		// Check if the request is allowed
		allowed, err := allowRequest(redisClient, ip, rateLimitingConfig, logger, middlewareType)
		if err != nil {
			logger.Error(fmt.Sprintf("[%s] Error checking rate limit for IP %s: %v", middlewareType, ip, err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// If the request exceeds the rate limit, return 429 (Too Many Requests)
		if !allowed {
			logger.Debug(fmt.Sprintf("[%s] Rate limit exceeded for IP: %s", middlewareType, ip))
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		// Debug: Log that the request was allowed
		logger.Debug(fmt.Sprintf("[%s] Request allowed for IP: %s", middlewareType, ip))

		next.ServeHTTP(w, r)
	})
}

// allowRequest checks if a request is allowed based on the rate limiting configuration and Redis data.
//
// Parameters:
// - redisClient: The Redis client used to store and retrieve rate limiting data.
// - ip: The IP address of the client making the request.
// - rateLimitingConfig: The configuration for rate limiting.
// - logger: The logger used to log messages.
// - middlewareType: The type of middleware for logging purposes.
//
// Returns:
// - bool: True if the request is allowed, false otherwise.
// - error: An error if there was an issue checking the rate limit.
func allowRequest(redisClient *redis.Client, ip string, rateLimitingConfig config.RateLimiting, logger *slog.Logger, middlewareType string) (bool, error) {
	ctx := context.Background()
	key := rateLimiterKeyPrefix + ip

	limit := rateLimitingConfig.RequestsPerSecond
	expiry := time.Second

	count, err := redisClient.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}

	if count == 1 {
		err = redisClient.Expire(ctx, key, expiry).Err()
		if err != nil {
			return false, err
		}
	}

	if count > int64(limit) {
		logger.Debug(fmt.Sprintf("[%s] Rate limit exceeded for IP: %s, count: %d", middlewareType, ip, count))
		return false, nil
	}

	logger.Debug(fmt.Sprintf("[%s] Request count for IP %s is %d, allowing request", middlewareType, ip, count))
	return true, nil
}
