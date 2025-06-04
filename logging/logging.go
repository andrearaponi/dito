package logging

import (
	"dito/writer"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gorilla/websocket"
	"github.com/lmittmann/tint"
)

var logger *slog.Logger

// Predefined styles for formatting log messages using the `color` package.
var (
	methodStyle       = color.New(color.FgHiWhite, color.BgGreen).SprintFunc()     // methodStyle formats HTTP methods.
	detailStyle       = color.New(color.FgHiWhite, color.BgRed).SprintFunc()       // detailStyle formats detailed log sections.
	boldWhiteStyle    = color.New(color.FgWhite, color.Bold).SprintFunc()          // boldWhiteStyle formats text in bold white.
	urlStyle          = color.New(color.FgHiWhite, color.BgHiCyan).SprintFunc()    // urlStyle formats URLs.
	headersStyle      = color.New(color.FgHiWhite, color.BgHiMagenta).SprintFunc() // headersStyle formats HTTP headers.
	statusStyle       = color.New(color.FgHiWhite, color.BgYellow).SprintFunc()    // statusStyle formats HTTP status codes.
	responseTimeStyle = color.New(color.FgHiWhite, color.BgHiYellow).SprintFunc()  // responseTimeStyle formats response times.
	warningStyle      = color.New(color.FgHiWhite, color.BgMagenta).SprintFunc()   // warningStyle formats warnings.
)

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
func LogRequestVerbose(logger *slog.Logger, req *http.Request, body []byte, headers http.Header, statusCode int, duration time.Duration) {
	if logger == nil {
		logger = GetLogger() // Fallback to global if nil, though tests should provide one.
	}
	var sb strings.Builder

	// Start building the log message
	sb.WriteString("\n")
	sb.WriteString(detailStyle("----------- Request Details -----------"))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("%s: %s\n\n", methodStyle("Method:"), boldWhiteStyle(req.Method)))
	sb.WriteString(fmt.Sprintf("%s: %s\n\n", urlStyle("URL:"), boldWhiteStyle(req.URL.String())))

	sb.WriteString(headersStyle("Request Headers:"))
	sb.WriteString("\n")
	for name, values := range headers {
		for _, h := range values {
			sb.WriteString(fmt.Sprintf("\t%s: %s\n", boldWhiteStyle(name), h))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%s\n\t%s\n\n", urlStyle("Request Body:"), string(body)))

	// Response details
	sb.WriteString(detailStyle("----------- Response Details -----------"))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("%s: %d\n\n", statusStyle("Status Code:"), statusCode))
	sb.WriteString(fmt.Sprintf("%s: %.6f seconds\n\n", boldWhiteStyle("Response Time:"), duration.Seconds()))

	sb.WriteString(detailStyle("---------------------------------------"))

	// Log the formatted string through the logger at Debug level.
	logger.Debug("Verbose request details", slog.String("formatted_output", sb.String()))
}

// LogRequestCompact logs the HTTP request and response in a compact format using structured logging.
func LogRequestCompact(logger *slog.Logger, r *http.Request, body []byte, headers http.Header, statusCode int, duration time.Duration) {
	if logger == nil {
		logger = GetLogger() // Fallback to global if nil.
	}
	clientIP := r.RemoteAddr
	method := r.Method
	url := r.URL.Path
	protocol := r.Proto
	userAgent := r.Header.Get("User-Agent")
	referer := r.Header.Get("Referer")

	logger.Info("HTTP request processed",
		slog.String("client_ip", clientIP),
		slog.String("method", method),
		slog.String("url", url),
		slog.String("protocol", protocol),
		slog.Int("status_code", statusCode),
		slog.String("referer", referer),
		slog.String("user_agent", userAgent),
		slog.Float64("duration_seconds", duration.Seconds()),
	)
}

// LogWebSocketMessage logs the details of a WebSocket message using structured logging.
func LogWebSocketMessage(logger *slog.Logger, messageType int, message []byte, err error, duration time.Duration) {
	if logger == nil {
		logger = GetLogger() // Fallback to global if nil.
	}
	messageTypeStr := getMessageTypeString(messageType)

	// Details to log
	logAttributes := []slog.Attr{
		slog.String("type", messageTypeStr),
		slog.Float64("duration_seconds", duration.Seconds()),
	}

	// Log in case of error
	if err != nil {
		logAttributes = append(logAttributes, slog.String("error", err.Error()))
		logger.Error("WebSocket message processing error", attrsToAny(logAttributes)...)
		return
	}

	// Log message content based on type
	switch messageType {
	case websocket.TextMessage:
		logAttributes = append(logAttributes, slog.String("message_content", truncateMessage(message)))
		logger.Info("WebSocket text message received", attrsToAny(logAttributes)...)
	case websocket.PingMessage, websocket.PongMessage:
		logger.Debug("WebSocket ping/pong message received", attrsToAny(logAttributes)...)
	default: // Includes BinaryMessage, CloseMessage, etc.
		logAttributes = append(logAttributes, slog.Int("message_size_bytes", len(message)))
		logger.Info("WebSocket message received", attrsToAny(logAttributes)...)
	}
}

// attrsToAny converts a slice of slog.Attr to a slice of any for slog methods.
func attrsToAny(attrs []slog.Attr) []any {
	anys := make([]any, len(attrs))
	for i, attr := range attrs {
		anys[i] = attr
	}
	return anys
}

// Utility function to truncate very long messages
func truncateMessage(message []byte) string {
	const maxLength = 100
	if len(message) > maxLength {
		return string(message[:maxLength]) + "..."
	}
	return string(message)
}

// Utility function to get the message type description
func getMessageTypeString(messageType int) string {
	switch messageType {
	case websocket.TextMessage:
		return "Text"
	case websocket.BinaryMessage:
		return "Binary"
	case websocket.CloseMessage:
		return "Close"
	case websocket.PingMessage:
		return "Ping"
	case websocket.PongMessage:
		return "Pong"
	default:
		return "Unknown"
	}
}

// LogResponse logs the details of the HTTP response using the new ResponseWriter.
func LogResponse(logger *slog.Logger, lrw *writer.ResponseWriter) {
	if logger == nil {
		logger = GetLogger()
	}

	// Get response metrics
	metrics := lrw.GetMetrics()

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(detailStyle("----------- Response Details ----------"))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("%s: %d\n\n", responseTimeStyle("Status Code:"), metrics.StatusCode))

	// Add content type info
	if metrics.ContentType != "" {
		sb.WriteString(fmt.Sprintf("%s: %s\n\n", boldWhiteStyle("Content-Type:"), metrics.ContentType))
	}

	// Add bytes written info
	sb.WriteString(fmt.Sprintf("%s: %d bytes\n\n", boldWhiteStyle("Total Bytes Written:"), metrics.BytesWritten))

	sb.WriteString(headersStyle("Headers:"))
	sb.WriteString("\n")
	for name, values := range lrw.Header() {
		for _, value := range values {
			sb.WriteString(fmt.Sprintf("\t%s: %s\n", boldWhiteStyle(name), value))
		}
	}

	// Handle body display based on buffering status
	sb.WriteString("\n")
	if metrics.IsStreaming {
		sb.WriteString(fmt.Sprintf("%s: %s\n", responseTimeStyle("Body:"), "[STREAMING MODE - Body not buffered]"))
		sb.WriteString(fmt.Sprintf("%s: First %d bytes buffered before streaming\n", boldWhiteStyle("Note:"), metrics.BufferedBytes))
	} else if metrics.IsBufferTruncated {
		bodyStr := lrw.GetBufferedBodyString()
		sb.WriteString(fmt.Sprintf("%s:\n\t%s\n", responseTimeStyle("Body (Truncated):"), bodyStr))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("%s: Response body was truncated. Buffered %d bytes out of %d total bytes.\n",
			warningStyle("WARNING:"), metrics.BufferedBytes, metrics.BytesWritten))
	} else {
		bodyStr := lrw.GetBufferedBodyString()
		if bodyStr == "" {
			sb.WriteString(fmt.Sprintf("%s: [Empty]\n", responseTimeStyle("Body:")))
		} else {
			sb.WriteString(fmt.Sprintf("%s:\n\t%s\n", responseTimeStyle("Body:"), bodyStr))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(detailStyle("--------------------------------------"))

	// Log the formatted string through the logger at Debug level.
	logger.Debug("Verbose response details", slog.String("formatted_output", sb.String()))
}

// LogResponseMetrics logs response metrics in a structured format
func LogResponseMetrics(logger *slog.Logger, metrics writer.ResponseMetrics, path string) {
	if logger == nil {
		logger = GetLogger()
	}

	logAttrs := []any{
		slog.String("path", path),
		slog.Int("status_code", metrics.StatusCode),
		slog.Int64("bytes_written", metrics.BytesWritten),
		slog.String("content_type", metrics.ContentType),
		slog.Bool("is_streaming", metrics.IsStreaming),
		slog.Bool("is_truncated", metrics.IsBufferTruncated),
		slog.Int("buffered_bytes", metrics.BufferedBytes),
	}

	if metrics.IsBufferTruncated {
		logger.Warn("Response buffer truncated", logAttrs...)
	} else if metrics.IsStreaming {
		logger.Debug("Response streamed", logAttrs...)
	} else {
		logger.Debug("Response buffered", logAttrs...)
	}
}
