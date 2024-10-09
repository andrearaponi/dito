package middlewares

import (
	"bytes"
	"dito/app"
	"dito/logging"
	"dito/writer"
	"io"
	"net/http"
	"time"
)

// LoggingMiddleware is an HTTP middleware that logs requests and responses.
// It logs the request body, headers, response status code, and duration of the request.
//
// Parameters:
// - next: The next http.Handler to be called.
// - dito: The Dito application instance containing the configuration and logger.
//
// Returns:
// - http.Handler: A handler that logs requests and responses based on the provided configuration.
func LoggingMiddleware(next http.Handler, dito *app.Dito) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var bodyBytes []byte
		if r.Body != nil {
			bodyBytes, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Restore the request body.
		}

		lrw := &writer.ResponseWriter{ResponseWriter: w}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		headers := r.Header.Clone()

		if dito.Config.Logging.Enabled && dito.Config.Logging.Verbose {
			logging.LogRequestVerbose(r, &bodyBytes, (*map[string][]string)(&headers), lrw.StatusCode, duration)
		} else {
			logging.LogRequestCompact(r, &bodyBytes, (*map[string][]string)(&r.Header), lrw.StatusCode, duration)
		}
	})
}
