package middlewares

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dito/config"
	"golang.org/x/time/rate"
)

// RateLimiter defines a rate limiter for each client IP.
type RateLimiter struct {
	limiter  *rate.Limiter
	lastSeen int64 // Unix timestamp for thread-safe updates
}

// In-memory store to track rate limiters for each IP
var clients = make(map[string]*RateLimiter)
var mu sync.RWMutex

// RateLimiterMiddleware manages the rate limiting for each IP address in the context of a specific location.
//
// Parameters:
// - next: The next http.Handler to be called if the request is allowed.
// - rateLimitingConfig: The configuration for rate limiting.
// - logger: The logger used to log messages.
//
// Returns:
// - http.Handler: A handler that applies rate limiting based on the provided configuration.
func RateLimiterMiddleware(next http.Handler, rateLimitingConfig config.RateLimiting, logger *slog.Logger) http.Handler {
	middlewareType := "RateLimiterMiddleware"
	logger.Debug(fmt.Sprintf("[%s] Executing", middlewareType))
	logger.Debug(fmt.Sprintf("[%s] Rate limiting is enabled with %v requests per second and a burst of %v\n", middlewareType, rateLimitingConfig.RequestsPerSecond, rateLimitingConfig.Burst))
	if !rateLimitingConfig.Enabled {
		logger.Debug(fmt.Sprintf("[%s] Rate limiting is disabled", middlewareType))
		return next // No rate limiting if disabled
	}

	// Periodically clean up old clients (those not seen for 3 minutes)
	go cleanupOldClients(logger, middlewareType)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r, logger, middlewareType)

		// Debug: Log the client IP and request
		logger.Debug(fmt.Sprintf("[%s] Handling request from IP: %s, Path: %s", middlewareType, ip, r.URL.Path))

		// Retrieve or create a new limiter for the client IP
		limiter := getOrCreateLimiter(ip, rateLimitingConfig, logger, middlewareType)

		// Check if the request is allowed
		allowed := limiter.limiter.Allow()
		logger.Debug(fmt.Sprintf("[%s] Rate limiter for IP %s: Allowed: %v", middlewareType, ip, allowed))

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

// getOrCreateLimiter retrieves or creates a new rate limiter for the client IP.
//
// Parameters:
// - ip: The IP address of the client making the request.
// - rateLimitingConfig: The configuration for rate limiting.
// - logger: The logger used to log messages.
// - middlewareType: The type of middleware for logging purposes.
//
// Returns:
// - *RateLimiter: The rate limiter for the client IP.
func getOrCreateLimiter(ip string, rateLimitingConfig config.RateLimiting, logger *slog.Logger, middlewareType string) *RateLimiter {
	mu.RLock()
	limiter, exists := clients[ip]
	mu.RUnlock()

	if !exists {
		mu.Lock()
		// Double check if the limiter was created during the RUnlock -> Lock phase
		limiter, exists = clients[ip]
		if !exists {
			logger.Debug(fmt.Sprintf("[%s] Creating new limiter for IP: %s", middlewareType, ip))
			limiter = &RateLimiter{
				limiter:  rate.NewLimiter(rate.Limit(rateLimitingConfig.RequestsPerSecond), rateLimitingConfig.Burst),
				lastSeen: time.Now().Unix(),
			}
			clients[ip] = limiter
		}
		mu.Unlock()
	}

	// Update the lastSeen timestamp atomically
	atomic.StoreInt64(&limiter.lastSeen, time.Now().Unix())

	return limiter
}

// cleanupOldClients cleans up clients that haven't been seen for more than 3 minutes.
//
// Parameters:
// - logger: The logger used to log messages.
// - middlewareType: The type of middleware for logging purposes.
func cleanupOldClients(logger *slog.Logger, middlewareType string) {
	for {
		time.Sleep(time.Minute)
		mu.Lock()
		for ip, limiter := range clients {
			if time.Now().Unix()-atomic.LoadInt64(&limiter.lastSeen) > 3*60 {
				logger.Debug(fmt.Sprintf("[%s] Cleaning up old client: %s\n", middlewareType, ip))
				delete(clients, ip)
			}
		}
		mu.Unlock()
	}
}

// getClientIP extracts the client's IP address from the request.
//
// Parameters:
// - r: The HTTP request.
// - logger: The logger used to log messages.
// - middlewareType: The type of middleware for logging purposes.
//
// Returns:
// - string: The client's IP address.
func getClientIP(r *http.Request, logger *slog.Logger, middlewareType string) string {
	ip := r.RemoteAddr

	// If behind a proxy, use X-Forwarded-For
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// Take the first IP from the X-Forwarded-For list
		ip = strings.Split(fwd, ",")[0]
		ip = strings.TrimSpace(ip)
	}

	// Handle cases where IP comes with port (e.g. [::1]:51260 or 127.0.0.1:8080)
	ip = strings.Split(ip, ":")[0]

	// Log the detected IP
	logger.Debug(fmt.Sprintf("[%s] Detected client IP: %s", middlewareType, ip))

	return ip
}
