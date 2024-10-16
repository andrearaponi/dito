package middlewares

import (
	"bytes"
	"dito/app"
	"dito/logging"
	"dito/metrics"
	"dito/writer"
	"io"
	"net/http"
	"time"
)

// LoggingMiddleware is an HTTP middleware that logs requests and responses.
// It logs the request body, headers, response status code, and duration of the request.
// It also tracks metrics such as active connections and data transferred.
//
// Parameters:
// - next: The next http.Handler to be called.
// - dito: The Dito application instance containing the configuration and logger.
//
// Returns:
// - http.Handler: A handler that logs requests, responses, and metrics based on the provided configuration.
func LoggingMiddleware(next http.Handler, dito *app.Dito) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Increment the active connections metric if metrics are enabled
		if dito.Config.Metrics.Enabled {
			metrics.UpdateActiveConnections(true)
			defer metrics.UpdateActiveConnections(false) // Ensure decrement after the request is processed
		}

		var bodyBytes []byte
		if r.Body != nil {
			// Read the request body and store it in bodyBytes
			bodyBytes, _ = io.ReadAll(r.Body)
			// Restore the request body so it can be read again
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Create a custom ResponseWriter to capture the response status code and bytes written
		lrw := &writer.ResponseWriter{ResponseWriter: w}

		// Call the next handler in the chain
		next.ServeHTTP(lrw, r)

		// Calculate the duration of the request
		duration := time.Since(start)
		// Clone the request headers
		headers := r.Header.Clone()

		// Record the request metrics if metrics are enabled in the configuration
		// This includes the HTTP method, URL path, response status code, and request duration
		if dito.Config.Metrics.Enabled {
			metrics.RecordRequest(r.Method, r.URL.Path, lrw.StatusCode, float64(duration.Seconds()))
			// Record data transferred for inbound and outbound traffic
			metrics.RecordDataTransferred("inbound", int(r.ContentLength))
			metrics.RecordDataTransferred("outbound", lrw.BytesWritten)
		}

		// Log the request based on the logging configuration
		if dito.Config.Logging.Enabled && dito.Config.Logging.Verbose {
			logging.LogRequestVerbose(r, &bodyBytes, (*map[string][]string)(&headers), lrw.StatusCode, duration)
		} else {
			logging.LogRequestCompact(r, &bodyBytes, (*map[string][]string)(&r.Header), lrw.StatusCode, duration)
		}
	})
}
