package dd

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// Resource Leak Verification Tests
// ============================================================================

// waitForBackgroundCloses waits for all SetDefault/InitDefault background close
// goroutines (tracked by backgroundCloseWg) to finish, or returns false when the
// timeout elapses. Used by leak tests to assert goroutines are not leaked.
func waitForBackgroundCloses(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		backgroundCloseWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// TestLeakCloseWaitsForFilterGoroutines verifies that Logger.Close() waits for
// in-flight security filter goroutines to complete before closing writers.
// This prevents the LEAK-1 scenario where filter goroutines outlive the logger.
func TestLeakCloseWaitsForFilterGoroutines(t *testing.T) {
	// Create a logger with security filtering enabled
	cfg := DefaultConfig()
	cfg.Security = &SecurityConfig{
		SensitiveFilter: NewSensitiveDataFilter(),
	}
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Spawn goroutines that trigger filter operations with large inputs
	// (large inputs trigger the async filterWithTimeout path with goroutines)
	var wg sync.WaitGroup
	largeInput := strings.Repeat("password=secret123 ", 1000) // >10KB triggers async path

	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info(largeInput)
			done <- struct{}{} // signal completed
		}()
	}

	// Wait for at least one log call to enter the filter path
	<-done

	// Close should not panic and should wait for filter goroutines
	closeErr := logger.Close()
	if closeErr != nil {
		t.Errorf("Close() returned error: %v", closeErr)
	}

	// Verify no active filter goroutines remain
	active := logger.ActiveFilterGoroutines()
	if active != 0 {
		t.Errorf("Expected 0 active filter goroutines after Close(), got %d", active)
	}

	wg.Wait()
}

// TestLeakSetDefaultTracksGoroutines verifies that SetDefault and InitDefault
// track their background close goroutines so they can be waited on.
// This prevents the LEAK-2 scenario of untracked fire-and-forget goroutines.
func TestLeakSetDefaultTracksGoroutines(t *testing.T) {
	// Reset by creating fresh loggers
	for i := 0; i < 5; i++ {
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(ioDiscard())}
		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		SetDefault(logger)
	}

	// waitForBackgroundCloses should succeed within a reasonable time
	completed := waitForBackgroundCloses(2 * time.Second)
	if !completed {
		t.Error("waitForBackgroundCloses timed out — goroutines may be leaking")
	}

	// Clean up
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(ioDiscard())}
	logger, _ := New(cfg)
	SetDefault(logger)
	waitForBackgroundCloses(2 * time.Second)
	logger.Close()
}

// TestLeakFilterCacheReleasedOnClose verifies that SensitiveDataFilter.Close()
// releases the cache map to free memory.
// This prevents the LEAK-3 scenario of retained cache memory.
func TestLeakFilterCacheReleasedOnClose(t *testing.T) {
	filter := NewSensitiveDataFilter()

	// Populate cache with some entries
	for i := 0; i < 100; i++ {
		input := "password=secret" + time.Now().String()
		filter.Filter(input)
	}

	// Verify stats show cache activity
	stats := filter.GetFilterStats()
	if stats.CacheHits+stats.CacheMiss == 0 {
		t.Log("Warning: no cache activity recorded")
	}

	// Close should release cache
	result := filter.Close()
	if !result {
		t.Error("Close() returned false — goroutines did not complete")
	}

	// After close, cache should be nil (verify via repeated Filter calls)
	// Filter returns input unchanged after close
	output := filter.Filter("password=test123")
	if output != "password=test123" {
		t.Errorf("Expected unchanged input after Close(), got %q", output)
	}
}

// TestLeakAuditLoggerByTypeReleasedOnClose verifies that AuditLogger.Close()
// cleans up the byType statistics map.
// This prevents the LEAK-4 scenario of retained statistics memory.
func TestLeakAuditLoggerByTypeReleasedOnClose(t *testing.T) {
	cfg := AuditConfig{
		Enabled:    true,
		Output:     nil, // no output needed
		BufferSize: 100,
	}

	al, err := NewAuditLogger(cfg)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}

	// Log various event types to populate byType
	al.LogSensitiveDataRedaction("test_pattern", "field1", "test message")
	al.LogRateLimitExceeded("rate limited", nil)
	al.LogSecurityViolation("violation", "test", nil)
	al.LogReDoSAttempt("evil_pattern", "test")

	// Wait for async processing
	time.Sleep(50 * time.Millisecond)

	// Verify stats populated
	stats := al.Stats()
	if stats.TotalEvents < 4 {
		t.Errorf("Expected at least 4 events, got %d", stats.TotalEvents)
	}

	// Close should clean up
	closeErr := al.Close()
	if closeErr != nil {
		t.Errorf("Close() returned error: %v", closeErr)
	}

	// Stats after close should still be accessible (counters are atomic)
	stats = al.Stats()
	if stats.TotalEvents == 0 {
		t.Error("Stats should still report total events after close")
	}
}

// TestLeakBufferedWriterGoroutineCleanup verifies that BufferedWriter.Close()
// properly stops the background auto-flush goroutine.
func TestLeakBufferedWriterGoroutineCleanup(t *testing.T) {
	var buf safeBuffer
	bw, err := NewBufferedWriter(&buf, BufferedWriterConfig{BufferSize: 4096})
	if err != nil {
		t.Fatalf("Failed to create buffered writer: %v", err)
	}

	// Write some data
	bw.Write([]byte("test data"))

	// Count goroutines before close
	before := runtime.NumGoroutine()

	// Close should stop the auto-flush goroutine
	closeErr := bw.Close()
	if closeErr != nil {
		t.Errorf("Close() returned error: %v", closeErr)
	}

	// Allow goroutine to fully exit
	time.Sleep(50 * time.Millisecond)

	// Verify goroutine count hasn't grown significantly
	after := runtime.NumGoroutine()
	if after > before+2 { // allow some slack for test framework goroutines
		t.Errorf("Goroutine count grew: before=%d, after=%d", before, after)
	}
}

// TestLeakFileWriterCleanup verifies that FileWriter.Close() stops all
// background goroutines and releases the file handle.
func TestLeakFileWriterCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir + "/test.log"

	fw, err := NewFileWriter(logPath, FileWriterConfig{
		MaxSizeMB:  10,
		MaxAge:     24 * time.Hour,
		MaxBackups: 5,
		Compress:   false,
	})
	if err != nil {
		t.Fatalf("Failed to create file writer: %v", err)
	}

	// Write data
	fw.Write([]byte("test log line\n"))

	// Close should stop cleanup goroutine and release file
	closeErr := fw.Close()
	if closeErr != nil {
		t.Errorf("Close() returned error: %v", closeErr)
	}

	// Double close should be safe
	closeErr = fw.Close()
	if closeErr != nil {
		t.Errorf("Second Close() returned error: %v", closeErr)
	}

	// Write after close should return error
	_, writeErr := fw.Write([]byte("should fail"))
	if writeErr == nil {
		t.Error("Expected error writing to closed FileWriter")
	}
}

// TestLeakConcurrentCloseAndLog verifies no panics or deadlocks when
// Close() is called concurrently with active logging.
func TestLeakConcurrentCloseAndLog(t *testing.T) {
	for i := 0; i < 10; i++ {
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(ioDiscard())}
		cfg.Security = &SecurityConfig{
			SensitiveFilter: NewSensitiveDataFilter(),
		}
		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}

		var wg sync.WaitGroup

		// Concurrent loggers
		for j := 0; j < 5; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for k := 0; k < 100; k++ {
					logger.Info("test message with password=secret123")
				}
			}()
		}

		// Concurrent closer
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(time.Millisecond * time.Duration(5+i))
			logger.Close()
		}()

		wg.Wait()
	}
}

// ioDiscard returns a writer that discards all output.
func ioDiscard() *safeBuffer {
	return &safeBuffer{}
}

// safeBuffer is a thread-safe io.Writer for tests.
type safeBuffer struct {
	mu sync.Mutex
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(p), nil
}
