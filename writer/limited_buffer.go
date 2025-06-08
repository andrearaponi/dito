package writer

import (
	"bytes"
	"errors"
	"io"
	"sync"
)

// ErrBufferFull is returned when attempting to write to a full buffer
var ErrBufferFull = errors.New("buffer size limit exceeded")

// ErrBufferOverflow is returned when write would exceed the maximum allowed size
var ErrBufferOverflow = errors.New("write would exceed buffer maximum size")

// LimitedBuffer is a thread-safe buffer with configurable size limits
type LimitedBuffer struct {
	mu        sync.RWMutex
	buffer    bytes.Buffer
	maxSize   int
	written   int
	overflow  bool
	totalSize int64 // Total size of all data that was attempted to be written
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

	// Track total attempted write size
	lb.totalSize += int64(len(p))

	// If max size is 0, don't write anything
	if lb.maxSize == 0 {
		lb.overflow = true
		return 0, ErrBufferFull
	}

	// Check if writing would exceed the limit
	if lb.written+len(p) > lb.maxSize {
		// Calculate how much we can write
		available := lb.maxSize - lb.written
		if available <= 0 {
			lb.overflow = true
			return 0, ErrBufferFull
		}

		// Write only what fits
		n, err := lb.buffer.Write(p[:available])
		lb.written += n
		lb.overflow = true
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

// WriteString writes a string to the buffer
func (lb *LimitedBuffer) WriteString(s string) (int, error) {
	return lb.Write([]byte(s))
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
	// Return a copy to prevent external modification
	data := lb.buffer.Bytes()
	result := make([]byte, len(data))
	copy(result, data)
	return result
}

// Len returns the number of bytes currently stored in the buffer
func (lb *LimitedBuffer) Len() int {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.written
}

// Cap returns the maximum capacity of the buffer
func (lb *LimitedBuffer) Cap() int {
	return lb.maxSize
}

// Available returns the number of bytes that can still be written to the buffer
func (lb *LimitedBuffer) Available() int {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.maxSize - lb.written
}

// IsOverflow returns true if the buffer has experienced overflow
func (lb *LimitedBuffer) IsOverflow() bool {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.overflow
}

// TotalSize returns the total size of all data that was attempted to be written
func (lb *LimitedBuffer) TotalSize() int64 {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.totalSize
}

// Reset resets the buffer to its initial state
func (lb *LimitedBuffer) Reset() {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.buffer.Reset()
	lb.written = 0
	lb.overflow = false
	lb.totalSize = 0
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

// Truncate discards all but the first n bytes from the buffer but continues to track total size
func (lb *LimitedBuffer) Truncate(n int) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if n < 0 {
		n = 0
	}
	if n >= lb.written {
		return
	}

	// Create new buffer with truncated content
	data := lb.buffer.Bytes()
	if n < len(data) {
		lb.buffer.Reset()
		lb.buffer.Write(data[:n])
		lb.written = n
	}
}

// Grow grows the buffer's capacity, if possible, to guarantee space for another n bytes
func (lb *LimitedBuffer) Grow(n int) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if lb.written+n > lb.maxSize {
		return ErrBufferOverflow
	}

	lb.buffer.Grow(n)
	return nil
}

// Clone creates a copy of the LimitedBuffer with the same content and state
func (lb *LimitedBuffer) Clone() *LimitedBuffer {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	// Create a completely new instance
	clone := &LimitedBuffer{
		maxSize:   lb.maxSize,
		totalSize: lb.totalSize,
	}

	// Get the data and create a completely independent buffer
	data := lb.buffer.Bytes()
	if len(data) > 0 {
		// Create a completely new buffer and write the data
		// This ensures no shared memory between the buffers
		content := string(data)           // Convert to string first
		clone.buffer.WriteString(content) // Then write as string to new buffer
	}

	// Set written and overflow based on actual content, not inherited state
	// This allows the clone to be independently modifiable
	clone.written = len(data)
	clone.overflow = len(data) >= clone.maxSize && clone.maxSize > 0

	return clone
}

// ReadFrom reads data from r until EOF and writes it to the buffer
func (lb *LimitedBuffer) ReadFrom(r io.Reader) (int64, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Calculate how much space we have
	available := lb.maxSize - lb.written
	if available <= 0 {
		lb.overflow = true
		return 0, ErrBufferFull
	}

	// Use limited reader to prevent overflow
	limitedReader := io.LimitReader(r, int64(available))
	n, err := lb.buffer.ReadFrom(limitedReader)
	lb.written += int(n)
	lb.totalSize += n

	// Check if we hit the limit
	if int(n) == available {
		// Try to read one more byte to see if there's more data
		var oneByte [1]byte
		if extraN, extraErr := r.Read(oneByte[:]); extraN > 0 || extraErr == nil {
			lb.overflow = true
			lb.totalSize += int64(extraN)
			return n, ErrBufferFull
		}
	}

	return n, err
}
