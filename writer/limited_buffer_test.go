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
}

func TestLimitedBuffer_Write(t *testing.T) {
	tests := []struct {
		name        string
		maxSize     int
		writes      [][]byte
		wantErr     []bool
		wantWritten []int
		wantLen     int
	}{
		{
			name:        "write within limit",
			maxSize:     10,
			writes:      [][]byte{[]byte("hello")},
			wantErr:     []bool{false},
			wantWritten: []int{5},
			wantLen:     5,
		},
		{
			name:        "write exactly at limit",
			maxSize:     10,
			writes:      [][]byte{[]byte("helloworld")},
			wantErr:     []bool{false},
			wantWritten: []int{10},
			wantLen:     10,
		},
		{
			name:        "write exceeds limit",
			maxSize:     10,
			writes:      [][]byte{[]byte("hello world!")},
			wantErr:     []bool{true},
			wantWritten: []int{10},
			wantLen:     10,
		},
		{
			name:        "multiple writes within limit",
			maxSize:     10,
			writes:      [][]byte{[]byte("hello"), []byte("world")},
			wantErr:     []bool{false, false},
			wantWritten: []int{5, 5},
			wantLen:     10,
		},
		{
			name:        "multiple writes exceed limit",
			maxSize:     10,
			writes:      [][]byte{[]byte("hello"), []byte("world!")},
			wantErr:     []bool{false, true},
			wantWritten: []int{5, 5},
			wantLen:     10,
		},
		{
			name:        "write to full buffer",
			maxSize:     5,
			writes:      [][]byte{[]byte("hello"), []byte("world")},
			wantErr:     []bool{false, true},
			wantWritten: []int{5, 0},
			wantLen:     5,
		},
		{
			name:        "empty write",
			maxSize:     10,
			writes:      [][]byte{[]byte("")},
			wantErr:     []bool{false},
			wantWritten: []int{0},
			wantLen:     0,
		},
		{
			name:        "zero max size",
			maxSize:     0,
			writes:      [][]byte{[]byte("hello")},
			wantErr:     []bool{true},
			wantWritten: []int{0},
			wantLen:     0,
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
		})
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

func TestLimitedBuffer_Reset(t *testing.T) {
	lb := NewLimitedBuffer(100)

	// Write some data
	_, err := lb.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Verify data is written
	if lb.Len() == 0 {
		t.Error("Buffer should not be empty after write")
	}

	// Reset
	lb.Reset()

	// Verify buffer is empty
	if lb.Len() != 0 {
		t.Errorf("Len() after reset = %d, want 0", lb.Len())
	}
	if lb.String() != "" {
		t.Errorf("String() after reset = %q, want empty", lb.String())
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
			}
		}(i)
	}

	wg.Wait()

	// Verify final state is consistent
	if lb.Len() > lb.Cap() {
		t.Errorf("Buffer length %d exceeds capacity %d", lb.Len(), lb.Cap())
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
