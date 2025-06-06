package handlers

import (
	"dito/app"
	"dito/config"
	"dito/metrics"
	"dito/plugin"
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

const (
	XForwardedFor   = "X-Forwarded-For"
	XForwardedProto = "X-Forwarded-Proto"
	XForwardedHost  = "X-Forwarded-Host"
)

// contains checks if a header is in the list of excluded headers.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// DynamicProxyHandler handles dynamic proxying of requests based on the configuration.
// It reads the request body, matches the request path with configured locations, and applies middlewares.
//
// Parameters:
// - dito: The Dito application instance containing the configuration and logger.
// - w: The HTTP response writer.
// - r: The HTTP request.
func DynamicProxyHandler(dito *app.Dito, w http.ResponseWriter, r *http.Request, plugins []plugin.Plugin) {

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
				handlerWithMiddlewares := applyMiddlewares(dito, handler, location, plugins)
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

			dito.Logger.Debug(fmt.Sprintf("Modifying headers for location: %s", location.Path))

			for _, header := range location.ExcludedHeaders {
				req.Header.Del(header)
			}

			for header, value := range location.AdditionalHeaders {
				req.Header.Set(header, value)
			}

			if hostHeader, ok := location.AdditionalHeaders["Host"]; ok {
				req.Host = hostHeader
			}

			if !contains(location.ExcludedHeaders, XForwardedFor) {
				clientIP := req.RemoteAddr
				if prior, ok := req.Header[XForwardedFor]; ok {
					req.Header.Set(XForwardedFor, prior[0] + ", " + clientIP)
				} else {
					req.Header.Set(XForwardedFor, clientIP)
				}
			}

			if !contains(location.ExcludedHeaders, XForwardedProto) {
				scheme := "https"
				if r.TLS == nil {
					scheme = "http"
				}
				req.Header.Set(XForwardedProto, scheme)
			}

			if !contains(location.ExcludedHeaders, XForwardedHost) {
				req.Header.Set(XForwardedHost, r.Host)
			}
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
func applyMiddlewares(dito *app.Dito, handler http.Handler, location config.LocationConfig, plugins []plugin.Plugin) http.Handler {
	for i := len(location.Middlewares) - 1; i >= 0; i-- {
		middlewareName := location.Middlewares[i]
		middlewareApplied := false

		// Controlla se un plugin fornisce il middleware richiesto
		for _, p := range plugins {
			if p.Name() == middlewareName {
				middleware := p.MiddlewareFunc()
				if middleware != nil {
					dito.Logger.Debug(fmt.Sprintf("Applying Middleware from Plugin: %s", p.Name()))
					handler = middleware(handler)
					middlewareApplied = true
					break
				}
			}
		}

		// Se il middleware non è stato trovato nei plugin, logghiamo un errore
		if !middlewareApplied {
			dito.Logger.Warn(fmt.Sprintf("Middleware '%s' not found in any plugin", middlewareName))
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
