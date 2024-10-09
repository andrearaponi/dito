package middlewares

import (
	"context"
	"dito/app"
	"dito/config"
	"dito/writer"
	"fmt"
	"net/http"
	"time"
)

// CacheMiddleware is an HTTP middleware that caches responses in Redis.
// It checks if caching is enabled and if the request allows caching.
// If a cached response is found, it serves the response from the cache.
// Otherwise, it processes the request and caches the response.
//
// Parameters:
// - next: The next http.Handler to be called if the request is not cached.
// - dito: The Dito application instance containing the Redis client and logger.
// - locationConfig: The configuration for caching.
//
// Returns:
// - http.Handler: A handler that applies caching based on the provided configuration.
func CacheMiddleware(next http.Handler, dito *app.Dito, locationConfig config.Cache) http.Handler {
	middlewareType := "CacheMiddlewareRedis"
	dito.Logger.Debug(fmt.Sprintf("[%s] Executing", middlewareType))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !locationConfig.Enabled || locationConfig.TTL <= 0 || r.Header.Get("Cache-Control") == "no-cache" {
			dito.Logger.Debug(fmt.Sprintf("[%s] Cache is not enabled or request has 'Cache-Control: no-cache'. Proceeding without cache.", middlewareType))
			next.ServeHTTP(w, r)
			return
		}

		cacheKey := generateCacheKey(r)

		cachedContentType, err1 := dito.RedisClient.Get(context.Background(), cacheKey+":content-type").Result()
		cachedResponse, err2 := dito.RedisClient.Get(context.Background(), cacheKey).Result()

		if err1 == nil && err2 == nil {
			dito.Logger.Debug(fmt.Sprintf("[%s] Cache hit for key: %s", middlewareType, cacheKey))

			w.Header().Set("Content-Type", cachedContentType)
			w.WriteHeader(http.StatusOK)
			_, writeErr := w.Write([]byte(cachedResponse))
			if writeErr != nil {
				dito.Logger.Error(fmt.Sprintf("[%s] Failed to write cached response: %v", middlewareType, writeErr))
			}
			return
		} else {
			dito.Logger.Debug(fmt.Sprintf("[%s] Cache miss for key: %s", middlewareType, cacheKey))
		}

		lrw := &writer.ResponseWriter{ResponseWriter: w}
		next.ServeHTTP(lrw, r)

		if lrw.StatusCode == http.StatusOK && lrw.Body.Len() > 0 {
			err := dito.RedisClient.Set(context.Background(), cacheKey, lrw.Body.String(), time.Duration(locationConfig.TTL)*time.Second).Err()
			if err != nil {
				dito.Logger.Error(fmt.Sprintf("[%s] Failed to cache response: %v", middlewareType, err))
			}

			contentType := lrw.Header().Get("Content-Type")
			err = dito.RedisClient.Set(context.Background(), cacheKey+":content-type", contentType, time.Duration(locationConfig.TTL)*time.Second).Err()
			if err != nil {
				dito.Logger.Error(fmt.Sprintf("[%s] Failed to cache content-type: %v", middlewareType, err))
			}
		}
	})
}

// generateCacheKey generates a cache key based on the request method and URI.
//
// Parameters:
// - r: The HTTP request.
//
// Returns:
// - string: The generated cache key.
func generateCacheKey(r *http.Request) string {
	return fmt.Sprintf("cache:%s:%s", r.Method, r.URL.RequestURI())
}
