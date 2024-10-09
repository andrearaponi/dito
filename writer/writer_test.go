package writer

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestResponseWriter tests the custom ResponseWriter implementation.
func TestResponseWriter(t *testing.T) {
	// Create an httptest.ResponseRecorder to use as the internal ResponseWriter.
	inner := httptest.NewRecorder()

	// Create our custom ResponseWriter.
	rw := &ResponseWriter{ResponseWriter: inner}

	// Test WriteHeader method.
	statusCode := http.StatusNotFound
	rw.WriteHeader(statusCode)
	if rw.StatusCode != statusCode {
		t.Errorf("Expected status code %d, got %d", statusCode, rw.StatusCode)
	}

	// Test Write method.
	testBody := "test body"
	_, err := rw.Write([]byte(testBody))
	if err != nil {
		t.Fatal("Failed to write to ResponseWriter:", err)
	}

	if rw.Body.String() != testBody {
		t.Errorf("Expected body '%s', got '%s'", testBody, rw.Body.String())
	}

	// Additionally, verify that the httptest.ResponseRecorder correctly recorded the data.
	if inner.Body.String() != testBody {
		t.Errorf("Expected inner body '%s', got '%s'", testBody, inner.Body.String())
	}

	if inner.Code != statusCode {
		t.Errorf("Expected inner status code %d, got %d", statusCode, inner.Code)
	}
}
