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

// logEntry represents a log entry with details about the HTTP request and response.
type logEntry struct {
	Dito         *app.Dito     // Reference to the Dito application instance.
	Request      *http.Request // The HTTP request.
	BodyBytes    []byte        // The body of the HTTP request.
	Headers      http.Header   // The headers of the HTTP request.
	StatusCode   int           // The status code of the HTTP response.
	Duration     time.Duration // The duration of the HTTP request processing.
	BytesWritten int           // The number of bytes written in the HTTP response.
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
	if entry.Dito.Config.Logging.Enabled && entry.Dito.Config.Logging.Verbose {
		logging.LogRequestVerbose(entry.Request, entry.BodyBytes, entry.Headers, entry.StatusCode, entry.Duration)
	} else {
		logging.LogRequestCompact(entry.Request, entry.BodyBytes, entry.Headers, entry.StatusCode, entry.Duration)
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

		var bodyBytes []byte
		if r.Body != nil {
			const MaxBodySize = 1024
			limitedReader := io.LimitReader(r.Body, MaxBodySize)
			bodyBytes, _ = io.ReadAll(limitedReader)
			r.Body = io.NopCloser(io.MultiReader(bytes.NewBuffer(bodyBytes), r.Body))
		}

		lrw := &writer.ResponseWriter{ResponseWriter: w}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)

		if dito.Config.Metrics.Enabled {
			metrics.RecordRequest(r.Method, r.URL.Path, lrw.StatusCode, float64(duration.Seconds()))
			metrics.RecordDataTransferred("inbound", int(r.ContentLength))
			metrics.RecordDataTransferred("outbound", lrw.BytesWritten)
		}

		select {
		case logChannel <- logEntry{
			Dito:         dito,
			Request:      r,
			BodyBytes:    bodyBytes,
			Headers:      r.Header,
			StatusCode:   lrw.StatusCode,
			Duration:     duration,
			BytesWritten: lrw.BytesWritten,
		}:
		default:
			dito.Logger.Warn("Log channel is full, discarding log entry")
		}
	})
}
