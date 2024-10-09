package logging

import (
	"dito/writer"
	"fmt"
	"github.com/fatih/color"
	"github.com/lmittmann/tint"
	"log/slog"
	"net/http"
	"os"
	"time"
)

var logger *slog.Logger

// InitializeLogger initializes a new logger with the specified log level.
func InitializeLogger(level string) *slog.Logger {
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
	return slog.New(handler)
}

// GetLogger returns the global logger instance.
func GetLogger() *slog.Logger {
	if logger == nil {
		// Initialize with a default level in case the logger wasn't set up
		logger = InitializeLogger("info")
	}
	return logger
}

// LogRequestVerbose logs detailed information about the HTTP request and response for debugging purposes.
//
// Parameters:
// - req: The HTTP request to be logged.
// - body: A pointer to the byte slice containing the request body.
// - headers: A pointer to the map containing the request headers.
// - statusCode: The HTTP status code of the response.
// - duration: The duration of the request.
func LogRequestVerbose(req *http.Request, body *[]byte, headers *map[string][]string, statusCode int, duration time.Duration) {
	// Define styles for color
	methodStyle := color.New(color.FgHiWhite, color.BgGreen).SprintFunc()
	detailStyle := color.New(color.FgHiWhite, color.BgRed).PrintlnFunc()
	boldWhiteStyle := color.New(color.FgWhite, color.Bold).SprintFunc()
	urlStyle := color.New(color.FgHiWhite, color.BgHiCyan).SprintFunc()
	headersStyle := color.New(color.FgHiWhite, color.BgHiMagenta).PrintlnFunc()
	statusStyle := color.New(color.FgHiWhite, color.BgYellow).SprintFunc()

	// Start logging the request
	fmt.Println()
	detailStyle("----------- Request Details -----------")
	fmt.Println()
	fmt.Printf("%s: %s\n\n", methodStyle("Method:"), boldWhiteStyle(req.Method))
	fmt.Printf("%s: %s\n\n", urlStyle("URL:"), boldWhiteStyle(req.URL))

	headersStyle("Request Headers:")
	for name, headersToPrint := range *headers {
		for _, h := range headersToPrint {
			fmt.Printf("\t%s: %s\n", boldWhiteStyle(name), h)
		}
	}

	fmt.Println()
	fmt.Printf("%s\n \t%s\n\n", urlStyle("Request Body:"), string(*body))

	// Response details
	detailStyle("----------- Response Details -----------")
	fmt.Println()
	fmt.Printf("%s: %d\n\n", statusStyle("Status Code:"), statusCode)
	fmt.Printf("%s: %f seconds\n\n", boldWhiteStyle("Response Time:"), duration.Seconds())

	detailStyle("---------------------------------------")
}

// LogRequestCompact logs the HTTP request and response in a compact format.
//
// Parameters:
// - r: The HTTP request to be logged.
// - body: A pointer to the byte slice containing the request body.
// - headers: A pointer to the map containing the request headers.
// - statusCode: The HTTP status code of the response.
// - duration: The duration of the request.
func LogRequestCompact(r *http.Request, body *[]byte, headers *map[string][]string, statusCode int, duration time.Duration) {
	logger := GetLogger()
	clientIP := r.RemoteAddr
	method := r.Method
	url := r.URL.Path
	protocol := r.Proto
	userAgent := r.Header.Get("User-Agent")
	referer := r.Header.Get("Referer")

	logger.Info(fmt.Sprintf("%s - - \"%s %s %s\" %d \"%s\" \"%s\" %f seconds",
		clientIP,
		method,
		url,
		protocol,
		statusCode,
		referer,
		userAgent,
		duration.Seconds(),
	))
}

func LogResponse(lrw *writer.ResponseWriter) {
	detailStyle := color.New(color.FgHiWhite, color.BgRed).PrintlnFunc()
	responseTimeStyle := color.New(color.FgHiWhite, color.BgHiYellow).SprintFunc()
	boldWhiteStyle := color.New(color.FgWhite, color.Bold).SprintFunc()
	headersStyle := color.New(color.FgHiWhite, color.BgHiMagenta).PrintlnFunc()

	fmt.Println()
	detailStyle("----------- Response Details ----------")
	fmt.Println()
	fmt.Printf("%s: %d\n\n", responseTimeStyle("Status Code:"), lrw.StatusCode)

	headersStyle("Headers:")
	for name, values := range lrw.Header() {
		for _, value := range values {
			fmt.Printf("\t%s: %s\n", boldWhiteStyle(name), value)
		}
	}
	fmt.Println()
	fmt.Printf("%s \t%s", responseTimeStyle("Body:"), string(lrw.Body.Bytes()))
	fmt.Println()
	detailStyle("--------------------------------------")
}
