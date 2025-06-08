package handlers

import (
	"bytes"
	"context"
	"dito/app"
	"dito/config"
	"dito/metrics"
	"dito/plugin"
	"dito/transport"
	"dito/websocket"
	"dito/writer"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeout         = 30 * time.Second
	maxRequestBodySize     = 10 * 1024 * 1024 // 10MB
	maxHeaderSize          = 1 * 1024 * 1024  // 1MB
	contentTypeJSON        = "application/json; charset=utf-8"
	headerXForwardedFor    = "X-Forwarded-For"
	headerXForwardedProto  = "X-Forwarded-Proto"
	headerXRealIP          = "X-Real-IP"
	headerXRequestID       = "X-Request-ID"
	headerXResponseTime    = "X-Response-Time"
	headerXRateLimitLimit  = "X-RateLimit-Limit"
	headerXRateLimitRemain = "X-RateLimit-Remaining"
	headerXRateLimitReset  = "X-RateLimit-Reset"
)

type contextKey string

const (
	ctxKeyRequestID     contextKey = "request-id"
	ctxKeyStartTime     contextKey = "start-time"
	ctxKeyOriginalHost  contextKey = "original-host"
	ctxKeyOriginalProto contextKey = "original-proto"
)

var (
	ErrInvalidTarget      = errors.New("invalid target URL")
	ErrServiceUnavailable = errors.New("service temporarily unavailable")
	ErrRequestTimeout     = errors.New("request timeout")
	ErrResponseTooLarge   = errors.New("response too large")
)

// errorResponse represents a standardized error response structure
type errorResponse struct {
	Error struct {
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Details map[string]interface{} `json:"details,omitempty"`
	} `json:"error"`
	RequestID string `json:"request_id,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// DynamicProxyHandler handles dynamic proxying of requests based on the configuration.
// It reads the request body, matches the request path with configured locations, and applies middlewares.
//
// Parameters:
// - dito: The Dito application instance containing the configuration and logger.
// - w: The HTTP response writer.
// - r: The HTTP request.
// - plugins: List of loaded plugins that may provide middleware functionality.
func DynamicProxyHandler(dito *app.Dito, w http.ResponseWriter, r *http.Request, plugins []plugin.Plugin) {
	// Enrich context with request metadata
	ctx := enrichContext(r.Context(), r)
	r = r.WithContext(ctx)

	// Validate the incoming request
	if !validateRequest(w, r, dito) {
		return
	}

	// Handle metrics endpoint if enabled
	if dito.Config.Metrics.Enabled && r.URL.Path == dito.Config.Metrics.Path {
		metrics.ExposeMetricsHandler().ServeHTTP(w, r)
		return
	}

	// Match request path against configured locations
	for i, location := range dito.Config.Locations {
		if location.CompiledRegex.MatchString(r.URL.Path) {
			// Handle WebSocket upgrade if enabled
			if location.EnableWebsocket && websocket.IsWebSocketRequest(r) {
				websocket.HandleWebSocketProxy(w, r, location.TargetURL, dito.Logger)
				return
			}

			// Handle regular HTTP request
			handleLocationMatch(dito, w, r, i, plugins)
			return
		}
	}

	// No matching location found
	sendError(w, r, http.StatusNotFound, "Not Found", nil)
}

// handleLocationMatch processes a request that matches a configured location.
// It sets up the appropriate response writers, applies middlewares, and proxies the request.
//
// Parameters:
// - dito: The Dito application instance.
// - w: The HTTP response writer.
// - r: The HTTP request.
// - locationIndex: Index of the matched location in the configuration.
// - plugins: Available plugins for middleware application.
func handleLocationMatch(dito *app.Dito, w http.ResponseWriter, r *http.Request, locationIndex int, plugins []plugin.Plugin) {
	location := dito.Config.Locations[locationIndex]

	// Create the proxy handler
	handler := createProxyHandler(dito, locationIndex)

	// Create custom response writer with location-specific configuration
	lrw := createResponseWriter(w, location, dito.Config)

	// Set up response size limiting if configured
	var targetWriter http.ResponseWriter = lrw
	var interceptor *responseLimitInterceptor

	if limit := location.GetEffectiveMaxResponseBodySize(dito.Config.ResponseLimits.MaxResponseBodySize); limit > 0 {
		interceptor = newResponseLimitInterceptor(lrw, limit, dito, r.URL.Path)
		targetWriter = interceptor
	}

	// Apply middlewares and serve the request
	finalHandler := applyMiddlewares(dito, handler, location, plugins)
	finalHandler.ServeHTTP(targetWriter, r)

	// Ensure any buffered data is flushed
	if interceptor != nil {
		interceptor.Flush()
	}

	// Record metrics
	recordMetrics(r, lrw, time.Since(getStartTime(r.Context())))
}

// createProxyHandler creates an HTTP handler that proxies requests with timeout management.
//
// Parameters:
// - dito: The Dito application instance.
// - locationIndex: Index of the location configuration.
//
// Returns:
// - http.HandlerFunc: The proxy handler function.
func createProxyHandler(dito *app.Dito, locationIndex int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Apply request timeout
		ctx, cancel := context.WithTimeout(r.Context(), getTimeout(dito.Config))
		defer cancel()
		r = r.WithContext(ctx)

		ServeProxy(dito, locationIndex, w, r)
	}
}

// ServeProxy handles the proxying of requests to the target URL specified in the location configuration.
//
// Parameters:
// - dito: The Dito application instance containing the configuration and logger.
// - locationIndex: The index of the location configuration in the Dito configuration.
// - w: The HTTP response writer.
// - r: The HTTP request.
func ServeProxy(dito *app.Dito, locationIndex int, w http.ResponseWriter, r *http.Request) {
	location := dito.Config.Locations[locationIndex]

	// Parse and validate target URL
	targetURL, err := parseAndValidateURL(location.TargetURL)
	if err != nil {
		dito.Logger.Error("Invalid target URL", "error", err, "url", location.TargetURL)
		sendError(w, r, http.StatusInternalServerError, "Configuration error", nil)
		return
	}

	// Create custom transport with location-specific configuration
	transport := &transport.Caronte{
		Location:       &location,
		TransportCache: dito.TransportCache,
	}

	// Create and configure reverse proxy
	proxy := createReverseProxy(targetURL, location, transport, dito, r)
	proxy.ServeHTTP(w, r)
}

// createReverseProxy creates a configured httputil.ReverseProxy instance.
//
// Parameters:
// - targetURL: The parsed target URL.
// - location: The location configuration.
// - transport: The HTTP transport to use.
// - dito: The Dito application instance.
// - originalReq: The original HTTP request.
//
// Returns:
// - *httputil.ReverseProxy: The configured reverse proxy.
func createReverseProxy(targetURL *url.URL, location config.LocationConfig, transport http.RoundTripper, dito *app.Dito, originalReq *http.Request) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Director:       createDirector(targetURL, location, originalReq),
		Transport:      transport,
		ErrorHandler:   createErrorHandler(dito),
		ModifyResponse: createResponseModifier(dito, originalReq),
		BufferPool:     newBufferPool(),
	}
}

// createDirector creates a director function that modifies the request before proxying.
//
// Parameters:
// - targetURL: The target URL to proxy to.
// - location: The location configuration.
// - originalReq: The original request.
//
// Returns:
// - func(*http.Request): The director function.
func createDirector(targetURL *url.URL, location config.LocationConfig, originalReq *http.Request) func(*http.Request) {
	return func(req *http.Request) {
		// Update request URL
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host

		// Handle path replacement or appending
		if location.ReplacePath {
			req.URL.Path = targetURL.Path
		} else {
			req.URL.Path = joinPaths(targetURL.Path, strings.TrimPrefix(originalReq.URL.Path, location.Path))
		}

		// Preserve query parameters
		req.URL.RawQuery = originalReq.URL.RawQuery

		// Update host header
		req.Host = targetURL.Host

		// Add request ID header if available
		if requestID := getRequestID(req.Context()); requestID != "" {
			req.Header.Set(headerXRequestID, requestID)
		}

		// Clean up proxy-specific headers
		sanitizeHeaders(req)
	}
}

// createErrorHandler creates an error handler for the reverse proxy.
//
// Parameters:
// - dito: The Dito application instance.
//
// Returns:
// - func(http.ResponseWriter, *http.Request, error): The error handler function.
func createErrorHandler(dito *app.Dito) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, req *http.Request, err error) {
		// Don't handle errors if response limit was already exceeded
		if interceptor, ok := w.(*responseLimitInterceptor); ok && interceptor.limitExceeded {
			return
		}

		// Log the error with context
		dito.Logger.Error("Proxy error",
			"error", err,
			"path", req.URL.Path,
			"method", req.Method,
			"request_id", getRequestID(req.Context()),
		)

		// Determine appropriate error response
		status := http.StatusBadGateway
		message := "Bad Gateway"

		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			status = http.StatusGatewayTimeout
			message = "Gateway Timeout"
		} else if errors.Is(err, context.Canceled) {
			status = http.StatusRequestTimeout
			message = "Request Timeout"
		}

		sendError(w, req, status, message, map[string]interface{}{
			"upstream_error": err.Error(),
		})
	}
}

// createResponseModifier creates a function to modify responses before sending to client.
//
// Parameters:
// - dito: The Dito application instance.
// - originalReq: The original request.
//
// Returns:
// - func(*http.Response) error: The response modifier function.
func createResponseModifier(dito *app.Dito, originalReq *http.Request) func(*http.Response) error {
	return func(resp *http.Response) error {
		// Add security headers
		addSecurityHeaders(resp)

		// Add request ID to response
		if requestID := getRequestID(originalReq.Context()); requestID != "" {
			resp.Header.Set(headerXRequestID, requestID)
		}

		// Add response time header
		if startTime := getStartTime(originalReq.Context()); !startTime.IsZero() {
			resp.Header.Set(headerXResponseTime, fmt.Sprintf("%.3fms", time.Since(startTime).Seconds()*1000))
		}

		return nil
	}
}

// applyMiddlewares applies the configured middlewares to the given handler.
//
// Parameters:
// - dito: The Dito application instance containing the configuration and logger.
// - handler: The HTTP handler to which the middlewares will be applied.
// - location: The location configuration containing the middlewares to be applied.
// - plugins: Available plugins that may provide middleware.
//
// Returns:
// - http.Handler: The handler with the applied middlewares.
func applyMiddlewares(dito *app.Dito, handler http.Handler, location config.LocationConfig, plugins []plugin.Plugin) http.Handler {
	// Define which middlewares are critical (must be present)
	criticalMiddlewares := map[string]bool{
		"auth":     true,
		"security": true,
	}

	var missingCritical []string

	// Apply middlewares in reverse order (last configured is innermost)
	for i := len(location.Middlewares) - 1; i >= 0; i-- {
		middlewareName := location.Middlewares[i]
		applied := false

		// Search for middleware in loaded plugins
		for _, p := range plugins {
			if p.Name() == middlewareName {
				if mw := p.MiddlewareFunc(); mw != nil {
					handler = mw(handler)
					applied = true
					break
				}
			}
		}

		// Track missing middlewares
		if !applied {
			if critical := criticalMiddlewares[middlewareName]; critical {
				missingCritical = append(missingCritical, middlewareName)
			}
			dito.Logger.Warn("Middleware not found", "name", middlewareName)
		}
	}

	// Block requests if critical middlewares are missing
	if len(missingCritical) > 0 {
		return createBlockingHandler(missingCritical)
	}

	return handler
}

// createBlockingHandler creates a handler that blocks requests due to missing critical components.
//
// Parameters:
// - missingMiddlewares: List of missing critical middleware names.
//
// Returns:
// - http.HandlerFunc: Handler that returns an error response.
func createBlockingHandler(missingMiddlewares []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sendError(w, r, http.StatusInternalServerError, "Service configuration error", map[string]interface{}{
			"missing_components": missingMiddlewares,
		})
	}
}

// responseLimitInterceptor wraps a ResponseWriter to intercept responses that exceed size limits
type responseLimitInterceptor struct {
	http.ResponseWriter
	mu              sync.Mutex
	maxSize         int64
	written         int64
	limitExceeded   bool
	headerWritten   bool
	dito            *app.Dito
	path            string
	buffer          *bytes.Buffer
	statusCode      int
	originalHeaders http.Header
}

// newResponseLimitInterceptor creates a new response limit interceptor.
//
// Parameters:
// - w: The underlying response writer.
// - maxSize: Maximum allowed response size in bytes.
// - dito: The Dito application instance.
// - path: The request path (for logging).
//
// Returns:
// - *responseLimitInterceptor: The configured interceptor.
func newResponseLimitInterceptor(w http.ResponseWriter, maxSize int64, dito *app.Dito, path string) *responseLimitInterceptor {
	return &responseLimitInterceptor{
		ResponseWriter:  w,
		maxSize:         maxSize,
		dito:            dito,
		path:            path,
		buffer:          new(bytes.Buffer),
		statusCode:      http.StatusOK,
		originalHeaders: make(http.Header),
	}
}

// WriteHeader intercepts status code and checks Content-Length against limits.
func (rli *responseLimitInterceptor) WriteHeader(statusCode int) {
	rli.mu.Lock()
	defer rli.mu.Unlock()

	if rli.headerWritten {
		return
	}

	// Store original headers
	for k, v := range rli.Header() {
		rli.originalHeaders[k] = v
	}

	rli.statusCode = statusCode

	// Check Content-Length header against limit
	if rli.checkContentLength() {
		rli.sendLimitExceededError()
		return
	}

	// Don't write headers immediately - let Write() handle it
	// This prevents the issue where headers are written before we can check the content size
}

// Write buffers response data and enforces size limits.
func (rli *responseLimitInterceptor) Write(b []byte) (int, error) {
	rli.mu.Lock()
	defer rli.mu.Unlock()

	if rli.limitExceeded {
		return 0, ErrResponseTooLarge
	}

	// If this is the first write, store headers and check content-length
	if !rli.headerWritten {
		// Store original headers
		for k, v := range rli.Header() {
			rli.originalHeaders[k] = v
		}

		// Check Content-Length header against limit before writing anything
		if rli.checkContentLength() {
			rli.sendLimitExceededError()
			return 0, ErrResponseTooLarge
		}
	}

	// Check if this write would exceed the limit
	newTotal := rli.written + int64(len(b))
	if rli.maxSize > 0 && newTotal > rli.maxSize {
		rli.limitExceeded = true

		// If headers haven't been written yet, we can send our error response
		if !rli.headerWritten {
			rli.sendLimitExceededError()
			return 0, ErrResponseTooLarge
		}

		// If headers were already written, we can't change the response
		// This is a rare edge case where the upstream sends a small content-length
		// but then sends more data than advertised
		return 0, ErrResponseTooLarge
	}

	// Buffer the data
	rli.buffer.Write(b)
	rli.written = newTotal

	// Always flush immediately to avoid chunking issues
	if !rli.headerWritten {
		rli.flushBuffer()
	}

	return len(b), nil
}

// Flush ensures any buffered data is written to the client.
func (rli *responseLimitInterceptor) Flush() {
	rli.mu.Lock()
	defer rli.mu.Unlock()

	if !rli.limitExceeded && !rli.headerWritten && rli.buffer.Len() > 0 {
		rli.flushBuffer()
	}

	if f, ok := rli.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// checkContentLength checks if Content-Length header exceeds the limit.
func (rli *responseLimitInterceptor) checkContentLength() bool {
	if cl := rli.originalHeaders.Get("Content-Length"); cl != "" {
		if length, err := strconv.ParseInt(cl, 10, 64); err == nil && length > rli.maxSize {
			rli.limitExceeded = true
			// Log warning about Content-Length exceeding limit
			rli.dito.Logger.Warn("Response Content-Length exceeds limit",
				"path", rli.path,
				"content_length", length,
				"limit_bytes", rli.maxSize,
			)
			return true
		}
	}
	return false
}

// flushBuffer writes buffered data to the underlying response writer.
func (rli *responseLimitInterceptor) flushBuffer() {
	if rli.headerWritten {
		return
	}

	rli.headerWritten = true

	// Restore original headers to the underlying response writer
	for k, v := range rli.originalHeaders {
		rli.ResponseWriter.Header()[k] = v
	}

	// If we have buffered content, set proper Content-Length to avoid chunking
	if rli.buffer.Len() > 0 {
		rli.ResponseWriter.Header().Set("Content-Length", strconv.Itoa(rli.buffer.Len()))
	}

	rli.ResponseWriter.WriteHeader(rli.statusCode)

	if rli.buffer.Len() > 0 {
		io.Copy(rli.ResponseWriter, rli.buffer)
		rli.buffer.Reset()
	}
}

// sendLimitExceededError sends a clean error response when size limit is exceeded.
func (rli *responseLimitInterceptor) sendLimitExceededError() {
	if rli.headerWritten {
		return
	}

	// Log warning about response size limit exceeded
	rli.dito.Logger.Warn("Response size limit exceeded",
		"path", rli.path,
		"limit_bytes", rli.maxSize,
		"actual_bytes", rli.written,
	)

	rli.buffer.Reset()
	rli.headerWritten = true

	// Clear existing headers
	for k := range rli.Header() {
		delete(rli.Header(), k)
	}

	// Create JSON error response
	errorResponse := fmt.Sprintf(`{"error":{"code":%d,"message":"%s","details":{"limit_bytes":%d,"path":"%s"}}}`,
		http.StatusRequestEntityTooLarge, "Response body size exceeds limit", rli.maxSize, rli.path)

	// Set error response headers
	rli.ResponseWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
	rli.ResponseWriter.Header().Set("X-Content-Type-Options", "nosniff")
	rli.ResponseWriter.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	rli.ResponseWriter.Header().Set("Content-Length", fmt.Sprintf("%d", len(errorResponse)))

	// Write status code and response
	rli.ResponseWriter.WriteHeader(http.StatusRequestEntityTooLarge)
	rli.ResponseWriter.Write([]byte(errorResponse))
}

// enrichContext adds request metadata to the context.
//
// Parameters:
// - ctx: The base context.
// - r: The HTTP request.
//
// Returns:
// - context.Context: The enriched context.
func enrichContext(ctx context.Context, r *http.Request) context.Context {
	// Generate or extract request ID
	requestID := r.Header.Get(headerXRequestID)
	if requestID == "" {
		requestID = generateRequestID()
	}

	ctx = context.WithValue(ctx, ctxKeyRequestID, requestID)
	ctx = context.WithValue(ctx, ctxKeyStartTime, time.Now())

	// Store original host and protocol
	originalHost := r.Host
	if originalHost == "" {
		originalHost = r.URL.Host
	}

	originalProto := "http"
	if r.TLS != nil {
		originalProto = "https"
	}
	if proto := r.Header.Get(headerXForwardedProto); proto != "" {
		originalProto = proto
	}

	ctx = context.WithValue(ctx, ctxKeyOriginalHost, originalHost)
	ctx = context.WithValue(ctx, ctxKeyOriginalProto, originalProto)

	return ctx
}

// validateRequest performs basic request validation.
//
// Parameters:
// - w: The HTTP response writer.
// - r: The HTTP request.
// - dito: The Dito application instance.
//
// Returns:
// - bool: True if request is valid, false otherwise.
func validateRequest(w http.ResponseWriter, r *http.Request, dito *app.Dito) bool {
	// Check request body size
	if r.ContentLength > maxRequestBodySize {
		sendError(w, r, http.StatusRequestEntityTooLarge, "Request body too large", nil)
		return false
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		sendError(w, r, http.StatusBadRequest, "Invalid request format", nil)
		return false
	}

	return true
}

// parseAndValidateURL parses and validates a target URL.
//
// Parameters:
// - targetURL: The URL string to parse.
//
// Returns:
// - *url.URL: The parsed URL.
// - error: An error if parsing or validation fails.
func parseAndValidateURL(targetURL string) (*url.URL, error) {
	u, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	if u.Scheme == "" || u.Host == "" {
		return nil, ErrInvalidTarget
	}

	return u, nil
}

// createResponseWriter creates a custom response writer with location-specific configuration.
//
// Parameters:
// - w: The underlying response writer.
// - location: The location configuration.
// - cfg: The proxy configuration.
//
// Returns:
// - *writer.ResponseWriter: The configured response writer.
func createResponseWriter(w http.ResponseWriter, location config.LocationConfig, cfg *config.ProxyConfig) *writer.ResponseWriter {
	opts := []writer.WriterOption{}

	// Disable buffering for health checks
	if isHealthCheck(location.Path) {
		opts = append(opts, writer.WithBuffering(false))
	}

	// Apply location-specific buffering settings
	if location.DisableResponseBuffering {
		opts = append(opts, writer.WithBuffering(false))
	}

	// Disable response body size limit in the writer - we handle this with responseLimitInterceptor
	// which provides better error responses with proper JSON formatting and proper 413 status code
	opts = append(opts, writer.WithMaxResponseBodySize(0)) // 0 = unlimited in writer

	return writer.NewResponseWriter(w, opts...)
}

// addSecurityHeaders adds security-related headers to the response.
//
// Parameters:
// - resp: The HTTP response.
func addSecurityHeaders(resp *http.Response) {
	resp.Header.Set("X-Content-Type-Options", "nosniff")
	resp.Header.Set("X-Frame-Options", "DENY")
	resp.Header.Set("Referrer-Policy", "strict-origin-when-cross-origin")

	// Add HSTS if not already present
	if resp.Header.Get("Strict-Transport-Security") == "" {
		resp.Header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}

// sanitizeHeaders removes hop-by-hop headers that shouldn't be forwarded.
//
// Parameters:
// - req: The HTTP request.
func sanitizeHeaders(req *http.Request) {
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Proxy-Authenticate")
	req.Header.Del("Proxy-Authorization")
	req.Header.Del("Connection")
}

// sendError sends a standardized JSON error response.
//
// Parameters:
// - w: The HTTP response writer.
// - r: The HTTP request (can be nil).
// - code: The HTTP status code.
// - message: The error message.
// - details: Additional error details.
func sendError(w http.ResponseWriter, r *http.Request, code int, message string, details map[string]interface{}) {
	// Create error response
	resp := errorResponse{
		Timestamp: time.Now().Unix(),
	}
	resp.Error.Code = code
	resp.Error.Message = message
	resp.Error.Details = details

	// Add request ID if available
	var requestID string
	if r != nil {
		requestID = getRequestID(r.Context())
		if requestID != "" {
			resp.RequestID = requestID
		}
	}

	// Marshal the response to get the exact size
	responseData, err := json.Marshal(resp)
	if err != nil {
		// Fallback to simple error if JSON marshaling fails
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Content-Length", strconv.Itoa(len(responseData)))

	// Add request ID if available
	if requestID != "" {
		w.Header().Set(headerXRequestID, requestID)
	}

	// Write status code and response
	w.WriteHeader(code)
	w.Write(responseData)
}

// recordMetrics records request metrics for monitoring.
//
// Parameters:
// - r: The HTTP request.
// - rw: The response writer (for response metrics).
// - duration: Request processing duration.
func recordMetrics(r *http.Request, rw *writer.ResponseWriter, duration time.Duration) {
	if rw == nil {
		return
	}

	m := rw.GetMetrics()
	metrics.RecordRequest(r.Method, r.URL.Path, m.StatusCode, duration.Seconds())

	if r.ContentLength > 0 {
		metrics.RecordDataTransferred("inbound", int(r.ContentLength))
	}
	if m.BytesWritten > 0 {
		metrics.RecordDataTransferred("outbound", int(m.BytesWritten))
	}
}

// joinPaths safely joins URL path segments.
//
// Parameters:
// - base: The base path.
// - additional: The path to append.
//
// Returns:
// - string: The joined path.
func joinPaths(base, additional string) string {
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(additional, "/")
}

// isHealthCheck determines if the path is a health check endpoint.
//
// Parameters:
// - path: The URL path.
//
// Returns:
// - bool: True if it's a health check path.
func isHealthCheck(path string) bool {
	return path == "/health" || path == "/healthz" || path == "/ready" || path == "/readyz"
}

// getTimeout returns the configured request timeout or default.
//
// Parameters:
// - cfg: The proxy configuration.
//
// Returns:
// - time.Duration: The timeout duration.
func getTimeout(cfg *config.ProxyConfig) time.Duration {
	if cfg.RequestTimeout > 0 {
		return cfg.RequestTimeout
	}
	return defaultTimeout
}

// getRequestID extracts the request ID from context.
//
// Parameters:
// - ctx: The context.
//
// Returns:
// - string: The request ID, or empty string if not found.
func getRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return id
	}
	return ""
}

// getStartTime extracts the request start time from context.
//
// Parameters:
// - ctx: The context.
//
// Returns:
// - time.Time: The start time, or zero time if not found.
func getStartTime(ctx context.Context) time.Time {
	if t, ok := ctx.Value(ctxKeyStartTime).(time.Time); ok {
		return t
	}
	return time.Time{}
}

// generateRequestID generates a unique request ID.
//
// Returns:
// - string: The generated request ID.
func generateRequestID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), fastRand())
}

// randPool provides a pool of random number generators for performance.
var randPool = sync.Pool{
	New: func() interface{} {
		v := time.Now().UnixNano()
		return &v
	},
}

// fastRand generates a fast pseudo-random number.
//
// Returns:
// - int64: A pseudo-random number.
func fastRand() int64 {
	vPtr := randPool.Get().(*int64)
	*vPtr = *vPtr*1103515245 + 12345
	result := *vPtr
	randPool.Put(vPtr)
	return result
}

// bufferPool implements httputil.BufferPool for efficient buffer reuse
type bufferPool struct {
	pool sync.Pool
}

// bufferWrapper wraps a byte slice to avoid allocation warnings
type bufferWrapper struct {
	data []byte
}

// newBufferPool creates a new buffer pool for the reverse proxy.
//
// Returns:
// - httputil.BufferPool: The buffer pool implementation.
func newBufferPool() httputil.BufferPool {
	return &bufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &bufferWrapper{data: make([]byte, 32*1024)}
			},
		},
	}
}

// Get retrieves a buffer from the pool.
func (bp *bufferPool) Get() []byte {
	wrapper := bp.pool.Get().(*bufferWrapper)
	return wrapper.data
}

// Put returns a buffer to the pool.
func (bp *bufferPool) Put(b []byte) {
	if cap(b) == 32*1024 {
		wrapper := &bufferWrapper{data: b}
		bp.pool.Put(wrapper)
	}
}
