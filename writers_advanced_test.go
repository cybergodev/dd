package dd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// ADVANCED WRITER TESTS - FileWriter Edge Cases
// ============================================================================

// TestFileWriterInvalidConfig tests file writer with invalid config
func TestFileWriterInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		config *FileWriterConfig
		errMsg string
	}{
		{
			name:   "empty path",
			path:   "",
			config: nil,
			errMsg: "empty",
		},
		{
			name:   "path too long",
			path:   strings.Repeat("a", 5000),
			config: nil,
			errMsg: "too long",
		},
		{
			name:   "null byte in path",
			path:   "test\x00.log",
			config: nil,
			errMsg: "null byte",
		},
		{
			name:   "path traversal",
			path:   "../../../etc/passwd",
			config: nil,
			errMsg: "traversal",
		},
		{
			name: "max size too large",
			path: "logs/test.log",
			config: &FileWriterConfig{
				MaxSizeMB: 20000, // 20GB
			},
			errMsg: "too large",
		},
		{
			name: "max backups too large",
			path: "logs/test.log",
			config: &FileWriterConfig{
				MaxBackups: 2000,
			},
			errMsg: "too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fw, err := NewFileWriter(tt.path, tt.config)
			if err == nil {
				if fw != nil {
					fw.Close()
				}
				t.Errorf("Expected error containing %q", tt.errMsg)
			} else if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errMsg)) {
				t.Errorf("Expected error containing %q, got: %v", tt.errMsg, err)
			}
		})
	}
}

// TestFileWriterRotationBehavior tests file rotation behavior
func TestFileWriterRotationBehavior(t *testing.T) {
	testPath := "logs/test_rotation_behavior.log"
	defer os.Remove(testPath)
	defer os.Remove(testPath + ".1")
	defer os.Remove(testPath + ".2")

	fw, err := NewFileWriter(testPath, &FileWriterConfig{
		MaxSizeMB:  1, // 1MB
		MaxBackups: 2,
		Compress:   false,
	})
	if err != nil {
		t.Fatalf("Failed to create file writer: %v", err)
	}
	defer fw.Close()

	// Write data to trigger rotation
	data := make([]byte, 1024) // 1KB
	for i := range data {
		data[i] = 'A'
	}

	// Write 1100KB to trigger rotation
	for i := 0; i < 1100; i++ {
		n, err := fw.Write(data)
		if err != nil {
			t.Errorf("Write failed at iteration %d: %v", i, err)
		}
		if n != len(data) {
			t.Errorf("Short write at iteration %d: %d/%d", i, n, len(data))
		}
	}

	// Check that main file exists
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		t.Error("Main log file should exist")
	}
}

// TestFileWriterConcurrentWrites tests concurrent writes to file writer
func TestFileWriterConcurrentWrites(t *testing.T) {
	testPath := "logs/test_concurrent.log"
	defer os.Remove(testPath)

	fw, err := NewFileWriter(testPath, &FileWriterConfig{
		MaxSizeMB:  10,
		MaxBackups: 3,
		Compress:   false,
	})
	if err != nil {
		t.Fatalf("Failed to create file writer: %v", err)
	}
	defer fw.Close()

	var wg sync.WaitGroup
	goroutines := 20
	writesPerGoroutine := 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				msg := fmt.Sprintf("goroutine %d message %d\n", id, j)
				_, err := fw.Write([]byte(msg))
				if err != nil {
					t.Errorf("Write failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify file exists and has content
	info, err := os.Stat(testPath)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	if info.Size() == 0 {
		t.Error("File should have content")
	}
}

// TestFileWriterCloseWithError tests closing file writer with errors
func TestFileWriterCloseWithError(t *testing.T) {
	testPath := "logs/test_close_error.log"
	defer os.Remove(testPath)

	fw, err := NewFileWriter(testPath, nil)
	if err != nil {
		t.Fatalf("Failed to create file writer: %v", err)
	}

	// Write some data
	fw.Write([]byte("test\n"))

	// Close should succeed
	if err := fw.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Second close may return an error (file already closed)
	// This is acceptable behavior
	err = fw.Close()
	t.Logf("Second close result: %v", err)
}

// ============================================================================
// ADVANCED WRITER TESTS - BufferedWriter Edge Cases
// ============================================================================

// TestBufferedWriterAutoFlush tests automatic flushing
func TestBufferedWriterAutoFlush(t *testing.T) {
	var buf bytes.Buffer
	bw, err := NewBufferedWriter(&buf, 1024)
	if err != nil {
		t.Fatalf("Failed to create buffered writer: %v", err)
	}
	defer bw.Close()

	// Write data that exceeds flush threshold
	data := make([]byte, 600) // More than half of 1024
	for i := range data {
		data[i] = 'A'
	}

	bw.Write(data)

	// Should auto-flush
	time.Sleep(50 * time.Millisecond)

	if buf.Len() == 0 {
		t.Error("Buffer should have been auto-flushed")
	}
}

// TestBufferedWriterPeriodicFlush tests periodic flushing
func TestBufferedWriterPeriodicFlush(t *testing.T) {
	var buf bytes.Buffer
	bw, err := NewBufferedWriter(&buf, 4096)
	if err != nil {
		t.Fatalf("Failed to create buffered writer: %v", err)
	}
	defer bw.Close()

	// Write small amount of data
	bw.Write([]byte("test\n"))

	// Wait for periodic flush
	time.Sleep(150 * time.Millisecond)

	if buf.Len() == 0 {
		t.Error("Buffer should have been periodically flushed")
	}
}

// TestBufferedWriterCloseFlushes tests that close flushes buffer
func TestBufferedWriterCloseFlushes(t *testing.T) {
	var buf bytes.Buffer
	bw, err := NewBufferedWriter(&buf, 4096)
	if err != nil {
		t.Fatalf("Failed to create buffered writer: %v", err)
	}

	// Write data
	bw.Write([]byte("test data\n"))

	// Close should flush
	if err := bw.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("Buffer should have been flushed on close")
	}

	if !strings.Contains(buf.String(), "test data") {
		t.Error("Expected data in buffer")
	}
}

// TestBufferedWriterConcurrentWrites tests concurrent writes
func TestBufferedWriterConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	bw, err := NewBufferedWriter(&buf, 4096)
	if err != nil {
		t.Fatalf("Failed to create buffered writer: %v", err)
	}
	defer bw.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				msg := fmt.Sprintf("goroutine %d message %d\n", id, j)
				bw.Write([]byte(msg))
			}
		}(i)
	}

	wg.Wait()
	bw.Flush()

	if buf.Len() == 0 {
		t.Error("Buffer should have content")
	}
}

// ============================================================================
// ADVANCED WRITER TESTS - MultiWriter Edge Cases
// ============================================================================

// TestMultiWriterPartialFailure tests multi writer with partial failures
func TestMultiWriterPartialFailure(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	failWriter := &advancedFailingWriter{failAfter: 2}

	mw := NewMultiWriter(&buf1, failWriter, &buf2)

	// First write should succeed for buf1 and buf2
	n, err := mw.Write([]byte("test1\n"))
	if err != nil {
		t.Logf("Expected partial failure: %v", err)
	}
	if n != 6 {
		t.Errorf("Expected 6 bytes written, got %d", n)
	}

	// buf1 and buf2 should have data
	if buf1.Len() == 0 {
		t.Error("buf1 should have data")
	}
	if buf2.Len() == 0 {
		t.Error("buf2 should have data")
	}
}

// TestMultiWriterAllFail tests multi writer when all writers fail
func TestMultiWriterAllFail(t *testing.T) {
	fail1 := &advancedFailingWriter{failAfter: 0}
	fail2 := &advancedFailingWriter{failAfter: 0}

	mw := NewMultiWriter(fail1, fail2)

	_, err := mw.Write([]byte("test\n"))
	if err == nil {
		t.Error("Expected error when all writers fail")
	}
}

// TestMultiWriterConcurrentOperations tests concurrent add/remove/write
func TestMultiWriterConcurrentOperations(t *testing.T) {
	var buf1, buf2, buf3 bytes.Buffer
	mw := NewMultiWriter(&buf1)

	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				mw.Write([]byte(fmt.Sprintf("msg %d-%d\n", id, j)))
			}
		}(i)
	}

	// Concurrent add
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			mw.AddWriter(&buf2)
			time.Sleep(time.Millisecond)
		}
	}()

	// Concurrent remove
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			mw.RemoveWriter(&buf3)
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
}

// TestMultiWriterClose tests closing multi writer
func TestMultiWriterClose(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	closer1 := &closeTrackingWriter{Writer: &buf1}
	closer2 := &closeTrackingWriter{Writer: &buf2}

	mw := NewMultiWriter(closer1, closer2)

	mw.Write([]byte("test\n"))

	err := mw.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if !closer1.closed {
		t.Error("closer1 should be closed")
	}
	if !closer2.closed {
		t.Error("closer2 should be closed")
	}
}

// ============================================================================
// HELPER TYPES FOR TESTING
// ============================================================================

// advancedFailingWriter is a writer that fails after N writes
type advancedFailingWriter struct {
	failAfter int
	count     int
	mu        sync.Mutex
}

func (fw *advancedFailingWriter) Write(p []byte) (int, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	fw.count++
	if fw.count > fw.failAfter {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

// closeTrackingWriter tracks if Close was called
type closeTrackingWriter struct {
	io.Writer
	closed bool
	mu     sync.Mutex
}

func (ctw *closeTrackingWriter) Close() error {
	ctw.mu.Lock()
	defer ctw.mu.Unlock()
	ctw.closed = true
	return nil
}
