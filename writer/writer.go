package writer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// DefaultMaxBufferSize is the default maximum size for response buffering (1MB)
	DefaultMaxBufferSize = 1 * 1024 * 1024

	// StreamingThreshold is when we switch to streaming mode (512KB)
	StreamingThreshold = 512 * 1024

	// DefaultMaxResponseBodySize is the default maximum response body size (100MB)
	DefaultMaxResponseBodySize = 100 * 1024 * 1024
)

// ErrMaxResponseBodySizeExceeded is returned when the response body size limit is exceeded
var ErrMaxResponseBodySizeExceeded = errors.New("max response body size exceeded")

// ResponseWriter is an enterprise-grade HTTP response writer for API gateways.
// It provides buffering, streaming, response size limiting, and detailed metrics.
type ResponseWriter struct {
	http.ResponseWriter
	StatusCode          int            // HTTP status code
	BodyBuffer          *LimitedBuffer // Buffer for response body
	BytesWritten        int64          // Total bytes written
	streamingMode       bool           // Whether in streaming mode
	bufferSize          int            // Maximum buffer size
	contentType         string         // Content-Type header value
	shouldBuffer        bool           // Whether to buffer response
	maxResponseBodySize int64          // Maximum allowed response size
	responseLimitHit    bool           // Whether response limit was exceeded
	limitError          error          // Error if limit was hit
	errorWritten        bool           // Whether error response was written

	// Use sync.Once to ensure WriteHeader is called only once
	writeHeaderOnce sync.Once
	headerMu        sync.Mutex // Protects header-related operations
}

// NewResponseWriter creates a new ResponseWriter optimized for enterprise use.
//
// Parameters:
// - w: The underlying http.ResponseWriter
// - opts: Optional configuration options
//
// Returns:
// - *ResponseWriter: The configured response writer
func NewResponseWriter(w http.ResponseWriter, opts ...WriterOption) *ResponseWriter {
	rw := &ResponseWriter{
		ResponseWriter:      w,
		StatusCode:          0, // 0 indicates headers not written
		bufferSize:          DefaultMaxBufferSize,
		shouldBuffer:        true,
		maxResponseBodySize: DefaultMaxResponseBodySize,
	}

	// Apply options
	for _, opt := range opts {
		opt(rw)
	}

	// Only create buffer if buffering is enabled
	if rw.shouldBuffer {
		rw.BodyBuffer = NewLimitedBuffer(rw.bufferSize)
	} else {
		// Create a zero-sized buffer when buffering is disabled
		rw.BodyBuffer = NewLimitedBuffer(0)
	}

	return rw
}

// WriterOption allows customization of ResponseWriter behavior
type WriterOption func(*ResponseWriter)

// WithMaxBufferSize sets the maximum buffer size.
//
// Parameters:
// - size: Maximum buffer size in bytes
//
// Returns:
// - WriterOption: The option function
func WithMaxBufferSize(size int) WriterOption {
	return func(rw *ResponseWriter) {
		rw.bufferSize = size
	}
}

// WithBuffering enables/disables response buffering.
//
// Parameters:
// - enabled: Whether to enable buffering
//
// Returns:
// - WriterOption: The option function
func WithBuffering(enabled bool) WriterOption {
	return func(rw *ResponseWriter) {
		rw.shouldBuffer = enabled
	}
}

// WithMaxResponseBodySize sets the maximum response body size (0 = unlimited).
//
// Parameters:
// - size: Maximum response body size in bytes
//
// Returns:
// - WriterOption: The option function
func WithMaxResponseBodySize(size int64) WriterOption {
	return func(rw *ResponseWriter) {
		rw.maxResponseBodySize = size
	}
}

// WriteHeader captures the status code and analyzes response headers.
// It enforces response size limits based on Content-Length header.
//
// Parameters:
// - statusCode: The HTTP status code to write
func (rw *ResponseWriter) WriteHeader(statusCode int) {
	// Use sync.Once to ensure this is called only once
	rw.writeHeaderOnce.Do(func() {
		rw.headerMu.Lock()
		defer rw.headerMu.Unlock()

		rw.StatusCode = statusCode
		rw.contentType = rw.Header().Get("Content-Type")

		// Check Content-Length against limit BEFORE writing headers
		if rw.maxResponseBodySize > 0 {
			if contentLength := rw.Header().Get("Content-Length"); contentLength != "" {
				if length, err := strconv.ParseInt(contentLength, 10, 64); err == nil && length > rw.maxResponseBodySize {
					// Response is too large - send error response instead
					rw.responseLimitHit = true
					rw.limitError = ErrMaxResponseBodySizeExceeded

					// Clear any existing headers
					for key := range rw.Header() {
						delete(rw.Header(), key)
					}

					// Set error response headers
					rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
					rw.Header().Set("X-Content-Type-Options", "nosniff")
					rw.Header().Set("X-Response-Limit-Exceeded", fmt.Sprintf("%d", rw.maxResponseBodySize))

					// Write error status
					rw.ResponseWriter.WriteHeader(http.StatusRequestEntityTooLarge)
					rw.StatusCode = http.StatusRequestEntityTooLarge
					return
				}
			}
		}

		// Decide buffering strategy based on content type and size hints
		rw.analyzeBufferingStrategy()

		// Write the actual headers
		rw.ResponseWriter.WriteHeader(statusCode)
	})
}

// Write implements io.Writer with intelligent buffering and response size limiting.
//
// Parameters:
// - b: The data to write
//
// Returns:
// - int: Number of bytes written
// - error: Any error that occurred
func (rw *ResponseWriter) Write(b []byte) (int, error) {
	// Ensure headers are written before body
	if !rw.HeadersWritten() {
		rw.WriteHeader(http.StatusOK)
	}

	// If we already hit the limit and sent an error response, handle appropriately
	if rw.responseLimitHit && rw.StatusCode == http.StatusRequestEntityTooLarge {
		if !rw.errorWritten {
			// Write the error message only once
			errorMsg := fmt.Sprintf("413 Request Entity Too Large: Response body size limit exceeded (limit: %d bytes)", rw.maxResponseBodySize)
			n, err := rw.ResponseWriter.Write([]byte(errorMsg))
			rw.errorWritten = true
			if err != nil {
				return n, err
			}
			// Count these bytes too
			atomic.AddInt64(&rw.BytesWritten, int64(n))
			return n, nil
		}
		// Already wrote the error, ignore subsequent writes
		return len(b), nil // Return len(b) to avoid errors in calling code
	}

	// Check response body size limit (0 means unlimited)
	if rw.maxResponseBodySize > 0 && !rw.responseLimitHit {
		currentSize := atomic.LoadInt64(&rw.BytesWritten)
		bytesAttemptingToWrite := len(b)
		potentialTotalSize := currentSize + int64(bytesAttemptingToWrite)

		if potentialTotalSize > rw.maxResponseBodySize {
			rw.responseLimitHit = true
			rw.limitError = ErrMaxResponseBodySizeExceeded

			// Calculate how much we can write before hitting the limit
			canWriteN := int(rw.maxResponseBodySize - currentSize)

			if canWriteN > 0 {
				// Write partial data up to the limit
				dataToWrite := b[:canWriteN]
				n, writeErr := rw.ResponseWriter.Write(dataToWrite)
				atomic.AddInt64(&rw.BytesWritten, int64(n))

				if writeErr != nil {
					return n, writeErr
				}
			}

			// Return full input length to indicate we "processed" all data
			// even though we may have truncated it
			return len(b), nil
		}
	}

	// Handle case where limit was already hit in previous writes
	if rw.responseLimitHit {
		// Silently discard all data and return full input length
		return len(b), nil
	}

	// Normal write path - no size limit exceeded
	n, err := rw.ResponseWriter.Write(b)

	// Update total bytes written atomically
	atomic.AddInt64(&rw.BytesWritten, int64(n))

	// Buffer management - only if buffering is enabled
	if rw.shouldBuffer && !rw.streamingMode {
		if rw.BodyBuffer.Len()+n > StreamingThreshold {
			// Switch to streaming mode
			rw.streamingMode = true
		} else {
			// Try to buffer (ignore error if buffer is full)
			rw.BodyBuffer.Write(b[:n])
		}
	}

	return n, err
}

// HeadersWritten returns true if headers have been written.
//
// Returns:
// - bool: True if headers have been written
func (rw *ResponseWriter) HeadersWritten() bool {
	rw.headerMu.Lock()
	defer rw.headerMu.Unlock()
	return rw.StatusCode != 0
}

// analyzeBufferingStrategy determines the buffering strategy based on response characteristics.
// It considers content type, content length, and transfer encoding to optimize buffering.
func (rw *ResponseWriter) analyzeBufferingStrategy() {
	// If buffering is explicitly disabled, don't override it
	if !rw.shouldBuffer {
		return
	}

	// Check Content-Length header
	if contentLength := rw.Header().Get("Content-Length"); contentLength != "" {
		if length, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			if length > int64(StreamingThreshold) {
				rw.shouldBuffer = false
				return
			}
		}
	}

	// Don't buffer streaming responses
	if rw.Header().Get("Transfer-Encoding") == "chunked" {
		rw.shouldBuffer = false
		return
	}

	// Smart buffering based on content type
	switch {
	case strings.HasPrefix(rw.contentType, "application/json"):
		rw.shouldBuffer = true // Always buffer JSON for analysis
	case strings.HasPrefix(rw.contentType, "application/xml"):
		rw.shouldBuffer = true // Buffer XML
	case strings.HasPrefix(rw.contentType, "text/"):
		rw.shouldBuffer = true // Buffer text
	case strings.HasPrefix(rw.contentType, "image/"):
		rw.shouldBuffer = false // Don't buffer images
	case strings.HasPrefix(rw.contentType, "video/"):
		rw.shouldBuffer = false // Don't buffer video
	case strings.HasPrefix(rw.contentType, "audio/"):
		rw.shouldBuffer = false // Don't buffer audio
	case strings.HasPrefix(rw.contentType, "application/octet-stream"):
		rw.shouldBuffer = false // Don't buffer binary
	case strings.HasPrefix(rw.contentType, "application/pdf"):
		rw.shouldBuffer = false // Don't buffer PDFs
	case strings.HasPrefix(rw.contentType, "application/zip"):
		rw.shouldBuffer = false // Don't buffer archives
	case rw.contentType == "":
		// No content type specified, make a conservative choice
		rw.shouldBuffer = true
	default:
		// For unknown content types, check if it looks like text
		if strings.Contains(rw.contentType, "text") ||
			strings.Contains(rw.contentType, "json") ||
			strings.Contains(rw.contentType, "xml") {
			rw.shouldBuffer = true
		} else {
			rw.shouldBuffer = false
		}
	}
}

// Hijack implements the http.Hijacker interface.
// Allows taking over the connection for protocols like WebSocket.
//
// Returns:
// - net.Conn: The network connection
// - *bufio.ReadWriter: Buffered reader/writer
// - error: Any error that occurred
func (rw *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

// Flush implements the http.Flusher interface.
// Sends any buffered data to the client.
func (rw *ResponseWriter) Flush() {
	// Ensure headers are written first
	if !rw.HeadersWritten() {
		rw.WriteHeader(http.StatusOK)
	}

	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Push implements the http.Pusher interface for HTTP/2.
//
// Parameters:
// - target: The target path to push
// - opts: Push options
//
// Returns:
// - error: Any error that occurred
func (rw *ResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := rw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

// ResponseMetrics provides detailed metrics about the response
type ResponseMetrics struct {
	StatusCode          int    // HTTP status code
	BytesWritten        int64  // Total bytes written
	BufferedBytes       int    // Bytes currently in buffer
	IsStreaming         bool   // Whether in streaming mode
	IsBufferTruncated   bool   // Whether buffer was truncated
	ContentType         string // Content-Type header value
	IsResponseLimitHit  bool   // Whether response limit was exceeded
	MaxResponseBodySize int64  // Maximum allowed response size
	ResponseLimitError  error  // Error if limit was hit
	ErrorWritten        bool   // Whether error response was written
}

// GetMetrics returns response metrics for monitoring.
//
// Returns:
// - ResponseMetrics: Current response metrics
func (rw *ResponseWriter) GetMetrics() ResponseMetrics {
	rw.headerMu.Lock()
	statusCode := rw.StatusCode
	contentType := rw.contentType
	rw.headerMu.Unlock()

	bufferLen := 0
	if rw.BodyBuffer != nil {
		bufferLen = rw.BodyBuffer.Len()
	}

	return ResponseMetrics{
		StatusCode:          statusCode,
		BytesWritten:        atomic.LoadInt64(&rw.BytesWritten),
		BufferedBytes:       bufferLen,
		IsStreaming:         rw.streamingMode,
		IsBufferTruncated:   bufferLen >= rw.bufferSize,
		ContentType:         contentType,
		IsResponseLimitHit:  rw.responseLimitHit,
		MaxResponseBodySize: rw.maxResponseBodySize,
		ResponseLimitError:  rw.limitError,
		ErrorWritten:        rw.errorWritten,
	}
}

// GetBufferedBody returns the buffered portion of the response body.
//
// Returns:
// - []byte: The buffered body data
func (rw *ResponseWriter) GetBufferedBody() []byte {
	if rw.BodyBuffer == nil {
		return nil
	}
	return rw.BodyBuffer.Bytes()
}

// GetBufferedBodyString returns the buffered body as a string.
//
// Returns:
// - string: The buffered body as string
func (rw *ResponseWriter) GetBufferedBodyString() string {
	if rw.BodyBuffer == nil {
		return ""
	}
	return rw.BodyBuffer.String()
}

// CopyBodyTo copies the buffered body to the given writer.
//
// Parameters:
// - w: The writer to copy to
//
// Returns:
// - int64: Number of bytes copied
// - error: Any error that occurred
func (rw *ResponseWriter) CopyBodyTo(w io.Writer) (int64, error) {
	if rw.BodyBuffer == nil {
		return 0, nil
	}
	return rw.BodyBuffer.WriteTo(w)
}

// IsBufferTruncated returns true if the buffer was truncated.
//
// Returns:
// - bool: True if buffer was truncated
func (rw *ResponseWriter) IsBufferTruncated() bool {
	if rw.BodyBuffer == nil {
		return false
	}
	return rw.BodyBuffer.Len() >= rw.bufferSize
}

// IsStreaming returns true if the response is in streaming mode.
//
// Returns:
// - bool: True if in streaming mode
func (rw *ResponseWriter) IsStreaming() bool {
	return rw.streamingMode
}

// IsResponseLimitHit returns true if the response body size limit was exceeded.
//
// Returns:
// - bool: True if limit was exceeded
func (rw *ResponseWriter) IsResponseLimitHit() bool {
	return rw.responseLimitHit
}

// GetResponseLimitError returns the error if the response limit was hit.
//
// Returns:
// - error: The limit error, or nil
func (rw *ResponseWriter) GetResponseLimitError() error {
	return rw.limitError
}

// GetMaxResponseBodySize returns the configured maximum response body size.
//
// Returns:
// - int64: Maximum response body size in bytes
func (rw *ResponseWriter) GetMaxResponseBodySize() int64 {
	return rw.maxResponseBodySize
}
