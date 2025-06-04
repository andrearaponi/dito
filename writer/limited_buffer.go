package writer

import (
	"bytes"
	"errors"
	"io"
	"sync"
)

// ErrBufferFull is returned when attempting to write to a full buffer
var ErrBufferFull = errors.New("buffer size limit exceeded")

// LimitedBuffer is a thread-safe buffer with configurable size limits
type LimitedBuffer struct {
	mu      sync.RWMutex
	buffer  bytes.Buffer
	maxSize int
	written int
}

// NewLimitedBuffer creates a new LimitedBuffer with the specified maximum size
func NewLimitedBuffer(maxSize int) *LimitedBuffer {
	return &LimitedBuffer{
		maxSize: maxSize,
	}
}

// Write writes data to the buffer if there's enough space
func (lb *LimitedBuffer) Write(p []byte) (int, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Check if writing would exceed the limit
	if lb.written+len(p) > lb.maxSize {
		// Calculate how much we can write
		available := lb.maxSize - lb.written
		if available <= 0 {
			return 0, ErrBufferFull
		}

		// Write only what fits
		n, err := lb.buffer.Write(p[:available])
		lb.written += n
		if err != nil {
			return n, err
		}
		return n, ErrBufferFull
	}

	// Write the full data
	n, err := lb.buffer.Write(p)
	lb.written += n
	return n, err
}

// Read reads data from the buffer
func (lb *LimitedBuffer) Read(p []byte) (int, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.buffer.Read(p)
}

// String returns the buffer content as a string
func (lb *LimitedBuffer) String() string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.buffer.String()
}

// Bytes returns the buffer content as a byte slice
func (lb *LimitedBuffer) Bytes() []byte {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.buffer.Bytes()
}

// Len returns the number of bytes written to the buffer
func (lb *LimitedBuffer) Len() int {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.written
}

// Cap returns the maximum capacity of the buffer
func (lb *LimitedBuffer) Cap() int {
	return lb.maxSize
}

// Reset resets the buffer
func (lb *LimitedBuffer) Reset() {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.buffer.Reset()
	lb.written = 0
}

// WriteTo writes the buffer content to the given writer
func (lb *LimitedBuffer) WriteTo(w io.Writer) (int64, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	n, err := lb.buffer.WriteTo(w)
	// Update written count after WriteTo drains the buffer
	lb.written = lb.buffer.Len()
	return n, err
}
