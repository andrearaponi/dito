package writer

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestNewLimitedBuffer(t *testing.T) {
	lb := NewLimitedBuffer(1024)
	if lb == nil {
		t.Fatal("NewLimitedBuffer returned nil")
	}
	if lb.Cap() != 1024 {
		t.Errorf("Expected capacity 1024, got %d", lb.Cap())
	}
	if lb.Len() != 0 {
		t.Errorf("Expected initial length 0, got %d", lb.Len())
	}
	if lb.Available() != 1024 {
		t.Errorf("Expected available space 1024, got %d", lb.Available())
	}
	if lb.IsOverflow() {
		t.Error("Expected no overflow initially")
	}
	if lb.TotalSize() != 0 {
		t.Errorf("Expected total size 0, got %d", lb.TotalSize())
	}
}

func TestLimitedBuffer_Write(t *testing.T) {
	tests := []struct {
		name         string
		maxSize      int
		writes       [][]byte
		wantErr      []bool
		wantWritten  []int
		wantLen      int
		wantTotal    int64
		wantOverflow bool
	}{
		{
			name:         "write within limit",
			maxSize:      10,
			writes:       [][]byte{[]byte("hello")},
			wantErr:      []bool{false},
			wantWritten:  []int{5},
			wantLen:      5,
			wantTotal:    5,
			wantOverflow: false,
		},
		{
			name:         "write exactly at limit",
			maxSize:      10,
			writes:       [][]byte{[]byte("helloworld")},
			wantErr:      []bool{false},
			wantWritten:  []int{10},
			wantLen:      10,
			wantTotal:    10,
			wantOverflow: false,
		},
		{
			name:         "write exceeds limit",
			maxSize:      10,
			writes:       [][]byte{[]byte("hello world!")},
			wantErr:      []bool{true},
			wantWritten:  []int{10},
			wantLen:      10,
			wantTotal:    12,
			wantOverflow: true,
		},
		{
			name:         "multiple writes within limit",
			maxSize:      10,
			writes:       [][]byte{[]byte("hello"), []byte("world")},
			wantErr:      []bool{false, false},
			wantWritten:  []int{5, 5},
			wantLen:      10,
			wantTotal:    10,
			wantOverflow: false,
		},
		{
			name:         "multiple writes exceed limit",
			maxSize:      10,
			writes:       [][]byte{[]byte("hello"), []byte("world!")},
			wantErr:      []bool{false, true},
			wantWritten:  []int{5, 5},
			wantLen:      10,
			wantTotal:    11,
			wantOverflow: true,
		},
		{
			name:         "write to full buffer",
			maxSize:      5,
			writes:       [][]byte{[]byte("hello"), []byte("world")},
			wantErr:      []bool{false, true},
			wantWritten:  []int{5, 0},
			wantLen:      5,
			wantTotal:    10,
			wantOverflow: true,
		},
		{
			name:         "empty write",
			maxSize:      10,
			writes:       [][]byte{[]byte("")},
			wantErr:      []bool{false},
			wantWritten:  []int{0},
			wantLen:      0,
			wantTotal:    0,
			wantOverflow: false,
		},
		{
			name:         "zero max size",
			maxSize:      0,
			writes:       [][]byte{[]byte("hello")},
			wantErr:      []bool{true},
			wantWritten:  []int{0},
			wantLen:      0,
			wantTotal:    5,
			wantOverflow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb := NewLimitedBuffer(tt.maxSize)

			for i, data := range tt.writes {
				n, err := lb.Write(data)

				if (err != nil) != tt.wantErr[i] {
					t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr[i])
				}

				if err == ErrBufferFull && !tt.wantErr[i] {
					t.Errorf("Got ErrBufferFull when not expected")
				}

				if n != tt.wantWritten[i] {
					t.Errorf("Write() returned %d, want %d", n, tt.wantWritten[i])
				}
			}

			if lb.Len() != tt.wantLen {
				t.Errorf("Final length = %d, want %d", lb.Len(), tt.wantLen)
			}

			if lb.TotalSize() != tt.wantTotal {
				t.Errorf("Total size = %d, want %d", lb.TotalSize(), tt.wantTotal)
			}

			if lb.IsOverflow() != tt.wantOverflow {
				t.Errorf("IsOverflow = %v, want %v", lb.IsOverflow(), tt.wantOverflow)
			}
		})
	}
}

func TestLimitedBuffer_WriteString(t *testing.T) {
	lb := NewLimitedBuffer(10)

	n, err := lb.WriteString("hello")
	if err != nil {
		t.Errorf("WriteString failed: %v", err)
	}
	if n != 5 {
		t.Errorf("WriteString returned %d, want 5", n)
	}
	if lb.String() != "hello" {
		t.Errorf("Buffer content = %q, want %q", lb.String(), "hello")
	}
}

func TestLimitedBuffer_Read(t *testing.T) {
	lb := NewLimitedBuffer(100)
	testData := []byte("hello world")

	// Write data
	n, err := lb.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	if n != len(testData) {
		t.Fatalf("Write returned %d, want %d", n, len(testData))
	}

	// Read data
	buf := make([]byte, 20)
	n, err = lb.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read returned %d, want %d", n, len(testData))
	}
	if !bytes.Equal(buf[:n], testData) {
		t.Errorf("Read data = %s, want %s", buf[:n], testData)
	}
}

func TestLimitedBuffer_String(t *testing.T) {
	lb := NewLimitedBuffer(100)
	testStr := "hello world"

	_, err := lb.Write([]byte(testStr))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	if got := lb.String(); got != testStr {
		t.Errorf("String() = %q, want %q", got, testStr)
	}
}

func TestLimitedBuffer_Bytes(t *testing.T) {
	lb := NewLimitedBuffer(100)
	testData := []byte("hello world")

	_, err := lb.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	if got := lb.Bytes(); !bytes.Equal(got, testData) {
		t.Errorf("Bytes() = %v, want %v", got, testData)
	}
}

func TestLimitedBuffer_Available(t *testing.T) {
	lb := NewLimitedBuffer(10)

	if lb.Available() != 10 {
		t.Errorf("Initial available = %d, want 10", lb.Available())
	}

	lb.Write([]byte("hello"))
	if lb.Available() != 5 {
		t.Errorf("Available after 5 bytes = %d, want 5", lb.Available())
	}

	lb.Write([]byte("world"))
	if lb.Available() != 0 {
		t.Errorf("Available after 10 bytes = %d, want 0", lb.Available())
	}
}

func TestLimitedBuffer_Reset(t *testing.T) {
	lb := NewLimitedBuffer(100)

	// Write some data and trigger overflow
	_, err := lb.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Write more to trigger overflow
	lb.Write([]byte(strings.Repeat("x", 200)))

	// Verify data is written and overflow occurred
	if lb.Len() == 0 {
		t.Error("Buffer should not be empty after write")
	}
	if !lb.IsOverflow() {
		t.Error("Buffer should have overflow")
	}
	if lb.TotalSize() == 0 {
		t.Error("Total size should not be zero")
	}

	// Reset
	lb.Reset()

	// Verify buffer is completely reset
	if lb.Len() != 0 {
		t.Errorf("Len() after reset = %d, want 0", lb.Len())
	}
	if lb.String() != "" {
		t.Errorf("String() after reset = %q, want empty", lb.String())
	}
	if lb.IsOverflow() {
		t.Error("IsOverflow() after reset should be false")
	}
	if lb.TotalSize() != 0 {
		t.Errorf("TotalSize() after reset = %d, want 0", lb.TotalSize())
	}
	if lb.Available() != 100 {
		t.Errorf("Available() after reset = %d, want 100", lb.Available())
	}

	// Verify we can write again after reset
	n, err := lb.Write([]byte("new data"))
	if err != nil {
		t.Errorf("Write after reset failed: %v", err)
	}
	if n != 8 {
		t.Errorf("Write after reset returned %d, want 8", n)
	}
}

func TestLimitedBuffer_WriteTo(t *testing.T) {
	lb := NewLimitedBuffer(100)
	testData := []byte("hello world")

	// Write data to limited buffer
	_, err := lb.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// WriteTo another buffer
	var buf bytes.Buffer
	n, err := lb.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}
	if n != int64(len(testData)) {
		t.Errorf("WriteTo returned %d, want %d", n, len(testData))
	}
	if !bytes.Equal(buf.Bytes(), testData) {
		t.Errorf("WriteTo data = %v, want %v", buf.Bytes(), testData)
	}

	// Verify buffer is drained after WriteTo
	if lb.Len() != 0 {
		t.Errorf("Len() after WriteTo = %d, want 0", lb.Len())
	}
}

func TestLimitedBuffer_Truncate(t *testing.T) {
	lb := NewLimitedBuffer(100)
	testData := []byte("hello world")

	lb.Write(testData)

	// Truncate to 5 bytes
	lb.Truncate(5)

	if lb.Len() != 5 {
		t.Errorf("Len after truncate = %d, want 5", lb.Len())
	}
	if lb.String() != "hello" {
		t.Errorf("String after truncate = %q, want %q", lb.String(), "hello")
	}

	// Truncate with negative value
	lb.Truncate(-1)
	if lb.Len() != 0 {
		t.Errorf("Len after negative truncate = %d, want 0", lb.Len())
	}
}

func TestLimitedBuffer_Grow(t *testing.T) {
	lb := NewLimitedBuffer(10)

	// Grow within capacity
	err := lb.Grow(5)
	if err != nil {
		t.Errorf("Grow within capacity failed: %v", err)
	}

	// Grow beyond capacity
	err = lb.Grow(20)
	if err != ErrBufferOverflow {
		t.Errorf("Grow beyond capacity should return ErrBufferOverflow, got %v", err)
	}
}

func TestLimitedBuffer_ReadFrom(t *testing.T) {
	lb := NewLimitedBuffer(10)

	// Read from a string reader
	reader := strings.NewReader("hello world")
	n, err := lb.ReadFrom(reader)

	if err != ErrBufferFull {
		t.Errorf("ReadFrom should return ErrBufferFull, got %v", err)
	}
	if n != 10 {
		t.Errorf("ReadFrom returned %d, want 10", n)
	}
	if lb.String() != "hello worl" {
		t.Errorf("Buffer content = %q, want %q", lb.String(), "hello worl")
	}
	if !lb.IsOverflow() {
		t.Error("Buffer should be in overflow state")
	}
	if lb.TotalSize() != 11 { // "hello world" is 11 bytes
		t.Errorf("Total size = %d, want 11", lb.TotalSize())
	}
}

func TestLimitedBuffer_ConcurrentAccess(t *testing.T) {
	lb := NewLimitedBuffer(1000)
	var wg sync.WaitGroup
	numGoroutines := 10
	writesPerGoroutine := 10

	// Start multiple writers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				data := []byte(strings.Repeat("a", 5))
				lb.Write(data)
			}
		}(i)
	}

	// Start multiple readers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			buf := make([]byte, 10)
			for j := 0; j < writesPerGoroutine; j++ {
				lb.Read(buf)
				_ = lb.String()
				_ = lb.Bytes()
				_ = lb.Len()
				_ = lb.Available()
				_ = lb.IsOverflow()
				_ = lb.TotalSize()
			}
		}(i)
	}

	wg.Wait()

	// Verify final state is consistent
	if lb.Len() > lb.Cap() {
		t.Errorf("Buffer length %d exceeds capacity %d", lb.Len(), lb.Cap())
	}
	if lb.Available() < 0 {
		t.Errorf("Available space cannot be negative: %d", lb.Available())
	}
}

func TestLimitedBuffer_PartialWrite(t *testing.T) {
	lb := NewLimitedBuffer(10)

	// Write 8 bytes
	n, err := lb.Write([]byte("12345678"))
	if err != nil {
		t.Fatalf("First write failed: %v", err)
	}
	if n != 8 {
		t.Errorf("First write returned %d, want 8", n)
	}

	// Try to write 5 more bytes (should only write 2)
	n, err = lb.Write([]byte("abcde"))
	if err != ErrBufferFull {
		t.Errorf("Expected ErrBufferFull, got %v", err)
	}
	if n != 2 {
		t.Errorf("Partial write returned %d, want 2", n)
	}

	// Verify buffer content
	expected := "12345678ab"
	if got := lb.String(); got != expected {
		t.Errorf("Buffer content = %q, want %q", got, expected)
	}
	if lb.Len() != 10 {
		t.Errorf("Buffer length = %d, want 10", lb.Len())
	}
	if lb.TotalSize() != 13 {
		t.Errorf("Total size = %d, want 13", lb.TotalSize())
	}
	if !lb.IsOverflow() {
		t.Error("Buffer should be in overflow state")
	}
}

func BenchmarkLimitedBuffer_Write(b *testing.B) {
	lb := NewLimitedBuffer(1024 * 1024) // 1MB
	data := []byte("hello world")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb.Write(data)
		if lb.Len() > 1024*1024-100 {
			lb.Reset()
		}
	}
}

func BenchmarkLimitedBuffer_ConcurrentWrite(b *testing.B) {
	lb := NewLimitedBuffer(1024 * 1024) // 1MB
	data := []byte("hello world")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lb.Write(data)
			if lb.Len() > 1024*1024-100 {
				lb.Reset()
			}
		}
	})
}

func BenchmarkLimitedBuffer_WithOverflow(b *testing.B) {
	data := []byte("hello world")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb := NewLimitedBuffer(5) // Small buffer to trigger overflow
		lb.Write(data)            // Will cause overflow
	}
}

func BenchmarkLimitedBuffer_ReadFrom(b *testing.B) {
	data := strings.Repeat("hello world ", 1000) // ~12KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb := NewLimitedBuffer(1024 * 1024) // 1MB buffer
		reader := strings.NewReader(data)
		lb.ReadFrom(reader)
	}
}
