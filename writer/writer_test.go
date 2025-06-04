package writer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewResponseWriter(t *testing.T) {
	inner := httptest.NewRecorder()

	// Test default configuration
	rw := NewResponseWriter(inner)
	if rw.bufferSize != DefaultMaxBufferSize {
		t.Errorf("Expected default buffer size %d, got %d", DefaultMaxBufferSize, rw.bufferSize)
	}
	if !rw.shouldBuffer {
		t.Error("Expected buffering to be enabled by default")
	}

	// Test with options
	customSize := 2 * 1024 * 1024
	rw2 := NewResponseWriter(inner, WithMaxBufferSize(customSize), WithBuffering(false))
	if rw2.bufferSize != customSize {
		t.Errorf("Expected buffer size %d, got %d", customSize, rw2.bufferSize)
	}
	if rw2.shouldBuffer {
		t.Error("Expected buffering to be disabled")
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

	// Test CloseNotifier
	t.Run("CloseNotifier", func(t *testing.T) {
		inner := httptest.NewRecorder()
		rw := NewResponseWriter(inner)

		ch := rw.CloseNotify()
		if ch != nil {
			t.Error("Expected nil channel from ResponseRecorder")
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
}
