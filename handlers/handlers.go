package handlers

import (
	"dito/app"
	"dito/config"
	"dito/metrics"
	cmid "dito/middlewares"
	"dito/transport"
	"dito/websocket"
	"dito/writer"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

const (
	InternalServerErrorMessage = "Internal Server Error"
)

// DynamicProxyHandler handles dynamic proxying of requests based on the configuration.
// It reads the request body, matches the request path with configured locations, and applies middlewares.
//
// Parameters:
// - dito: The Dito application instance containing the configuration and logger.
// - w: The HTTP response writer.
// - r: The HTTP request.
func DynamicProxyHandler(dito *app.Dito, w http.ResponseWriter, r *http.Request) {

	if isMetricsEndpoint(r.URL.Path, dito.Config.Metrics.Path) && dito.Config.Metrics.Enabled {
		dito.Logger.Debug("Handling metrics endpoint")
		handler := metrics.ExposeMetricsHandler()
		handler.ServeHTTP(w, r)
		return
	}

	for i, location := range dito.Config.Locations {
		if location.CompiledRegex.MatchString(r.URL.Path) {
			if location.EnableWebsocket && websocket.IsWebSocketRequest(r) {
				dito.Logger.Info("Upgrading to WebSocket for", "path", location.Path)
				websocket.HandleWebSocketProxy(w, r, location.TargetURL, dito.Logger)
				return

			}
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ServeProxy(dito, i, w, r)
			})

			lrw := &writer.ResponseWriter{ResponseWriter: w}
			if len(location.Middlewares) > 0 {
				handlerWithMiddlewares := applyMiddlewares(dito, handler, location)
				handlerWithMiddlewares.ServeHTTP(lrw, r)
			} else {
				handler.ServeHTTP(lrw, r)
			}
			return
		}
	}

	http.NotFound(w, r)

}

// ServeProxy handles the proxying of requests to the target URL specified in the location configuration.
//
// Parameters:
// - dito: The Dito application instance containing the configuration and logger.
// - locationIndex: The index of the location configuration in the Dito configuration.
// - lrw: The HTTP response writer.
// - r: The HTTP request.
func ServeProxy(dito *app.Dito, locationIndex int, lrw http.ResponseWriter, r *http.Request) {
	location := dito.Config.Locations[locationIndex]

	caronteTransport := &transport.Caronte{
		Location:       &location,
		TransportCache: dito.TransportCache,
	}

	targetURL, err := url.Parse(location.TargetURL)
	if err != nil {
		dito.Logger.Error("Error parsing the target URL: ", "error", err)
		http.Error(lrw, InternalServerErrorMessage, http.StatusInternalServerError)
		return
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host

			if location.ReplacePath {
				req.URL.Path = targetURL.Path
			} else {
				additionalPath := strings.TrimPrefix(r.URL.Path, location.Path)
				req.URL.Path = normalizePath(targetURL.Path, additionalPath)
			}

			req.URL.RawQuery = r.URL.RawQuery

			req.Host = targetURL.Host
		},
		Transport: caronteTransport,
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			dito.Logger.Error(fmt.Sprintf("Error proxying request: %v", err))

			if os.IsTimeout(err) {
				http.Error(w, "Gateway Timeout", http.StatusGatewayTimeout)
			} else {
				http.Error(w, "Bad Gateway", http.StatusBadGateway)
			}
		},
	}
	proxy.ServeHTTP(lrw, r)
}

// applyMiddlewares applies the configured middlewares to the given handler.
//
// Parameters:
// - dito: The Dito application instance containing the configuration and logger.
// - handler: The HTTP handler to which the middlewares will be applied.
// - location: The location configuration containing the middlewares to be applied.
//
// Returns:
// - http.Handler: The handler with the applied middlewares.
func applyMiddlewares(dito *app.Dito, handler http.Handler, location config.LocationConfig) http.Handler {
	for i := len(location.Middlewares) - 1; i >= 0; i-- {
		middleware := location.Middlewares[i]
		switch middleware {
		case "auth":
			dito.Logger.Debug("Applying Auth Middleware")
			handler = cmid.AuthMiddleware(handler, dito.Logger)
		case "rate-limiter":
			if location.RateLimiting.Enabled {
				dito.Logger.Debug("Applying Rate Limiter Middleware")
				handler = cmid.RateLimiterMiddleware(handler, location.RateLimiting, dito.Logger)
			}
		case "rate-limiter-redis":
			if location.RateLimiting.Enabled && dito.RedisClient != nil && dito.Config.Redis.Enabled {
				dito.Logger.Debug("Applying Rate Limiter Middleware")
				handler = cmid.RateLimiterMiddlewareWithRedis(handler, location.RateLimiting, dito.RedisClient, dito.Logger)
			}
		case "cache":
			if dito.RedisClient != nil && dito.Config.Redis.Enabled && location.Cache.Enabled {
				dito.Logger.Debug(fmt.Sprintf("Applying Cache Middleware with TTL: %d seconds", location.Cache.TTL))
				handler = cmid.CacheMiddleware(handler, dito, location.Cache)
			}
		}
	}
	return handler
}

// normalizePath normalizes the base path and additional path by ensuring there is exactly one slash between them.
//
// Parameters:
// - basePath: The base path.
// - additionalPath: The additional path to be appended to the base path.
//
// Returns:
// - string: The normalized path.
func normalizePath(basePath, additionalPath string) string {
	return strings.TrimSuffix(basePath, "/") + "/" + strings.TrimPrefix(additionalPath, "/")
}

// isMetricsEndpoint checks if the request path matches the configured metrics path.
//
// Parameters:
// - requestPath: The path of the incoming HTTP request.
// - metricsPath: The configured path for metrics.
//
// Returns:
// - bool: True if the request path matches the metrics path, false otherwise.
func isMetricsEndpoint(requestPath string, metricsPath string) bool {
	return requestPath == metricsPath
}
