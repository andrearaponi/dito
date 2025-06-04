package writer

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	// DefaultMaxBufferSize is the default maximum size for response buffering (1MB)
	DefaultMaxBufferSize = 1 * 1024 * 1024

	// StreamingThreshold is when we switch to streaming mode (512KB)
	StreamingThreshold = 512 * 1024
)

// ResponseWriter is an enterprise-grade HTTP response writer for API gateways
type ResponseWriter struct {
	http.ResponseWriter
	StatusCode    int
	BodyBuffer    *LimitedBuffer
	BytesWritten  int64
	streamingMode bool
	bufferSize    int
	contentType   string
	shouldBuffer  bool
}

// NewResponseWriter creates a new ResponseWriter optimized for enterprise use
func NewResponseWriter(w http.ResponseWriter, opts ...WriterOption) *ResponseWriter {
	rw := &ResponseWriter{
		ResponseWriter: w,
		bufferSize:     DefaultMaxBufferSize,
		shouldBuffer:   true,
	}

	// Apply options
	for _, opt := range opts {
		opt(rw)
	}

	rw.BodyBuffer = NewLimitedBuffer(rw.bufferSize)
	return rw
}

// WriterOption allows customization of ResponseWriter behavior
type WriterOption func(*ResponseWriter)

// WithMaxBufferSize sets the maximum buffer size
func WithMaxBufferSize(size int) WriterOption {
	return func(rw *ResponseWriter) {
		rw.bufferSize = size
	}
}

// WithBuffering enables/disables buffering
func WithBuffering(enabled bool) WriterOption {
	return func(rw *ResponseWriter) {
		rw.shouldBuffer = enabled
	}
}

// WriteHeader captures the status code and analyzes response headers
func (rw *ResponseWriter) WriteHeader(statusCode int) {
	rw.StatusCode = statusCode
	rw.contentType = rw.Header().Get("Content-Type")

	// Decide buffering strategy based on content type and size hints
	rw.analyzeBufferingStrategy()

	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write implements io.Writer with intelligent buffering
func (rw *ResponseWriter) Write(b []byte) (int, error) {
	// Set default status code if not set
	if rw.StatusCode == 0 {
		rw.StatusCode = http.StatusOK
	}

	// Always write to the actual response writer first
	n, err := rw.ResponseWriter.Write(b)

	// Update total bytes written atomically
	atomic.AddInt64(&rw.BytesWritten, int64(n))

	// Buffer management
	if rw.shouldBuffer && !rw.streamingMode {
		if rw.BodyBuffer.Len()+len(b) > StreamingThreshold {
			// Switch to streaming mode
			rw.streamingMode = true
		} else {
			// Try to buffer (ignore error if buffer is full)
			rw.BodyBuffer.Write(b[:n])
		}
	}

	return n, err
}

// analyzeBufferingStrategy determines the buffering strategy based on response characteristics
func (rw *ResponseWriter) analyzeBufferingStrategy() {
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

// Hijack implements the http.Hijacker interface
func (rw *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

// Flush implements the http.Flusher interface
func (rw *ResponseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// CloseNotify implements the http.CloseNotifier interface (deprecated but still used)
func (rw *ResponseWriter) CloseNotify() <-chan bool {
	if notifier, ok := rw.ResponseWriter.(http.CloseNotifier); ok {
		return notifier.CloseNotify()
	}
	return nil
}

// Push implements the http.Pusher interface for HTTP/2
func (rw *ResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := rw.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

// ResponseMetrics provides metrics about the response
type ResponseMetrics struct {
	StatusCode        int
	BytesWritten      int64
	BufferedBytes     int
	IsStreaming       bool
	IsBufferTruncated bool
	ContentType       string
}

// GetMetrics returns response metrics for monitoring
func (rw *ResponseWriter) GetMetrics() ResponseMetrics {
	return ResponseMetrics{
		StatusCode:        rw.StatusCode,
		BytesWritten:      atomic.LoadInt64(&rw.BytesWritten),
		BufferedBytes:     rw.BodyBuffer.Len(),
		IsStreaming:       rw.streamingMode,
		IsBufferTruncated: rw.BodyBuffer.Len() >= rw.bufferSize,
		ContentType:       rw.contentType,
	}
}

// GetBufferedBody returns the buffered portion of the response body
func (rw *ResponseWriter) GetBufferedBody() []byte {
	if rw.BodyBuffer == nil {
		return nil
	}
	return rw.BodyBuffer.Bytes()
}

// GetBufferedBodyString returns the buffered body as a string
func (rw *ResponseWriter) GetBufferedBodyString() string {
	if rw.BodyBuffer == nil {
		return ""
	}
	return rw.BodyBuffer.String()
}

// CopyBodyTo copies the buffered body to the given writer
func (rw *ResponseWriter) CopyBodyTo(w io.Writer) (int64, error) {
	if rw.BodyBuffer == nil {
		return 0, nil
	}
	return rw.BodyBuffer.WriteTo(w)
}

// IsBufferTruncated returns true if the buffer was truncated
func (rw *ResponseWriter) IsBufferTruncated() bool {
	if rw.BodyBuffer == nil {
		return false
	}
	return rw.BodyBuffer.Len() >= rw.bufferSize
}

// IsStreaming returns true if the response is in streaming mode
func (rw *ResponseWriter) IsStreaming() bool {
	return rw.streamingMode
}
