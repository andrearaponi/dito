package writer

import (
	"bufio"
	"bytes"
	"net"
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

	// Write to the buffer.
	_, err := rw.Body.Write(b) // n from here is bytes written to buffer, not response
	if err != nil {
		// Error writing to buffer, probably shouldn't proceed to write to response
		return 0, err
	}

	// Write the same data to the actual ResponseWriter so that it is sent to the client.
	// This is the authoritative count of bytes written to the client.
	n, err := rw.ResponseWriter.Write(b)
	rw.BytesWritten += n // Update the total bytes written based on actual write to response

	return n, err
}

// Hijack allows the caller to take over the connection from the HTTP server.
// This function is typically used for implementing WebSockets or other protocols
// that require raw network access.
//
// Returns:
// - net.Conn: The underlying network connection.
// - *bufio.ReadWriter: A buffered read/write interface for the connection.
// - error: An error if the hijacking fails.
func (w *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}
