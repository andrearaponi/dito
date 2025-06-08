package middlewares

import (
	"bytes"
	"dito/app"
	"dito/logging"
	"dito/metrics"
	"dito/writer"
	"io"
	"net/http"
	"strings"
	"time"
)

// logEntry represents a log entry with details about the HTTP request and response.
type logEntry struct {
	Dito            *app.Dito              // Reference to the Dito application instance.
	Request         *http.Request          // The HTTP request.
	BodyBytes       []byte                 // The body of the HTTP request.
	Headers         http.Header            // The headers of the HTTP request.
	StatusCode      int                    // The status code of the HTTP response.
	Duration        time.Duration          // The duration of the HTTP request processing.
	BytesWritten    int64                  // The number of bytes written in the HTTP response.
	ResponseMetrics writer.ResponseMetrics // Detailed response metrics
}

// Global log channel
var logChannel = make(chan logEntry, 10000)

// Number of worker goroutines for logging
const numLogWorkers = 5

// init initializes the logging workers.
func init() {
	// Start multiple goroutines for logging
	for i := 0; i < numLogWorkers; i++ {
		go func() {
			for entry := range logChannel {
				processLogEntry(entry)
			}
		}()
	}
}

// processLogEntry processes a log entry and logs it based on the configuration.
func processLogEntry(entry logEntry) {
	// Log basic request info
	if entry.Dito.Config.Logging.Enabled && entry.Dito.Config.Logging.Verbose {
		logging.LogRequestVerbose(entry.Dito.Logger, entry.Request, entry.BodyBytes, entry.Headers, entry.StatusCode, entry.Duration)
	} else {
		logging.LogRequestCompact(entry.Dito.Logger, entry.Request, entry.BodyBytes, entry.Headers, entry.StatusCode, entry.Duration)
	}

	// Log additional response metrics if available
	if entry.ResponseMetrics.IsBufferTruncated {
		entry.Dito.Logger.Warn("Response body was truncated",
			"path", entry.Request.URL.Path,
			"content_type", entry.ResponseMetrics.ContentType,
			"buffered_bytes", entry.ResponseMetrics.BufferedBytes,
			"total_bytes", entry.ResponseMetrics.BytesWritten,
		)
	}

	if entry.ResponseMetrics.IsStreaming {
		entry.Dito.Logger.Debug("Response was streamed",
			"path", entry.Request.URL.Path,
			"content_type", entry.ResponseMetrics.ContentType,
			"bytes", entry.ResponseMetrics.BytesWritten,
		)
	}

	// Log response body size limit violations
	if entry.ResponseMetrics.IsResponseLimitHit {
		entry.Dito.Logger.Warn("Response body size limit exceeded",
			"path", entry.Request.URL.Path,
			"content_type", entry.ResponseMetrics.ContentType,
			"limit_bytes", entry.ResponseMetrics.MaxResponseBodySize,
			"attempted_bytes", entry.ResponseMetrics.BytesWritten,
			"error", entry.ResponseMetrics.ResponseLimitError,
		)
	}
}

// LoggingMiddleware is an HTTP middleware that logs the details of each request and response.
//
// Parameters:
// - next: The next HTTP handler in the chain.
// - dito: The Dito application instance.
//
// Returns:
// - http.Handler: The HTTP handler with logging functionality.
func LoggingMiddleware(next http.Handler, dito *app.Dito) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if dito.Config.Metrics.Enabled {
			metrics.UpdateActiveConnections(true)
			defer metrics.UpdateActiveConnections(false)
		}

		// Read request body for logging (limited to prevent memory issues)
		var bodyBytes []byte
		if r.Body != nil {
			const MaxBodySize = 1024
			limitedReader := io.LimitReader(r.Body, MaxBodySize)
			bodyBytes, _ = io.ReadAll(limitedReader)
			r.Body = io.NopCloser(io.MultiReader(bytes.NewBuffer(bodyBytes), r.Body))
		}

		// Create the new ResponseWriter with appropriate options including response size limits
		lrw := createResponseWriterWithLimits(w, r, dito)

		// Serve the request
		next.ServeHTTP(lrw, r)

		duration := time.Since(start)

		// Get response metrics
		responseMetrics := lrw.GetMetrics()

		// Record metrics if enabled
		if dito.Config.Metrics.Enabled {
			metrics.RecordRequest(r.Method, r.URL.Path, responseMetrics.StatusCode, float64(duration.Seconds()))
			metrics.RecordDataTransferred("inbound", int(r.ContentLength))
			metrics.RecordDataTransferred("outbound", int(responseMetrics.BytesWritten))

			// Additional metrics for streaming and truncation
			if responseMetrics.IsStreaming {
				// You could add a streaming_responses metric here
			}
			if responseMetrics.IsBufferTruncated {
				// You could add a truncated_responses metric here
			}
			if responseMetrics.IsResponseLimitHit {
				// You could add a response_limit_exceeded metric here
			}
		}

		// Send log entry
		select {
		case logChannel <- logEntry{
			Dito:            dito,
			Request:         r,
			BodyBytes:       bodyBytes,
			Headers:         r.Header,
			StatusCode:      responseMetrics.StatusCode,
			Duration:        duration,
			BytesWritten:    responseMetrics.BytesWritten,
			ResponseMetrics: responseMetrics,
		}:
		default:
			dito.Logger.Warn("Log channel is full, discarding log entry")
		}
	})
}

// createResponseWriterWithLimits creates a ResponseWriter with appropriate configuration based on the request and response limits
func createResponseWriterWithLimits(w http.ResponseWriter, r *http.Request, dito *app.Dito) *writer.ResponseWriter {
	// Default options
	opts := []writer.WriterOption{}

	// Determine buffer size based on configuration or request type
	bufferSize := writer.DefaultMaxBufferSize

	// Disable response body size limit in the writer - we handle this in the handler interceptor
	// which provides better error responses with proper JSON formatting
	opts = append(opts, writer.WithMaxResponseBodySize(0)) // 0 = unlimited in writer

	// You can customize buffer size based on path, headers, etc.
	// For example, smaller buffer for health checks
	if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
		opts = append(opts, writer.WithBuffering(false))
	}

	// Or based on expected content type from Accept header
	accept := r.Header.Get("Accept")
	switch {
	case strings.Contains(accept, "application/json"):
		// JSON APIs might need larger buffers for debugging
		bufferSize = 5 * 1024 * 1024 // 5MB
	case strings.Contains(accept, "image/") || strings.Contains(accept, "video/"):
		// Media requests probably don't need buffering
		bufferSize = 64 * 1024 // 64KB
	}

	// Apply custom buffer size if different from default
	if bufferSize != writer.DefaultMaxBufferSize {
		opts = append(opts, writer.WithMaxBufferSize(bufferSize))
	}

	// You could also check for specific headers or paths that indicate
	// file downloads and adjust accordingly
	if isFileDownloadPath(r.URL.Path) {
		opts = append(opts, writer.WithMaxBufferSize(64*1024)) // Minimal buffering
	}

	return writer.NewResponseWriter(w, opts...)
}

// createResponseWriter creates a ResponseWriter with appropriate configuration based on the request
// This is the old function name - keeping it for backward compatibility but it now delegates to the new function
func createResponseWriter(w http.ResponseWriter, r *http.Request, dito *app.Dito) *writer.ResponseWriter {
	return createResponseWriterWithLimits(w, r, dito)
}

// isFileDownloadPath checks if the path is likely a file download
func isFileDownloadPath(path string) bool {
	// Add your logic here
	// For example:
	return strings.HasPrefix(path, "/download/") ||
		strings.HasPrefix(path, "/files/") ||
		strings.HasSuffix(path, ".pdf") ||
		strings.HasSuffix(path, ".zip")
}
