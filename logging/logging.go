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
func LogRequestVerbose(req *http.Request, body []byte, headers http.Header, statusCode int, duration time.Duration) {
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

	// Print the final log message
	fmt.Println(sb.String())
}

// LogRequestCompact logs the HTTP request and response in a compact format.
func LogRequestCompact(r *http.Request, body []byte, headers http.Header, statusCode int, duration time.Duration) {
	logger := GetLogger()
	clientIP := r.RemoteAddr
	method := r.Method
	url := r.URL.Path
	protocol := r.Proto
	userAgent := r.Header.Get("User-Agent")
	referer := r.Header.Get("Referer")

	logger.Info(fmt.Sprintf("%s - \"%s %s %s\" %d \"%s\" \"%s\" %.6f seconds",
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

// LogWebSocketMessage logs the details of a WebSocket message.
func LogWebSocketMessageOLD(messageType int, message []byte, err error, duration time.Duration) {
	logger := GetLogger()
	messageTypeStr := "Unknown"
	switch messageType {
	case websocket.TextMessage:
		messageTypeStr = "Text"
	case websocket.BinaryMessage:
		messageTypeStr = "Binary"
	case websocket.CloseMessage:
		messageTypeStr = "Close"
	case websocket.PingMessage:
		messageTypeStr = "Ping"
	case websocket.PongMessage:
		messageTypeStr = "Pong"
	}

	if err != nil {
		logger.Error(fmt.Sprintf("WebSocket %s Message Error: %v", messageTypeStr, err))
	} else {
		logger.Info(fmt.Sprintf("WebSocket %s Message: %s (%.6f seconds)", messageTypeStr, string(message), duration.Seconds()))
	}
}

// LogWebSocketMessage logs the details of a WebSocket message.
func LogWebSocketMessage(messageType int, message []byte, err error, duration time.Duration) {
	logger := GetLogger()
	messageTypeStr := getMessageTypeString(messageType)

	// Dettagli da loggare
	logDetails := map[string]interface{}{
		"type":     messageTypeStr,
		"duration": duration.Seconds(),
	}

	// Log in caso di errore
	if err != nil {
		logDetails["error"] = err.Error()
		logger.Error("WebSocket Message Error", slog.Any("details", logDetails))
		return
	}

	// Logga il contenuto del messaggio in base al tipo
	switch messageType {
	case websocket.TextMessage:
		logDetails["message"] = truncateMessage(message)
		logger.Info("WebSocket Text Message", slog.Any("details", logDetails))
	case websocket.PingMessage, websocket.PongMessage:
		logger.Debug("WebSocket Ping/Pong Message", slog.Any("details", logDetails))
	default:
		logDetails["message_size"] = len(message)
		logger.Info("WebSocket Message", slog.Any("details", logDetails))
	}
}

// Funzione di utilità per abbreviare messaggi molto lunghi
func truncateMessage(message []byte) string {
	const maxLength = 100
	if len(message) > maxLength {
		return string(message[:maxLength]) + "..."
	}
	return string(message)
}

// Funzione di utilità per ottenere la descrizione del tipo di messaggio
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

// LogResponse logs the details of the HTTP response.
func LogResponse(lrw *writer.ResponseWriter) {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(detailStyle("----------- Response Details ----------"))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("%s: %d\n\n", responseTimeStyle("Status Code:"), lrw.StatusCode))

	sb.WriteString(headersStyle("Headers:"))
	sb.WriteString("\n")
	for name, values := range lrw.Header() {
		for _, value := range values {
			sb.WriteString(fmt.Sprintf("\t%s: %s\n", boldWhiteStyle(name), value))
		}
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("%s\t%s\n", responseTimeStyle("Body:"), lrw.Body.String()))
	sb.WriteString("\n")
	sb.WriteString(detailStyle("--------------------------------------"))

	// Print the final log message
	fmt.Println(sb.String())
}
