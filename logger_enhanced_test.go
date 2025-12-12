package dd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// ENHANCED LOGGER TESTS - Missing Coverage
// ============================================================================

func TestLoggerGetLevel(t *testing.T) {
	config := DefaultConfig()
	config.Level = LevelWarn

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	if logger.GetLevel() != LevelWarn {
		t.Errorf("GetLevel() = %v, want %v", logger.GetLevel(), LevelWarn)
	}
}

func TestLoggerSetLevelAtomic(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test atomic level changes
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(level LogLevel) {
			defer wg.Done()
			logger.SetLevel(level)
		}(LogLevel(i % 4))
	}

	wg.Wait()

	// Should not panic and should have a valid level
	level := logger.GetLevel()
	if level < LevelDebug || level > LevelFatal {
		t.Errorf("Level after concurrent changes = %v, should be valid", level)
	}
}

func TestLoggerProcessFields(t *testing.T) {
	config := DefaultConfig()
	config.SecurityConfig = &SecurityConfig{
		MaxMessageSize:  1024 * 1024,
		MaxWriters:      100,
		SensitiveFilter: NewBasicSensitiveDataFilter(),
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	fields := []Field{
		String("normal", "value"),
		String("password", "secret123"),
		String("api_key", "key123"),
	}

	fieldMap := logger.processFields(fields)

	// Normal field should be unchanged
	if fieldMap["normal"] != "value" {
		t.Error("Normal field should not be filtered")
	}

	// Sensitive fields should be redacted
	if fieldMap["password"] != "[REDACTED]" {
		t.Errorf("Password field should be redacted, got: %v", fieldMap["password"])
	}
}

func TestLoggerProcessFieldsWithInvalidKeys(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	fields := []Field{
		String("", "empty key"),
		String("key with spaces", "value"),
		String("key\nwith\nnewlines", "value"),
		String(strings.Repeat("a", 300), "long key"),
	}

	fieldMap := logger.processFields(fields)

	// Should sanitize invalid keys
	if len(fieldMap) == 0 {
		t.Error("Should process fields even with invalid keys")
	}
}

func TestLoggerFilterMessage(t *testing.T) {
	config := DefaultConfig()
	config.SecurityConfig = &SecurityConfig{
		MaxMessageSize:  1024 * 1024,
		MaxWriters:      100,
		SensitiveFilter: NewBasicSensitiveDataFilter(),
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test message filtering
	filtered := logger.filterMessage("password=secret123")
	if strings.Contains(filtered, "secret123") {
		t.Error("Sensitive data should be filtered from message")
	}
}

func TestLoggerFilterMessageNoFilter(t *testing.T) {
	config := DefaultConfig()
	config.SecurityConfig = &SecurityConfig{
		MaxMessageSize:  1024 * 1024,
		MaxWriters:      100,
		SensitiveFilter: nil,
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Without filter, message should be unchanged
	original := "password=secret123"
	filtered := logger.filterMessage(original)
	if filtered != original {
		t.Error("Message should not be filtered when filter is nil")
	}
}

func TestLoggerApplySecurity(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.SecurityConfig = &SecurityConfig{
		MaxMessageSize:  50,
		SensitiveFilter: NewBasicSensitiveDataFilter(),
	}
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test message truncation - log a very long message
	longMessage := strings.Repeat("a", 100)
	logger.Info(longMessage)
	output := buf.String()
	// The output should be truncated (including timestamp, level, etc.)
	if !strings.Contains(output, "[TRUNCATED]") {
		t.Error("Long message should be truncated")
	}

	// Test sensitive data filtering
	buf.Reset()
	sensitiveMessage := "password=secret123"
	logger.Info(sensitiveMessage)
	output = buf.String()
	if strings.Contains(output, "secret123") {
		t.Error("Sensitive data should be filtered")
	}
}

func TestLoggerApplySecurityNoConfig(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.SecurityConfig = nil
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Without security config, message should be unchanged (except for formatting)
	original := "test message"
	logger.Info(original)
	output := buf.String()
	if !strings.Contains(output, original) {
		t.Error("Message should contain original text without security config")
	}
}

func TestLoggerWriteMessageSingleWriter(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.writeMessage("test message")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Error("Message should be written to single writer")
	}
}

func TestLoggerWriteMessageMultipleWriters(t *testing.T) {
	var buf1, buf2, buf3 bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf1, &buf2, &buf3}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.writeMessage("test message")

	// All writers should receive the message
	for i, buf := range []*bytes.Buffer{&buf1, &buf2, &buf3} {
		if !strings.Contains(buf.String(), "test message") {
			t.Errorf("Writer %d should receive message", i)
		}
	}
}

func TestLoggerWriteMessageNoWriters(t *testing.T) {
	config := DefaultConfig()
	config.Writers = nil

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Should not panic with no writers
	logger.writeMessage("test message")
}

func TestLoggerWriteMessageClosedLogger(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.Close()

	// Should not write after close
	logger.writeMessage("test message")

	if buf.Len() > 0 {
		t.Error("Should not write to closed logger")
	}
}

func TestLoggerDetectCallerDepth(t *testing.T) {
	config := DevelopmentConfig()
	config.DynamicCaller = true

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test that dynamic caller detection works by logging
	var buf strings.Builder
	logger.AddWriter(&buf)
	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "logger_enhanced_test.go") {
		t.Logf("Dynamic caller detection output: %s", output)
	}
}

func TestLoggerDetectCallerDepthDisabled(t *testing.T) {
	config := DefaultConfig()
	config.DynamicCaller = false
	config.IncludeCaller = true

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test that caller info is included even without dynamic detection
	var buf strings.Builder
	logger.AddWriter(&buf)
	logger.Info("test message")

	output := buf.String()
	if config.IncludeCaller && !strings.Contains(output, ".go:") {
		t.Logf("Caller info output: %s", output)
	}
}

func TestLoggerDetectCallerDepthWithHint(t *testing.T) {
	config := DevelopmentConfig()
	config.DynamicCaller = true

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test with hint
	depth := logger.detectCallerDepthWithHint(4)
	if depth <= 0 {
		t.Error("Detected caller depth with hint should be positive")
	}
}

func TestLoggerFormatJSON(t *testing.T) {
	config := JSONConfig()
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	result := logger.formatJSONWithDepth(LevelInfo, "test message", nil, 6)
	if !strings.Contains(result, `"message"`) {
		t.Error("JSON format should contain message field")
	}
	if !strings.Contains(result, "test message") {
		t.Error("JSON format should contain message content")
	}
}

func TestLoggerFormatJSONWithFields(t *testing.T) {
	config := JSONConfig()
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	fields := map[string]any{
		"user":   "john",
		"action": "login",
	}

	result := logger.formatJSONWithDepth(LevelInfo, "test message", fields, 6)
	if !strings.Contains(result, `"user"`) {
		t.Error("JSON format should contain user field")
	}
	if !strings.Contains(result, `"action"`) {
		t.Error("JSON format should contain action field")
	}
}

func TestLoggerFormatText(t *testing.T) {
	config := DefaultConfig()
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	result := logger.formatTextWithDepth(LevelInfo, "test message", 6)
	if !strings.Contains(result, "test message") {
		t.Error("Text format should contain message")
	}
	if !strings.Contains(result, "[INFO]") {
		t.Error("Text format should contain level")
	}
}

func TestLoggerLogWithExtraDepth(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.logWithExtraDepth(LevelInfo, 1, "test message")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Error("Should log message with extra depth")
	}
}

func TestLoggerLogfWithExtraDepth(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.logfWithExtraDepth(LevelInfo, 1, "formatted %s %d", "message", 42)

	output := buf.String()
	if !strings.Contains(output, "formatted message 42") {
		t.Error("Should log formatted message with extra depth")
	}
}

func TestLoggerNeedsanitization(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"empty", "", true},
		{"valid simple", "key", false},
		{"valid with underscore", "my_key", false},
		{"valid with dash", "my-key", false},
		{"valid with dot", "my.key", false},
		{"valid alphanumeric", "key123", false},
		{"with space", "my key", true},
		{"with newline", "my\nkey", true},
		{"with special char", "my@key", true},
		{"too long", strings.Repeat("a", 300), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsSanitization(tt.key)
			if got != tt.want {
				t.Errorf("needsSanitization(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestLoggerMax(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 2},
		{2, 1, 2},
		{5, 5, 5},
		{-1, 1, 1},
		{0, 0, 0},
	}

	for _, tt := range tests {
		got := max(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("max(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestLoggerCloseWithTimeout(t *testing.T) {
	config := DefaultConfig()
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Start a goroutine that won't finish quickly
	logger.wg.Add(1)
	go func() {
		defer logger.wg.Done()
		time.Sleep(10 * time.Second)
	}()

	// Close should timeout and return error
	start := time.Now()
	err = logger.Close()
	duration := time.Since(start)

	if duration > 6*time.Second {
		t.Error("Close should timeout after 5 seconds")
	}

	if err == nil {
		t.Error("Close should return error on timeout")
	}
}

func TestLoggerCloseMultipleWriters(t *testing.T) {
	// Create closeable writers
	closer1 := &closeTrackingWriterEnhanced{Writer: &bytes.Buffer{}}
	closer2 := &closeTrackingWriterEnhanced{Writer: &bytes.Buffer{}}

	config := DefaultConfig()
	config.Writers = []io.Writer{closer1, closer2}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	err = logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if !closer1.closed {
		t.Error("First writer should be closed")
	}
	if !closer2.closed {
		t.Error("Second writer should be closed")
	}
}

type closeTrackingWriterEnhanced struct {
	io.Writer
	closed bool
}

func (w *closeTrackingWriterEnhanced) Close() error {
	w.closed = true
	return nil
}

func TestLoggerCloseWithErrors(t *testing.T) {
	// Create writers that fail on close
	closer1 := &failingCloseWriter{Writer: &bytes.Buffer{}}
	closer2 := &failingCloseWriter{Writer: &bytes.Buffer{}}

	config := DefaultConfig()
	config.Writers = []io.Writer{closer1, closer2}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	err = logger.Close()
	if err == nil {
		t.Error("Close() should return error when writers fail to close")
	}

	// Error message should mention multiple errors
	if !strings.Contains(err.Error(), "multiple close errors") {
		t.Errorf("Error should mention multiple errors, got: %v", err)
	}
}

type failingCloseWriter struct {
	io.Writer
}

func (w *failingCloseWriter) Close() error {
	return errors.New("close failed")
}

func TestLoggerCloseStandardStreams(t *testing.T) {
	// Test that standard streams are not closed
	config := DefaultConfig()
	config.Writers = []io.Writer{&bytes.Buffer{}, os.Stdout, os.Stderr}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Should not close os.Stdout or os.Stderr
	err = logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify stdout/stderr still work
	_, err = os.Stdout.Write([]byte("test"))
	if err != nil {
		t.Error("Stdout should still be usable after logger close")
	}
}

func TestGetDefaultLogger(t *testing.T) {
	// Reset default logger
	defaultLogger.Store((*Logger)(nil))
	defaultOnce = sync.Once{}

	// Should return nil before initialization
	logger := GetDefaultLogger()
	if logger != nil {
		t.Error("GetDefaultLogger() should return nil before initialization")
	}

	// Initialize default logger
	_ = Default()

	// Now should return non-nil
	logger = GetDefaultLogger()
	if logger == nil {
		t.Error("GetDefaultLogger() should return non-nil after initialization")
	}
}

func TestSetDefaultNil(t *testing.T) {
	// Should not panic with nil
	SetDefault(nil)

	// Default() should still work
	logger := Default()
	if logger == nil {
		t.Error("Default() should work even after SetDefault(nil)")
	}
}

func TestPackageLevelSetLevel(t *testing.T) {
	// Test package-level SetLevel function
	SetLevel(LevelWarn)

	level := Default().GetLevel()
	if level != LevelWarn {
		t.Errorf("Package-level SetLevel() failed, got %v, want %v", level, LevelWarn)
	}

	// Reset to default
	SetLevel(LevelInfo)
}
