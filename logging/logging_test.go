package logging

import (
	"bytes"
	"github.com/lmittmann/tint"
	"log/slog"
	"os"

	"net/http"
	"testing"
	"time"
)

// TestLogRequestVerbose tests the LogRequestVerbose function.
// It creates a sample HTTP request and logs it verbosely with headers, status code, and duration.
func TestLogRequestVerbose(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", bytes.NewBuffer([]byte("request body")))
	headers := map[string][]string{
		"Content-Type": {"application/json"},
	}
	statusCode := 200
	duration := 2 * time.Second

	body := []byte("request body")
	LogRequestVerbose(req, &body, &headers, statusCode, duration)
}

// TestLogRequestCompact tests the LogRequestCompact function.
// It creates a sample HTTP request and logs it compactly with headers, status code, and duration.
func TestLogRequestCompact(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", bytes.NewBuffer([]byte("request body")))
	headers := map[string][]string{
		"Content-Type": {"application/json"},
	}
	statusCode := 200
	duration := 2 * time.Second

	body := []byte("request body")
	LogRequestCompact(req, &body, &headers, statusCode, duration)
}

// InitializeLogger initializes a new logger with the specified log level.
func initializeLogger(level string) *slog.Logger {
	if logger != nil {
		return logger
	}

	levelVar := new(slog.LevelVar)

	// Set the log level based on the provided string
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo // Default to info if unrecognized
	}

	levelVar.Set(logLevel)

	handler := tint.NewHandler(os.Stdout, &tint.Options{Level: levelVar})
	logger = slog.New(handler)

	return logger
}
