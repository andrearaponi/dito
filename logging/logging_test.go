package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dito/writer" // For LogResponse test
	"log/slog"

	"github.com/gorilla/websocket" // For LogWebSocketMessage test
	"github.com/stretchr/testify/assert"
)

func TestInitializeLogger(t *testing.T) {
	testCases := []struct {
		name          string
		levelStr      string
		expectedLevel slog.Level
		checkLevels   map[slog.Level]bool // map of level to check -> expected enabled status
	}{
		{
			name:          "debug level",
			levelStr:      "debug",
			expectedLevel: slog.LevelDebug,
			checkLevels: map[slog.Level]bool{
				slog.LevelDebug: true,
				slog.LevelInfo:  true,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
		{
			name:          "info level",
			levelStr:      "info",
			expectedLevel: slog.LevelInfo,
			checkLevels: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  true,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
		{
			name:          "warn level",
			levelStr:      "warn",
			expectedLevel: slog.LevelWarn,
			checkLevels: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  false,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
		{
			name:          "error level",
			levelStr:      "error",
			expectedLevel: slog.LevelError,
			checkLevels: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  false,
				slog.LevelWarn:  false,
				slog.LevelError: true,
			},
		},
		{
			name:          "invalid level defaults to info",
			levelStr:      "invalid_level",
			expectedLevel: slog.LevelInfo, // Default level
			checkLevels: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  true,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger := InitializeLogger(tc.levelStr)
			assert.NotNil(t, logger)

			for level, expected := range tc.checkLevels {
				assert.Equal(t, expected, logger.Handler().Enabled(context.Background(), level), "Level %s enabled status mismatch", level.String())
			}
		})
	}
}

// Helper to create a logger that writes to a buffer for testing.
// For text output (substring matching).
func newTestTextLogger(buf *bytes.Buffer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: level}))
}

// Helper to create a logger that writes JSON to a buffer for testing.
func newTestJSONLogger(buf *bytes.Buffer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level}))
}

func TestLogRequestVerbose(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestTextLogger(&buf, slog.LevelDebug)

	req, _ := http.NewRequest("GET", "http://example.com/test?query=1", bytes.NewBufferString("request body content"))
	req.Header.Add("X-Test-Header", "TestValue")
	req.Header.Add("User-Agent", "TestAgent")

	requestBodyBytes := []byte("request body content")
	// The 'headers' parameter in LogRequestVerbose is for request headers.
	// Pass req.Header to it.
	LogRequestVerbose(logger, req, requestBodyBytes, req.Header, http.StatusOK, 2*time.Second)

	output := buf.String()
	assert.Contains(t, output, "Verbose request details")
	assert.Contains(t, output, "Method:")
	assert.Contains(t, output, "GET")
	assert.Contains(t, output, "URL:")
	assert.Contains(t, output, "http://example.com/test?query=1")
	assert.Contains(t, output, "Request Headers:")
	assert.Contains(t, output, "X-Test-Header: TestValue")
	assert.Contains(t, output, "User-Agent: TestAgent")
	assert.Contains(t, output, "Request Body:")
	assert.Contains(t, output, "request body content")
	assert.Contains(t, output, "Response Details") // This part is actually from the LogRequestVerbose formatting itself
	assert.Contains(t, output, "Status Code:")     // This is also from LogRequestVerbose formatting
	assert.Contains(t, output, "200")              // Status code
	assert.Contains(t, output, "Response Time:")   // From LogRequestVerbose formatting
	assert.Contains(t, output, "2.000000 seconds") // Duration
}

func TestLogResponse(t *testing.T) {
	t.Run("normal response", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newTestTextLogger(&buf, slog.LevelDebug)

		rr := httptest.NewRecorder()
		// Create ResponseWriter with the new implementation
		customResponseWriter := writer.NewResponseWriter(rr)
		customResponseWriter.WriteHeader(http.StatusAccepted) // 202
		customResponseWriter.Header().Set("X-Resp-Header", "RespValue")
		customResponseWriter.Header().Set("Content-Type", "application/json")

		// Write some data
		testBody := "response body content"
		customResponseWriter.Write([]byte(testBody))

		LogResponse(logger, customResponseWriter)

		output := buf.String()
		assert.Contains(t, output, "Verbose response details")
		assert.Contains(t, output, "Status Code:")
		assert.Contains(t, output, "202")
		assert.Contains(t, output, "Content-Type:")
		assert.Contains(t, output, "application/json")
		assert.Contains(t, output, "Total Bytes Written:")
		assert.Contains(t, output, "21 bytes") // length of "response body content"
		assert.Contains(t, output, "Headers:")
		assert.Contains(t, output, "X-Resp-Header: RespValue")
		assert.Contains(t, output, "Body:")
		assert.Contains(t, output, "response body content")
	})

	t.Run("truncated response", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newTestTextLogger(&buf, slog.LevelDebug)

		rr := httptest.NewRecorder()
		// Create ResponseWriter with small buffer
		customResponseWriter := writer.NewResponseWriter(rr, writer.WithMaxBufferSize(10))
		customResponseWriter.WriteHeader(http.StatusOK)

		// Write more data than buffer can hold
		customResponseWriter.Write([]byte("This is a very long response body that will be truncated"))

		LogResponse(logger, customResponseWriter)

		output := buf.String()
		assert.Contains(t, output, "Body (Truncated):")
		assert.Contains(t, output, "WARNING:")
		assert.Contains(t, output, "Response body was truncated")
		assert.Contains(t, output, "Buffered 10 bytes out of")
	})

	t.Run("streaming response", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newTestTextLogger(&buf, slog.LevelDebug)

		rr := httptest.NewRecorder()
		customResponseWriter := writer.NewResponseWriter(rr)
		customResponseWriter.WriteHeader(http.StatusOK)
		customResponseWriter.Header().Set("Content-Type", "video/mp4")

		// Write enough to trigger streaming mode
		largeData := strings.Repeat("a", 600*1024) // 600KB
		customResponseWriter.Write([]byte(largeData))

		LogResponse(logger, customResponseWriter)

		output := buf.String()
		assert.Contains(t, output, "[STREAMING MODE - Body not buffered]")
		assert.Contains(t, output, "First")
		assert.Contains(t, output, "bytes buffered before streaming")
	})

	t.Run("empty response", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newTestTextLogger(&buf, slog.LevelDebug)

		rr := httptest.NewRecorder()
		customResponseWriter := writer.NewResponseWriter(rr)
		customResponseWriter.WriteHeader(http.StatusNoContent)

		LogResponse(logger, customResponseWriter)

		output := buf.String()
		assert.Contains(t, output, "Body:")
		assert.Contains(t, output, "[Empty]")
	})
}

func TestLogRequestCompact(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestJSONLogger(&buf, slog.LevelInfo)

	reqTime := time.Now()
	req, _ := http.NewRequest("POST", "http://example.com/compact?key=val", bytes.NewBufferString("compact body"))
	req.RemoteAddr = "192.168.1.100:12345"
	req.Proto = "HTTP/1.1"
	req.Header.Set("User-Agent", "CompactAgent")
	req.Header.Set("Referer", "http://referer.com")

	requestHeaders := req.Header        // LogRequestCompact expects request headers
	bodyBytes := []byte("compact body") // LogRequestCompact expects body bytes

	LogRequestCompact(logger, req, bodyBytes, requestHeaders, http.StatusCreated, 500*time.Millisecond)

	var logOutput map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logOutput)
	assert.NoError(t, err, "Failed to unmarshal log output")

	assert.Equal(t, slog.LevelInfo.String(), logOutput["level"], "Log level mismatch")
	assert.Equal(t, "HTTP request processed", logOutput["msg"], "Log message mismatch")

	assert.Equal(t, "192.168.1.100:12345", logOutput["client_ip"])
	assert.Equal(t, "POST", logOutput["method"])
	assert.Equal(t, "/compact", logOutput["url"]) // r.URL.Path does not include query string
	assert.Equal(t, "HTTP/1.1", logOutput["protocol"])
	assert.Equal(t, float64(http.StatusCreated), logOutput["status_code"]) // JSON numbers are float64
	assert.Equal(t, "http://referer.com", logOutput["referer"])
	assert.Equal(t, "CompactAgent", logOutput["user_agent"])
	assert.Equal(t, 0.5, logOutput["duration_seconds"]) // 500ms = 0.5s

	// Check time is recent
	logTimeStr, ok := logOutput["time"].(string)
	assert.True(t, ok, "Time field not a string or not present")
	logTime, err := time.Parse(time.RFC3339Nano, logTimeStr)
	assert.NoError(t, err, "Failed to parse log time")
	assert.WithinDuration(t, reqTime, logTime, 5*time.Second, "Log time is not recent")
}

func TestLogWebSocketMessage(t *testing.T) {
	testCases := []struct {
		name            string
		messageType     int
		message         []byte
		err             error
		expectedLevel   slog.Level
		expectedMsg     string
		expectErrorAttr bool
		expectedAttrs   map[string]interface{}
	}{
		{
			name:          "text message",
			messageType:   websocket.TextMessage,
			message:       []byte("hello websocket"),
			err:           nil,
			expectedLevel: slog.LevelInfo,
			expectedMsg:   "WebSocket text message received",
			expectedAttrs: map[string]interface{}{"type": "Text", "message_content": "hello websocket"},
		},
		{
			name:          "text message truncation",
			messageType:   websocket.TextMessage,
			message:       []byte(strings.Repeat("a", 150)),
			err:           nil,
			expectedLevel: slog.LevelInfo,
			expectedMsg:   "WebSocket text message received",
			expectedAttrs: map[string]interface{}{"type": "Text", "message_content": strings.Repeat("a", 100) + "..."},
		},
		{
			name:          "binary message",
			messageType:   websocket.BinaryMessage,
			message:       []byte{0x01, 0x02, 0x03},
			err:           nil,
			expectedLevel: slog.LevelInfo,
			expectedMsg:   "WebSocket message received",
			expectedAttrs: map[string]interface{}{"type": "Binary", "message_size_bytes": float64(3)}, // JSON numbers
		},
		{
			name:          "ping message",
			messageType:   websocket.PingMessage,
			message:       []byte{},
			err:           nil,
			expectedLevel: slog.LevelDebug,
			expectedMsg:   "WebSocket ping/pong message received",
			expectedAttrs: map[string]interface{}{"type": "Ping"},
		},
		{
			name:          "pong message",
			messageType:   websocket.PongMessage,
			message:       []byte{},
			err:           nil,
			expectedLevel: slog.LevelDebug,
			expectedMsg:   "WebSocket ping/pong message received",
			expectedAttrs: map[string]interface{}{"type": "Pong"},
		},
		{
			name:            "message with error",
			messageType:     websocket.TextMessage,
			message:         []byte("some data"),
			err:             errors.New("test ws error"),
			expectedLevel:   slog.LevelError,
			expectedMsg:     "WebSocket message processing error",
			expectErrorAttr: true,
			expectedAttrs:   map[string]interface{}{"type": "Text", "error": "test ws error"},
		},
		{
			name:          "close message",
			messageType:   websocket.CloseMessage,
			message:       []byte{0x03, 0xe8}, // Example close code 1000
			err:           nil,
			expectedLevel: slog.LevelInfo,
			expectedMsg:   "WebSocket message received",
			expectedAttrs: map[string]interface{}{"type": "Close", "message_size_bytes": float64(2)},
		},
		{
			name:          "unknown message type",
			messageType:   0x0F, // Made up type
			message:       []byte("unknown data"),
			err:           nil,
			expectedLevel: slog.LevelInfo,
			expectedMsg:   "WebSocket message received",
			expectedAttrs: map[string]interface{}{"type": "Unknown", "message_size_bytes": float64(12)},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := newTestJSONLogger(&buf, slog.LevelDebug) // Use debug to capture all levels for checking

			logTimeStart := time.Now()
			LogWebSocketMessage(logger, tc.messageType, tc.message, tc.err, 100*time.Millisecond)

			var logOutput map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logOutput)
			assert.NoError(t, err, "Failed to unmarshal log output: %s", buf.String())

			assert.Equal(t, tc.expectedLevel.String(), logOutput["level"], "Log level mismatch")
			assert.Equal(t, tc.expectedMsg, logOutput["msg"], "Log message mismatch")

			// Check common attributes
			assert.Equal(t, 0.1, logOutput["duration_seconds"], "Duration mismatch") // 100ms

			// Check specific attributes from tc.expectedAttrs
			for key, expectedValue := range tc.expectedAttrs {
				assert.Equal(t, expectedValue, logOutput[key], "Attribute '%s' mismatch", key)
			}

			if tc.expectErrorAttr {
				_, ok := logOutput["error"].(string)
				assert.True(t, ok, "Expected 'error' attribute to be present and a string")
			} else {
				_, ok := logOutput["error"]
				assert.False(t, ok, "Did not expect 'error' attribute to be present")
			}

			// Check time is recent
			logTimeStr, ok := logOutput["time"].(string)
			assert.True(t, ok, "Time field not a string or not present")
			logTime, err := time.Parse(time.RFC3339Nano, logTimeStr)
			assert.NoError(t, err, "Failed to parse log time")
			assert.WithinDuration(t, logTimeStart, logTime, 2*time.Second, "Log time is not recent")
		})
	}
}

// TestGetLogger tests the GetLogger function, particularly its singleton behavior.
func TestGetLogger(t *testing.T) {
	// Reset the global logger for a clean test, if possible (not easily done without exporting it or a reset func)
	// For this test, we'll rely on the fact that it's initialized once.
	logger1 := GetLogger()
	assert.NotNil(t, logger1, "GetLogger returned nil on first call")

	logger2 := GetLogger()
	assert.NotNil(t, logger2, "GetLogger returned nil on second call")

	// Test that it returns the same instance
	assert.Same(t, logger1, logger2, "GetLogger did not return the same instance")

	// Test that it's a functional logger (e.g., Info level is enabled by default)
	assert.True(t, logger1.Handler().Enabled(context.Background(), slog.LevelInfo), "Default logger does not have Info level enabled")
}

func TestLogResponseMetrics(t *testing.T) {
	t.Run("normal response", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newTestJSONLogger(&buf, slog.LevelDebug)

		metrics := writer.ResponseMetrics{
			StatusCode:        200,
			BytesWritten:      1024,
			BufferedBytes:     1024,
			IsStreaming:       false,
			IsBufferTruncated: false,
			ContentType:       "application/json",
		}

		LogResponseMetrics(logger, metrics, "/api/test")

		var logOutput map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logOutput)
		assert.NoError(t, err)

		assert.Equal(t, "DEBUG", logOutput["level"])
		assert.Equal(t, "Response buffered", logOutput["msg"])
		assert.Equal(t, "/api/test", logOutput["path"])
		assert.Equal(t, float64(200), logOutput["status_code"])
		assert.Equal(t, float64(1024), logOutput["bytes_written"])
		assert.Equal(t, "application/json", logOutput["content_type"])
		assert.Equal(t, false, logOutput["is_streaming"])
		assert.Equal(t, false, logOutput["is_truncated"])
	})

	t.Run("truncated response", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newTestJSONLogger(&buf, slog.LevelWarn)

		metrics := writer.ResponseMetrics{
			StatusCode:        200,
			BytesWritten:      2048,
			BufferedBytes:     1024,
			IsStreaming:       false,
			IsBufferTruncated: true,
			ContentType:       "text/html",
		}

		LogResponseMetrics(logger, metrics, "/large-page")

		var logOutput map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logOutput)
		assert.NoError(t, err)

		assert.Equal(t, "WARN", logOutput["level"])
		assert.Equal(t, "Response buffer truncated", logOutput["msg"])
		assert.Equal(t, true, logOutput["is_truncated"])
		assert.Equal(t, float64(1024), logOutput["buffered_bytes"])
	})

	t.Run("streaming response", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newTestJSONLogger(&buf, slog.LevelDebug)

		metrics := writer.ResponseMetrics{
			StatusCode:        200,
			BytesWritten:      1048576, // 1MB
			BufferedBytes:     524288,  // 512KB
			IsStreaming:       true,
			IsBufferTruncated: false,
			ContentType:       "video/mp4",
		}

		LogResponseMetrics(logger, metrics, "/video/stream")

		var logOutput map[string]interface{}
		err := json.Unmarshal(buf.Bytes(), &logOutput)
		assert.NoError(t, err)

		assert.Equal(t, "DEBUG", logOutput["level"])
		assert.Equal(t, "Response streamed", logOutput["msg"])
		assert.Equal(t, true, logOutput["is_streaming"])
		assert.Equal(t, "video/mp4", logOutput["content_type"])
	})
}
