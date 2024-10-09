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
}

// WriteHeader logs the status code and writes it to the underlying ResponseWriter.
//
// Parameters:
// - statusCode: The HTTP status code to be written.
func (rw *ResponseWriter) WriteHeader(statusCode int) {
	rw.StatusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *ResponseWriter) Write(b []byte) (int, error) {
	// Set status code to 200 if it hasn't been set yet.

	// Write to the buffer and log the data being written.
	n, err := rw.Body.Write(b)

	if err != nil {
		return n, err
	}

	// Write the same data to the actual ResponseWriter so that it is sent to the client.
	n, err = rw.ResponseWriter.Write(b)

	return n, err
}
