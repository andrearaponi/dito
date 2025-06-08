package writer

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockResponseWriter is a test helper that can simulate write errors
type mockResponseWriter struct {
	header      http.Header
	body        []byte
	statusCode  int
	shouldError bool
}

func (m *mockResponseWriter) Header() http.Header {
	return m.header
}

func (m *mockResponseWriter) Write(data []byte) (int, error) {
	if m.shouldError {
		// Write some data before erroring
		if len(data) > 50 {
			m.body = append(m.body, data[:50]...)
			return 50, errors.New("mock write error")
		}
		return 0, errors.New("mock write error")
	}
	m.body = append(m.body, data...)
	return len(data), nil
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

func TestNewResponseWriter(t *testing.T) {
	inner := httptest.NewRecorder()

	// Test default configuration
	rw := NewResponseWriter(inner)
	if rw.bufferSize != DefaultMaxBufferSize {
		t.Errorf("Expected default buffer size %d, got %d", DefaultMaxBufferSize, rw.bufferSize)
		t.Run("underlying writer error takes precedence", func(t *testing.T) {
			// Create a mock ResponseWriter that returns an error
			mockWriter := &mockResponseWriter{
				header:      make(http.Header),
				shouldError: true,
			}
			rw := NewResponseWriter(mockWriter, WithMaxResponseBodySize(100))

			// Write data that exceeds the limit
			data := strings.Repeat("a", 150)
			n, err := rw.Write([]byte(data))

			// Should return the underlying writer error, not the limit error
			if err == nil {
				t.Error("Expected an error from underlying writer")
			}
			if err.Error() != "mock write error" {
				t.Errorf("Expected mock write error, got: %v", err)
			}
			// Should return the actual bytes written by underlying writer (not len(b))
			if n != 50 { // mockResponseWriter writes 50 bytes before erroring
				t.Errorf("Expected 50 bytes written, got %d", n)
			}
		})

	}
	if !rw.shouldBuffer {
		t.Error("Expected buffering to be enabled by default")
	}
	if rw.maxResponseBodySize != DefaultMaxResponseBodySize {
		t.Errorf("Expected default max response body size %d, got %d", DefaultMaxResponseBodySize, rw.maxResponseBodySize)
	}

	// Test with options
	customSize := 2 * 1024 * 1024
	customLimit := int64(50 * 1024 * 1024)
	rw2 := NewResponseWriter(inner, WithMaxBufferSize(customSize), WithBuffering(false), WithMaxResponseBodySize(customLimit))
	if rw2.bufferSize != customSize {
		t.Errorf("Expected buffer size %d, got %d", customSize, rw2.bufferSize)
	}
	if rw2.shouldBuffer {
		t.Error("Expected buffering to be disabled")
	}
	if rw2.maxResponseBodySize != customLimit {
		t.Errorf("Expected max response body size %d, got %d", customLimit, rw2.maxResponseBodySize)
	}
}

// Update these test cases in writer_test.go

func TestResponseBodySizeLimit(t *testing.T) {
	t.Run("within limit", func(t *testing.T) {
		inner := httptest.NewRecorder()
		rw := NewResponseWriter(inner, WithMaxResponseBodySize(100)) // 100 bytes limit

		data := strings.Repeat("a", 50) // 50 bytes
		n, err := rw.Write([]byte(data))

		if err != nil {
			t.Errorf("Write should not fail for data within limit: %v", err)
		}
		if n != 50 {
			t.Errorf("Expected to write 50 bytes, wrote %d", n)
		}
		if rw.IsResponseLimitHit() {
			t.Error("Response limit should not be hit")
		}
	})

	t.Run("exactly at limit", func(t *testing.T) {
		inner := httptest.NewRecorder()
		rw := NewResponseWriter(inner, WithMaxResponseBodySize(100)) // 100 bytes limit

		data := strings.Repeat("a", 100) // exactly 100 bytes
		n, err := rw.Write([]byte(data))

		if err != nil {
			t.Errorf("Write should not fail for data exactly at limit: %v", err)
		}
		if n != 100 {
			t.Errorf("Expected to write 100 bytes, wrote %d", n)
		}
		if rw.IsResponseLimitHit() {
			t.Error("Response limit should not be hit for exact limit")
		}
	})

	t.Run("exceeds limit in single write", func(t *testing.T) {
		inner := httptest.NewRecorder()
		rw := NewResponseWriter(inner, WithMaxResponseBodySize(100)) // 100 bytes limit

		data := strings.Repeat("a", 150) // 150 bytes - exceeds limit
		n, err := rw.Write([]byte(data))

		// Should NOT return an error anymore - silently truncates
		if err != nil {
			t.Errorf("Write should not return an error: %v", err)
		}
		if n != 150 { // Should return len(b) to indicate processing
			t.Errorf("Expected to return 150 (full input length), got %d", n)
		}
		if !rw.IsResponseLimitHit() {
			t.Error("Response limit should be hit")
		}

		// Check that only 100 bytes were actually written to the client
		if inner.Body.Len() != 100 {
			t.Errorf("Expected 100 bytes written to client, got %d", inner.Body.Len())
		}

		// Verify the limit error is stored internally
		if rw.GetResponseLimitError() == nil {
			t.Error("Expected response limit error to be set internally")
		}
	})

	t.Run("exceeds limit across multiple writes", func(t *testing.T) {
		inner := httptest.NewRecorder()
		rw := NewResponseWriter(inner, WithMaxResponseBodySize(100)) // 100 bytes limit

		// First write: 60 bytes (within limit)
		data1 := strings.Repeat("a", 60)
		n1, err1 := rw.Write([]byte(data1))

		if err1 != nil {
			t.Errorf("First write should not fail: %v", err1)
		}
		if n1 != 60 {
			t.Errorf("Expected to write 60 bytes in first write, wrote %d", n1)
		}
		if rw.IsResponseLimitHit() {
			t.Error("Response limit should not be hit after first write")
		}

		// Second write: 50 bytes (would exceed limit)
		data2 := strings.Repeat("b", 50)
		n2, err2 := rw.Write([]byte(data2))

		// Should NOT return an error anymore - silently truncates
		if err2 != nil {
			t.Errorf("Second write should not return an error: %v", err2)
		}
		if n2 != 50 {
			t.Errorf("Expected to return 50 (full input length), got %d", n2)
		}
		if !rw.IsResponseLimitHit() {
			t.Error("Response limit should be hit after second write")
		}

		// Check that only 100 bytes total were written to the client
		if inner.Body.Len() != 100 {
			t.Errorf("Expected 100 bytes total written to client, got %d", inner.Body.Len())
		}

		// Check the content is correct (60 'a's + 40 'b's)
		content := inner.Body.String()
		if len(content) != 100 {
			t.Errorf("Content length should be 100, got %d", len(content))
		}
		expectedContent := strings.Repeat("a", 60) + strings.Repeat("b", 40)
		if content != expectedContent {
			t.Errorf("Content mismatch. Expected %s..., got %s...", expectedContent[:20], content[:20])
		}
	})

	t.Run("unlimited response size (0)", func(t *testing.T) {
		inner := httptest.NewRecorder()
		rw := NewResponseWriter(inner, WithMaxResponseBodySize(0)) // unlimited

		// Write a large amount of data
		data := strings.Repeat("a", 1024*1024) // 1MB
		n, err := rw.Write([]byte(data))

		if err != nil {
			t.Errorf("Write should not fail for unlimited response size: %v", err)
		}
		if n != 1024*1024 {
			t.Errorf("Expected to write %d bytes, wrote %d", 1024*1024, n)
		}
		if rw.IsResponseLimitHit() {
			t.Error("Response limit should not be hit for unlimited size")
		}
	})

	t.Run("third write after limit hit", func(t *testing.T) {
		inner := httptest.NewRecorder()
		rw := NewResponseWriter(inner, WithMaxResponseBodySize(50)) // 50 bytes limit

		// First write: 30 bytes
		rw.Write([]byte(strings.Repeat("a", 30)))

		// Second write: 30 bytes (exceeds limit)
		rw.Write([]byte(strings.Repeat("b", 30)))

		// Third write: should be silently discarded
		n3, err3 := rw.Write([]byte(strings.Repeat("c", 10)))

		// Should NOT return an error - silently discards
		if err3 != nil {
			t.Errorf("Third write should not return an error: %v", err3)
		}
		if n3 != 10 { // Should return len(b) even though nothing was written
			t.Errorf("Expected to return 10 (input length), got %d", n3)
		}

		// Check total bytes written to client
		if inner.Body.Len() != 50 {
			t.Errorf("Expected 50 bytes total written to client, got %d", inner.Body.Len())
		}
	})

	t.Run("underlying writer error takes precedence", func(t *testing.T) {
		// Create a mock ResponseWriter that returns an error
		mockWriter := &mockResponseWriter{
			header:      make(http.Header),
			shouldError: true,
		}
		rw := NewResponseWriter(mockWriter, WithMaxResponseBodySize(100))

		// Write data that exceeds the limit
		data := strings.Repeat("a", 150)
		n, err := rw.Write([]byte(data))

		// Should return the underlying writer error
		if err == nil {
			t.Error("Expected an error from underlying writer")
		}
		if err != nil && err.Error() != "mock write error" {
			t.Errorf("Expected mock write error, got: %v", err)
		}
		// Should return the actual bytes written by underlying writer
		if n != 50 { // mockResponseWriter writes 50 bytes before erroring
			t.Errorf("Expected 50 bytes written, got %d", n)
		}
	})
}
func TestResponseMetricsWithLimits(t *testing.T) {
	inner := httptest.NewRecorder()
	limit := int64(100)
	rw := NewResponseWriter(inner, WithMaxResponseBodySize(limit))

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)

	// Write data that exceeds the limit
	data := []byte(strings.Repeat("x", 150))
	rw.Write(data)

	metrics := rw.GetMetrics()

	if metrics.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, metrics.StatusCode)
	}

	if metrics.BytesWritten != 100 {
		t.Errorf("Expected 100 bytes written, got %d", metrics.BytesWritten)
	}

	if metrics.ContentType != "application/json" {
		t.Errorf("Expected content type %q, got %q", "application/json", metrics.ContentType)
	}

	if !metrics.IsResponseLimitHit {
		t.Error("Expected response limit to be hit")
	}

	if metrics.MaxResponseBodySize != limit {
		t.Errorf("Expected max response body size %d, got %d", limit, metrics.MaxResponseBodySize)
	}

	if metrics.ResponseLimitError == nil {
		t.Error("Expected response limit error to be set")
	}
}

func TestContentTypeBuffering(t *testing.T) {
	tests := []struct {
		name         string
		contentType  string
		shouldBuffer bool
	}{
		{"JSON", "application/json", true},
		{"JSON with charset", "application/json; charset=utf-8", true},
		{"XML", "application/xml", true},
		{"Plain text", "text/plain", true},
		{"HTML", "text/html", true},
		{"Image JPEG", "image/jpeg", false},
		{"Image PNG", "image/png", false},
		{"Video MP4", "video/mp4", false},
		{"Audio MP3", "audio/mpeg", false},
		{"Binary", "application/octet-stream", false},
		{"PDF", "application/pdf", false},
		{"ZIP", "application/zip", false},
		{"Unknown text-like", "application/vnd.api+json", true},
		{"Unknown binary-like", "application/vnd.myapp.data", false},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := httptest.NewRecorder()
			rw := NewResponseWriter(inner)

			// Set content type
			rw.Header().Set("Content-Type", tt.contentType)
			rw.WriteHeader(http.StatusOK)

			if rw.shouldBuffer != tt.shouldBuffer {
				t.Errorf("For content type %q, expected shouldBuffer=%v, got %v",
					tt.contentType, tt.shouldBuffer, rw.shouldBuffer)
			}
		})
	}
}

func TestStreamingMode(t *testing.T) {
	inner := httptest.NewRecorder()
	rw := NewResponseWriter(inner)

	// Write small chunks that fit in buffer
	smallData := strings.Repeat("a", 100*1024) // 100KB
	for i := 0; i < 4; i++ {
		rw.Write([]byte(smallData))
		if rw.IsStreaming() {
			t.Errorf("Should not be in streaming mode after %d KB", (i+1)*100)
		}
	}

	// Write more data to trigger streaming mode
	rw.Write([]byte(strings.Repeat("b", 200*1024))) // 200KB more
	if !rw.IsStreaming() {
		t.Error("Should be in streaming mode after exceeding threshold")
	}

	// Verify metrics
	metrics := rw.GetMetrics()
	if !metrics.IsStreaming {
		t.Error("Metrics should indicate streaming mode")
	}
	if metrics.BytesWritten != 600*1024 {
		t.Errorf("Expected 600KB written, got %d", metrics.BytesWritten)
	}
}

func TestContentLengthHeader(t *testing.T) {
	tests := []struct {
		name          string
		contentLength string
		shouldBuffer  bool
	}{
		{"Small file", "1024", true},
		{"Medium file", "524288", true},  // 512KB, at threshold
		{"Large file", "1048576", false}, // 1MB, over threshold
		{"Invalid", "invalid", true},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := httptest.NewRecorder()
			rw := NewResponseWriter(inner)

			rw.Header().Set("Content-Length", tt.contentLength)
			rw.WriteHeader(http.StatusOK)

			if rw.shouldBuffer != tt.shouldBuffer {
				t.Errorf("For Content-Length %q, expected shouldBuffer=%v, got %v",
					tt.contentLength, tt.shouldBuffer, rw.shouldBuffer)
			}
		})
	}
}

func TestChunkedTransferEncoding(t *testing.T) {
	inner := httptest.NewRecorder()
	rw := NewResponseWriter(inner)

	rw.Header().Set("Transfer-Encoding", "chunked")
	rw.WriteHeader(http.StatusOK)

	if rw.shouldBuffer {
		t.Error("Should not buffer chunked responses")
	}
}

func TestBufferTruncation(t *testing.T) {
	inner := httptest.NewRecorder()
	// Small buffer for testing
	rw := NewResponseWriter(inner, WithMaxBufferSize(1024))

	// Write 2KB of data
	data := strings.Repeat("a", 2048)
	n, err := rw.Write([]byte(data))

	if err != nil {
		t.Errorf("Write should not fail: %v", err)
	}
	if n != 2048 {
		t.Errorf("Expected to write 2048 bytes, wrote %d", n)
	}

	// Check buffer contains only 1KB
	buffered := rw.GetBufferedBody()
	if len(buffered) > 1024 {
		t.Errorf("Buffer should contain at most 1024 bytes, got %d", len(buffered))
	}

	if !rw.IsBufferTruncated() {
		t.Error("Buffer should be marked as truncated")
	}

	// But full response should be sent to client
	if inner.Body.Len() != 2048 {
		t.Errorf("Client should receive full 2048 bytes, got %d", inner.Body.Len())
	}
}

func TestMetrics(t *testing.T) {
	inner := httptest.NewRecorder()
	rw := NewResponseWriter(inner)

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)

	data := []byte(`{"status":"ok"}`)
	rw.Write(data)

	metrics := rw.GetMetrics()

	if metrics.StatusCode != http.StatusCreated {
		t.Errorf("Expected status code %d, got %d", http.StatusCreated, metrics.StatusCode)
	}

	if metrics.BytesWritten != int64(len(data)) {
		t.Errorf("Expected %d bytes written, got %d", len(data), metrics.BytesWritten)
	}

	if metrics.ContentType != "application/json" {
		t.Errorf("Expected content type %q, got %q", "application/json", metrics.ContentType)
	}

	if metrics.IsStreaming {
		t.Error("Should not be in streaming mode for small response")
	}

	if metrics.IsResponseLimitHit {
		t.Error("Response limit should not be hit")
	}
}

func TestHTTPInterfaces(t *testing.T) {
	// Test Flusher
	t.Run("Flusher", func(t *testing.T) {
		inner := httptest.NewRecorder()
		rw := NewResponseWriter(inner)

		// httptest.ResponseRecorder implements Flusher
		rw.Flush() // Should not panic
	})

	// Test Hijacker
	t.Run("Hijacker", func(t *testing.T) {
		// httptest.ResponseRecorder doesn't implement Hijacker
		inner := httptest.NewRecorder()
		rw := NewResponseWriter(inner)

		_, _, err := rw.Hijack()
		if err != http.ErrNotSupported {
			t.Error("Expected ErrNotSupported for Hijacker")
		}
	})

	// Test Pusher
	t.Run("Pusher", func(t *testing.T) {
		inner := httptest.NewRecorder()
		rw := NewResponseWriter(inner)

		err := rw.Push("/resource", nil)
		if err != http.ErrNotSupported {
			t.Error("Expected ErrNotSupported for Pusher")
		}
	})
}

func TestConcurrentWrites(t *testing.T) {
	inner := httptest.NewRecorder()
	rw := NewResponseWriter(inner)

	// Concurrent writes should be safe
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			data := strings.Repeat(string(rune('a'+id)), 100)
			rw.Write([]byte(data))
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	metrics := rw.GetMetrics()
	if metrics.BytesWritten != 1000 {
		t.Errorf("Expected 1000 bytes written, got %d", metrics.BytesWritten)
	}
}

func TestBufferDisabled(t *testing.T) {
	inner := httptest.NewRecorder()
	rw := NewResponseWriter(inner, WithBuffering(false))

	data := []byte("test data")
	rw.Write(data)

	if len(rw.GetBufferedBody()) != 0 {
		t.Error("Buffer should be empty when buffering is disabled")
	}

	if rw.BytesWritten != int64(len(data)) {
		t.Error("Bytes should still be counted even with buffering disabled")
	}
}

func BenchmarkResponseWriter(b *testing.B) {
	b.Run("SmallJSON", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			inner := httptest.NewRecorder()
			rw := NewResponseWriter(inner)
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)
			rw.Write([]byte(`{"status":"ok","id":12345}`))
		}
	})

	b.Run("LargeJSON", func(b *testing.B) {
		largeJSON := []byte(strings.Repeat(`{"id":12345,"data":"test"},`, 10000))
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			inner := httptest.NewRecorder()
			rw := NewResponseWriter(inner)
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)
			rw.Write(largeJSON)
		}
	})

	b.Run("Streaming", func(b *testing.B) {
		chunk := []byte(strings.Repeat("a", 100*1024)) // 100KB chunks
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			inner := httptest.NewRecorder()
			rw := NewResponseWriter(inner)
			rw.Header().Set("Content-Type", "application/octet-stream")
			rw.WriteHeader(http.StatusOK)

			// Write 10 chunks
			for j := 0; j < 10; j++ {
				rw.Write(chunk)
			}
		}
	})

	b.Run("WithResponseLimit", func(b *testing.B) {
		data := []byte(strings.Repeat("a", 1024)) // 1KB data
		limit := int64(512)                       // 512 byte limit
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			inner := httptest.NewRecorder()
			rw := NewResponseWriter(inner, WithMaxResponseBodySize(limit))
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)
			rw.Write(data) // Will hit the limit
		}
	})
}
