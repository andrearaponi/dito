package writer

import (
	"bytes"
	"net/http"
)

// ResponseWriter is an HTTP response writer that logs the status code and body.
type ResponseWriter struct {
	http.ResponseWriter              // Embeds the standard HTTP ResponseWriter.
	StatusCode          int          // Stores the HTTP status code of the response.
	Body                bytes.Buffer // Buffers the body of the response.
	BytesWritten        int          // Tracks the number of bytes written to the response.
}

// WriteHeader logs the status code and writes it to the underlying ResponseWriter.
//
// Parameters:
// - statusCode: The HTTP status code to be written.
func (rw *ResponseWriter) WriteHeader(statusCode int) {
	rw.StatusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write writes the data to the buffer and logs the number of bytes written.
// It also writes the same data to the actual ResponseWriter so that it is sent to the client.
//
// Parameters:
// - b: The byte slice to write to the response.
//
// Returns:
// - int: The number of bytes written.
// - error: An error if the write fails.
func (rw *ResponseWriter) Write(b []byte) (int, error) {
	// Set status code to 200 if it hasn't been set yet.
	if rw.StatusCode == 0 {
		rw.StatusCode = http.StatusOK
	}

	// Write to the buffer and track the bytes being written.
	n, err := rw.Body.Write(b)
	rw.BytesWritten += n // Update the total bytes written

	if err != nil {
		return n, err
	}

	// Write the same data to the actual ResponseWriter so that it is sent to the client.
	n, err = rw.ResponseWriter.Write(b)
	rw.BytesWritten += n // Update the total bytes written again if necessary

	return n, err
}
