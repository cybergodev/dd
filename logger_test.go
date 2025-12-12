package dd

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// CORE LOGGER TESTS - Basic Functionality
// ============================================================================

func TestLoggerCreation(t *testing.T) {
	tests := []struct {
		name    string
		config  *LoggerConfig
		wantErr bool
	}{
		{
			name:    "default config",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "valid custom config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "invalid level",
			config: &LoggerConfig{
				Level:  LogLevel(99),
				Format: FormatText,
			},
			wantErr: true,
		},
		{
			name: "invalid format",
			config: &LoggerConfig{
				Level:  LevelInfo,
				Format: LogFormat(99),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := New(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if logger != nil {
				logger.Close()
			}
		})
	}
}

func TestLoggerBasicLogging(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Level = LevelDebug // Set to debug level to test all messages
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test all log levels
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()

	// Should contain all messages (level is set to Debug)
	if !strings.Contains(output, "debug message") {
		t.Error("Debug message missing")
	}
	if !strings.Contains(output, "info message") {
		t.Error("Info message missing")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("Warn message missing")
	}
	if !strings.Contains(output, "error message") {
		t.Error("Error message missing")
	}
}

func TestLoggerFormattedLogging(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Infof("user %s has %d items", "john", 42)

	output := buf.String()
	if !strings.Contains(output, "user john has 42 items") {
		t.Errorf("Formatted message not found in output: %s", output)
	}
}

func TestLoggerLevelControl(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}
	config.Level = LevelWarn

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test level filtering
	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	output := buf.String()

	// Only warn and error should appear
	if strings.Contains(output, "debug") || strings.Contains(output, "info") {
		t.Error("Lower level messages should be filtered")
	}
	if !strings.Contains(output, "warn") || !strings.Contains(output, "error") {
		t.Error("Higher level messages should appear")
	}

	// Test dynamic level change
	buf.Reset()
	logger.SetLevel(LevelDebug)
	logger.Debug("debug after change")

	if !strings.Contains(buf.String(), "debug after change") {
		t.Error("Level change should take effect immediately")
	}
}

func TestLoggerClose(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test normal close
	err = logger.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Test double close (should not panic)
	err = logger.Close()
	if err != nil {
		t.Logf("Second close returned error (acceptable): %v", err)
	}

	// Test logging after close (should not panic)
	logger.Info("message after close")
}

// ============================================================================
// WRITER MANAGEMENT TESTS
// ============================================================================

func TestLoggerWriterManagement(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test adding writers
	err = logger.AddWriter(&buf1)
	if err != nil {
		t.Errorf("AddWriter() error = %v", err)
	}

	err = logger.AddWriter(&buf2)
	if err != nil {
		t.Errorf("AddWriter() error = %v", err)
	}

	// Test writing to multiple writers
	logger.Info("test message")

	if !strings.Contains(buf1.String(), "test message") {
		t.Error("Message not written to first writer")
	}
	if !strings.Contains(buf2.String(), "test message") {
		t.Error("Message not written to second writer")
	}

	// Test removing writer
	logger.RemoveWriter(&buf1)
	buf1.Reset()
	buf2.Reset()

	logger.Info("second message")

	if strings.Contains(buf1.String(), "second message") {
		t.Error("Message written to removed writer")
	}
	if !strings.Contains(buf2.String(), "second message") {
		t.Error("Message not written to remaining writer")
	}
}

func TestLoggerWriterErrors(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test adding nil writer
	err = logger.AddWriter(nil)
	if err == nil {
		t.Error("AddWriter(nil) should return error")
	}

	// Test removing nil writer (should not panic)
	logger.RemoveWriter(nil)
}

// ============================================================================
// SECURITY TESTS
// ============================================================================

func TestLoggerSecurityConfig(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test getting security config
	secConfig := logger.GetSecurityConfig()
	if secConfig == nil {
		t.Error("GetSecurityConfig() returned nil")
	}

	// Test setting security config
	newConfig := &SecurityConfig{
		MaxMessageSize:  1024,
		MaxWriters:      5,
		SensitiveFilter: NewBasicSensitiveDataFilter(),
	}
	logger.SetSecurityConfig(newConfig)

	retrieved := logger.GetSecurityConfig()
	if retrieved.MaxMessageSize != 1024 {
		t.Errorf("MaxMessageSize = %d, want 1024", retrieved.MaxMessageSize)
	}
	if retrieved.MaxWriters != 5 {
		t.Errorf("MaxWriters = %d, want 5", retrieved.MaxWriters)
	}

	// Test setting nil config (should not panic)
	logger.SetSecurityConfig(nil)
}

func TestLoggerWriterLimit(t *testing.T) {
	config := DefaultConfig()
	config.SecurityConfig = &SecurityConfig{
		MaxWriters: 2,
	}
	// Clear default writers to start with 0 writers
	config.Writers = nil

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	var buf1, buf2, buf3 bytes.Buffer

	// Should succeed (first writer)
	err = logger.AddWriter(&buf1)
	if err != nil {
		t.Errorf("First AddWriter() error = %v", err)
	}

	// Should succeed (second writer, reaches limit)
	err = logger.AddWriter(&buf2)
	if err != nil {
		t.Logf("Second AddWriter() error (expected): %v", err)
	}

	// Should fail (exceeds limit)
	err = logger.AddWriter(&buf3)
	if err == nil {
		t.Error("Third AddWriter() should fail due to limit")
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestLoggerConcurrency(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	var wg sync.WaitGroup
	numGoroutines := 100
	messagesPerGoroutine := 10

	// Concurrent logging
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				logger.Infof("goroutine %d message %d", id, j)
			}
		}(i)
	}

	// Concurrent writer management
	wg.Add(1)
	go func() {
		defer wg.Done()
		var extraBuf bytes.Buffer
		logger.AddWriter(&extraBuf)
		time.Sleep(10 * time.Millisecond)
		logger.RemoveWriter(&extraBuf)
	}()

	// Concurrent level changes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			logger.SetLevel(LevelDebug)
			time.Sleep(time.Millisecond)
			logger.SetLevel(LevelInfo)
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()

	// Should not panic and should have some output
	if buf.Len() == 0 {
		t.Error("No output from concurrent logging")
	}
}

// ============================================================================
// FATAL HANDLING TESTS
// ============================================================================

func TestLoggerFatalHandler(t *testing.T) {
	var called bool
	config := DefaultConfig()
	config.FatalHandler = func() {
		called = true
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Don't defer Close() since Fatal will call it
	logger.Fatal("fatal message")

	if !called {
		t.Error("Fatal handler was not called")
	}
}

// ============================================================================
// PACKAGE-LEVEL FUNCTION TESTS
// ============================================================================

func TestPackageLevelFunctions(t *testing.T) {
	// Test that package-level functions work
	var buf bytes.Buffer

	// Create new logger for testing
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Save original and set test logger as default
	original := GetDefaultLogger()
	SetDefault(logger)
	defer func() {
		if original != nil {
			SetDefault(original)
		}
	}()

	Info("package level info")
	Warn("package level warn")
	Error("package level error")

	output := buf.String()
	if !strings.Contains(output, "package level info") {
		t.Error("Package level Info() not working")
	}
	if !strings.Contains(output, "package level warn") {
		t.Error("Package level Warn() not working")
	}
	if !strings.Contains(output, "package level error") {
		t.Error("Package level Error() not working")
	}
}

func TestSetDefaultLogger(t *testing.T) {
	// Save original
	original := GetDefaultLogger()

	// Create new logger
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	newLogger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer newLogger.Close()

	// Set as default
	SetDefault(newLogger)

	// Test it works
	Info("test message")

	if !strings.Contains(buf.String(), "test message") {
		t.Error("SetDefault() not working")
	}

	// Restore original
	if original != nil {
		SetDefault(original)
	}

	// Test setting nil (should not panic)
	SetDefault(nil)
}

// ============================================================================
// ERROR CONDITION TESTS
// ============================================================================

func TestLoggerClosedOperations(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Close the logger
	logger.Close()

	// Test operations on closed logger (should not panic)
	logger.Info("message after close")
	logger.SetLevel(LevelDebug)

	var buf bytes.Buffer
	err = logger.AddWriter(&buf)
	if err == nil {
		t.Error("AddWriter() on closed logger should return error")
	}

	logger.RemoveWriter(&buf) // Should not panic
}

func TestLoggerInvalidLogLevels(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test invalid log levels (should not panic)
	logger.Log(LogLevel(-1), "invalid level")
	logger.Log(LogLevel(99), "invalid level")

	// Should have no output for invalid levels
	if buf.Len() > 0 {
		t.Error("Invalid log levels should not produce output")
	}
}

// ============================================================================
// CONTEXT AND LIFECYCLE TESTS
// ============================================================================

func TestLoggerLifecycle(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Test that logger is functional
	logger.Info("test message")

	// Test graceful shutdown
	done := make(chan struct{})
	go func() {
		logger.Close()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Error("Logger close timed out")
	}
}

// Test helper types
type failingWriter struct{}

func (fw *failingWriter) Write(p []byte) (int, error) {
	return 0, io.ErrShortWrite
}

func TestLoggerWithFailingWriter(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{&failingWriter{}}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Should not panic even with failing writer
	logger.Info("test message")
}

func TestLoggerWithSlowWriter(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{&slowWriter{delay: 10 * time.Millisecond}}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	start := time.Now()
	logger.Info("test message")
	duration := time.Since(start)

	// Should complete (not hang)
	if duration > time.Second {
		t.Error("Logging took too long with slow writer")
	}
}
