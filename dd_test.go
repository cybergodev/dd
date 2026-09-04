package dd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// LOGGER CREATION AND CONFIGURATION TESTS
// ============================================================================

// TestLoggerCreation is covered by TestNewConfig("build logger") in builder_test.go

func TestLoggerSetSecurityConfig(t *testing.T) {
	cfg := DefaultConfig()
	logger, _ := New(cfg)

	secConfig := &SecurityConfig{
		MaxMessageSize:  1000,
		MaxWriters:      10,
		SensitiveFilter: newBasicSensitiveDataFilter(),
	}

	logger.SetSecurityConfig(secConfig)

	retrieved := logger.GetSecurityConfig()
	if retrieved == nil {
		t.Fatal("GetSecurityConfig() should return config")
	}
	if retrieved.MaxMessageSize != 1000 {
		t.Errorf("MaxMessageSize = %d, want 1000", retrieved.MaxMessageSize)
	}
	if retrieved.SensitiveFilter == nil {
		t.Error("SensitiveFilter should be set")
	}
}

// ============================================================================
// BASIC LOGGING TESTS
// ============================================================================

func TestBasicLogging(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	tests := []struct {
		name   string
		level  LogLevel
		method func(...any)
	}{
		{"Debug", LevelDebug, func(args ...any) { logger.Debug(args...) }},
		{"Info", LevelInfo, func(args ...any) { logger.Info(args...) }},
		{"Warn", LevelWarn, func(args ...any) { logger.Warn(args...) }},
		{"Error", LevelError, func(args ...any) { logger.Error(args...) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.method("test message")
			if tt.level >= LevelInfo && buf.Len() == 0 {
				t.Errorf("%s should write output", tt.name)
			}
		})
	}
}

func TestAllFormattedMethods(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelDebug
	logger, _ := New(cfg)

	tests := []struct {
		name   string
		method func(string, ...any)
	}{
		{"Debugf", logger.Debugf},
		{"Infof", logger.Infof},
		{"Warnf", logger.Warnf},
		{"Errorf", logger.Errorf},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.method("test %s", "value")
			if buf.Len() == 0 {
				t.Errorf("%s should write output", tt.name)
			}
		})
	}
}

func TestStructuredLogging(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	logger.InfoWith("test", String("key", "value"), Int("count", 42))
	output := buf.String()

	if !strings.Contains(output, "key=value") {
		t.Error("Structured logging should include fields")
	}
	if !strings.Contains(output, "count=42") {
		t.Error("Structured logging should include count")
	}
}

func TestAllStructuredLoggingMethods(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelDebug
	logger, _ := New(cfg)

	tests := []struct {
		name   string
		method func(string, ...Field)
	}{
		{"DebugWith", logger.DebugWith},
		{"InfoWith", logger.InfoWith},
		{"WarnWith", logger.WarnWith},
		{"ErrorWith", logger.ErrorWith},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.method("test", String("key", "value"))
			if buf.Len() == 0 {
				t.Errorf("%s should write output", tt.name)
			}
		})
	}
}

func TestFatalLogging(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(*Logger)
		expected string
	}{
		{
			name:     "Fatal",
			logFunc:  func(l *Logger) { l.Fatal("fatal message") },
			expected: "fatal message",
		},
		{
			name:     "Fatalf",
			logFunc:  func(l *Logger) { l.Fatalf("fatal %s", "message") },
			expected: "fatal message",
		},
		{
			name:     "FatalWith",
			logFunc:  func(l *Logger) { l.FatalWith("fatal", String("key", "value")) },
			expected: "fatal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			exited := false

			cfg := DefaultConfig()
			cfg.Targets = []OutputTarget{CustomOutput(&buf)}
			cfg.Level = LevelInfo
			cfg.FatalHandler = func() { exited = true }
			logger, _ := New(cfg)

			tt.logFunc(logger)

			if !exited {
				t.Error("Fatal handler should be called")
			}
			if !strings.Contains(buf.String(), tt.expected) {
				t.Errorf("Expected %q in output", tt.expected)
			}
		})
	}
}

// TestJSONLogging is covered by TestJSONLoggingPipeline below

// ============================================================================
// LOG LEVEL MANAGEMENT TESTS
// ============================================================================

func TestLoggerLevelManagement(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	// Test level filtering
	logger.Debug("debug message")
	if buf.Len() != 0 {
		t.Error("Debug message should not be logged at Info level")
	}

	logger.Info("info message")
	if buf.Len() == 0 {
		t.Error("Info message should be logged at Info level")
	}

	// Test level change
	buf.Reset()
	logger.SetLevel(LevelDebug)
	logger.Debug("debug message")
	if buf.Len() == 0 {
		t.Error("Debug message should be logged after setting level to Debug")
	}

	// Test invalid level
	err := logger.SetLevel(LogLevel(99))
	if err == nil {
		t.Error("SetLevel() should return error for invalid level")
	}
}

// TestGlobalFunctions consolidates all global function tests into a single
// table-driven test for better maintainability and reduced boilerplate.
func TestGlobalFunctions(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelDebug
	logger, _ := New(cfg)
	SetDefault(logger)

	// Ensure level is reset before each subtest
	resetLevel := func() {
		SetLevel(LevelDebug)
		buf.Reset()
	}

	t.Run("LevelManagement", func(t *testing.T) {
		tests := []struct {
			setLevel  LogLevel
			wantLevel LogLevel
		}{
			{LevelInfo, LevelInfo},
			{LevelDebug, LevelDebug},
			{LevelError, LevelError},
		}
		for _, tt := range tests {
			t.Run(tt.setLevel.String(), func(t *testing.T) {
				SetLevel(tt.setLevel)
				if got := GetLevel(); got != tt.wantLevel {
					t.Errorf("GetLevel() = %v, want %v", got, tt.wantLevel)
				}
			})
		}
	})

	t.Run("BasicLogging", func(t *testing.T) {
		resetLevel()
		tests := []struct {
			name    string
			method  func(...any)
			message string
		}{
			{"Debug", Debug, "debug msg"},
			{"Info", Info, "info msg"},
			{"Warn", Warn, "warn msg"},
			{"Error", Error, "error msg"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				buf.Reset()
				tt.method(tt.message)
				if !strings.Contains(buf.String(), tt.message) {
					t.Errorf("Global %s should log message", tt.name)
				}
			})
		}
	})

	t.Run("FormattedLogging", func(t *testing.T) {
		resetLevel()
		tests := []struct {
			name   string
			method func(string, ...any)
		}{
			{"Debugf", Debugf},
			{"Infof", Infof},
			{"Warnf", Warnf},
			{"Errorf", Errorf},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				buf.Reset()
				tt.method("test %s", "value")
				if !strings.Contains(buf.String(), "test value") {
					t.Errorf("Global %s should format message", tt.name)
				}
			})
		}
	})

	t.Run("StructuredLogging", func(t *testing.T) {
		resetLevel()
		tests := []struct {
			name   string
			method func(string, ...Field)
		}{
			{"DebugWith", DebugWith},
			{"InfoWith", InfoWith},
			{"WarnWith", WarnWith},
			{"ErrorWith", ErrorWith},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				buf.Reset()
				tt.method("structured", String("key", "value"))
				output := buf.String()
				if !strings.Contains(output, "structured") {
					t.Errorf("Global %s should log message", tt.name)
				}
				if !strings.Contains(output, "key=value") {
					t.Errorf("Global %s should include field", tt.name)
				}
			})
		}
	})
}

// ============================================================================
// WRITER MANAGEMENT TESTS
// ============================================================================

func TestWriterManagement(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf1)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	// Add writer
	err := logger.AddWriter(&buf2)
	if err != nil {
		t.Fatalf("AddWriter() error = %v", err)
	}

	logger.Info("test")
	if buf1.Len() == 0 || buf2.Len() == 0 {
		t.Error("Message should be written to both writers")
	}

	// Remove writer
	err = logger.RemoveWriter(&buf2)
	if err != nil {
		t.Fatalf("RemoveWriter() error = %v", err)
	}

	buf1.Reset()
	buf2.Reset()
	logger.Info("test 2")
	if buf2.Len() != 0 {
		t.Error("Message should not be written to removed writer")
	}
}

func TestWriterTypes(t *testing.T) {
	t.Run("FileWriter", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "test.log")
		fw, err := NewFileWriter(tmpFile, FileWriterConfig{
			MaxSizeMB:  1,
			MaxBackups: 3,
			MaxAge:     24 * time.Hour,
			Compress:   false,
		})
		if err != nil {
			t.Fatalf("NewFileWriter() error = %v", err)
		}
		defer fw.Close()

		data := []byte("test log message\n")
		n, err := fw.Write(data)
		if err != nil {
			t.Errorf("Write() error = %v", err)
		}
		if n != len(data) {
			t.Errorf("Write() wrote %d bytes, want %d", n, len(data))
		}
	})

	t.Run("BufferedWriter", func(t *testing.T) {
		var buf bytes.Buffer
		bw, err := NewBufferedWriter(&buf, BufferedWriterConfig{BufferSize: 4096})
		if err != nil {
			t.Fatalf("NewBufferedWriter() error = %v", err)
		}
		defer bw.Close()

		data := []byte("test message\n")
		n, err := bw.Write(data)
		if err != nil {
			t.Errorf("Write() error = %v", err)
		}
		if n != len(data) {
			t.Errorf("Write() wrote %d bytes, want %d", n, len(data))
		}

		bw.Flush()
		if buf.Len() == 0 {
			t.Error("BufferedWriter should have written data after flush")
		}
	})

	t.Run("BufferedWriterAutoFlush", func(t *testing.T) {
		var buf bytes.Buffer
		bw, err := NewBufferedWriter(&buf, BufferedWriterConfig{BufferSize: 4096})
		if err != nil {
			t.Fatalf("NewBufferedWriter() error = %v", err)
		}
		defer bw.Close()

		largeData := make([]byte, 2048)
		for i := 0; i < 10; i++ {
			bw.Write(largeData)
		}
		time.Sleep(100 * time.Millisecond)

		if buf.Len() == 0 {
			t.Error("Auto-flush should have written some data")
		}
	})
}

func TestMultiWriter(t *testing.T) {
	var buf1, buf2, buf3 bytes.Buffer
	mw := NewMultiWriter(&buf1, &buf2, &buf3)

	data := []byte("test message\n")
	n, err := mw.Write(data)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() wrote %d bytes, want %d", n, len(data))
	}

	if buf1.Len() == 0 || buf2.Len() == 0 || buf3.Len() == 0 {
		t.Error("MultiWriter should write to all writers")
	}
}

func TestMultiWriterManagement(t *testing.T) {
	var buf1, buf2, buf3 bytes.Buffer
	mw := NewMultiWriter(&buf1, &buf2)

	// Test AddWriter
	if err := mw.AddWriter(&buf3); err != nil {
		t.Errorf("AddWriter failed: %v", err)
	}
	data := []byte("test\n")
	mw.Write(data)

	if buf3.Len() == 0 {
		t.Error("AddWriter should add writer to MultiWriter")
	}

	// Test RemoveWriter
	buf1.Reset()
	buf2.Reset()
	buf3.Reset()
	mw.RemoveWriter(&buf2)
	mw.Write(data)

	if buf2.Len() != 0 {
		t.Error("RemoveWriter should remove writer from MultiWriter")
	}
	if buf1.Len() == 0 || buf3.Len() == 0 {
		t.Error("Remaining writers should still work")
	}

	// Test Close
	mw.Close()
}

func TestMultiWriterClose(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.log")
	fw, _ := NewFileWriter(tmpFile, DefaultFileWriterConfig())

	var buf bytes.Buffer
	mw := NewMultiWriter(&buf, fw)

	err := mw.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// File should be closed
	_, err = fw.Write([]byte("test"))
	if err == nil {
		t.Error("FileWriter should be closed after MultiWriter.Close()")
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestConcurrentLogging(t *testing.T) {
	safeWriter := &threadSafeWriter{w: &bytes.Buffer{}}
	cfg := DefaultConfig()
	cfg.Level = LevelInfo
	cfg.IncludeTime = false
	cfg.IncludeLevel = false
	cfg.Targets = []OutputTarget{CustomOutput(safeWriter)}
	cfg.Security = &SecurityConfig{SensitiveFilter: nil}
	logger, _ := New(cfg)

	const goroutines = 100
	const messagesPerGoroutine = 10

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				logger.Info(fmt.Sprintf("goroutine %d message %d", id, j))
			}
		}(i)
	}
	wg.Wait()

	output := safeWriter.String()
	if len(output) == 0 {
		t.Error("Concurrent logging should produce output")
	}
}

// threadSafeWriter wraps a writer with mutex for thread-safe writes in tests
type threadSafeWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (t *threadSafeWriter) Write(p []byte) (n int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.w.Write(p)
}

func (t *threadSafeWriter) String() string {
	if buf, ok := t.w.(*bytes.Buffer); ok {
		t.mu.Lock()
		defer t.mu.Unlock()
		return buf.String()
	}
	return ""
}

// ============================================================================
// SECURITY AND SANITIZATION TESTS
// ============================================================================

// TestHookOnFilterFiresOnRedaction verifies that the previously-never-triggered
// HookOnFilter event now fires when sensitive data is redacted, carrying the
// redacted field's key (never its value).
func TestHookOnFilterFiresOnRedaction(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Level = LevelInfo
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Security = DefaultSecurityConfig()

	var fired int
	var seenKey string

	logger, _ := New(cfg)
	logger.AddHook(HookOnFilter, func(ctx context.Context, h *HookContext) error {
		fired++
		if h.Metadata != nil {
			if k, ok := h.Metadata["field"].(string); ok {
				seenKey = k
			}
		}
		return nil
	})

	// "password" is a sensitive key → its value is redacted → HookOnFilter fires.
	logger.InfoWith("login attempt", String("password", "secret123"))
	// Non-sensitive content → no redaction → hook must not fire.
	logger.InfoWith("plain message", String("user", "alice"))
	logger.Close()

	if fired != 1 {
		t.Fatalf("HookOnFilter fired %d time(s), want 1 (once for the redacted field)", fired)
	}
	if seenKey != "password" {
		t.Errorf("HookOnFilter metadata field = %q, want %q", seenKey, "password")
	}
	// The sensitive value must never reach the output.
	if strings.Contains(buf.String(), "secret123") {
		t.Error("redacted value leaked to the output")
	}
}

// ============================================================================
// EDGE CASES AND ERROR HANDLING TESTS
// ============================================================================

func TestLoggerEdgeCases(t *testing.T) {
	t.Run("NilWriter", func(t *testing.T) {
		cfg := DefaultConfig()
		logger, _ := New(cfg)

		if err := logger.AddWriter(nil); err == nil {
			t.Error("AddWriter(nil) should return error")
		}
		if err := logger.RemoveWriter(nil); err == nil {
			t.Error("RemoveWriter(nil) should return error")
		}
	})

	t.Run("MaxWritersExceeded", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf)}
		logger, _ := New(cfg)

		writers := make([]io.Writer, 0, 100)
		for i := 0; i < 99; i++ {
			var b bytes.Buffer
			writers = append(writers, &b)
		}
		logger.writersPtr.Store(&writers)

		var buf100 bytes.Buffer
		if err := logger.AddWriter(&buf100); err != nil {
			t.Errorf("Adding 100th writer should succeed, got %v", err)
		}

		var buf101 bytes.Buffer
		if err := logger.AddWriter(&buf101); err == nil {
			t.Error("Adding 101st writer should fail")
		}
	})

	t.Run("ClosedLogger", func(t *testing.T) {
		cfg := DefaultConfig()
		logger, _ := New(cfg)
		logger.Close()

		var buf bytes.Buffer
		if err := logger.AddWriter(&buf); err == nil {
			t.Error("AddWriter on closed logger should fail")
		}
		logger.Info("test") // Should not panic
	})

	t.Run("EmptyAndNilInputs", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf)}
		cfg.Level = LevelInfo
		logger, _ := New(cfg)

		logger.Info()
		logger.Info("")
		if buf.Len() == 0 {
			t.Error("Empty Info calls should still produce some output")
		}

		buf.Reset()
		logger.InfoWith("test")
		if buf.Len() == 0 {
			t.Error("InfoWith with no fields should produce output")
		}
	})

	t.Run("LargeMessage", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf)}
		cfg.Level = LevelInfo
		logger, _ := New(cfg)

		logger.Info(strings.Repeat("test", 10000))
		if !strings.Contains(buf.String(), "test") {
			t.Error("Large message content should appear in output")
		}
	})

	t.Run("ManyFields", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf)}
		cfg.Level = LevelInfo
		logger, _ := New(cfg)

		fields := make([]Field, 100)
		for i := range 100 {
			fields[i] = Int(fmt.Sprintf("field%d", i), i)
		}
		logger.InfoWith("many fields", fields...)
		if buf.Len() == 0 {
			t.Error("Message with many fields should be logged")
		}
	})
}

// ============================================================================
// LOGGER LIFECYCLE TESTS
// ============================================================================

func TestLoggerClose(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.log")

	fw, _ := NewFileWriter(tmpFile, DefaultFileWriterConfig())
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(fw)}
	logger, _ := New(cfg)

	err := logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Double close should be safe
	err = logger.Close()
	if err != nil {
		t.Errorf("Second Close() should not error, got %v", err)
	}

	// Should not close stdout/stderr
	stdoutCfg := DefaultConfig()
	stdoutCfg.Targets = []OutputTarget{CustomOutput(os.Stdout)}
	stdoutLogger, _ := New(stdoutCfg)
	stdoutLogger.Close() // Should not panic or exit
}

// ============================================================================
// FIELD CONSTRUCTORS TESTS
// ============================================================================

func TestFieldConstructors(t *testing.T) {
	tests := []struct {
		name  string
		field Field
		key   string
	}{
		// String and basic types
		{"String", String("k", "v"), "k"},
		{"Int", Int("k", 42), "k"},
		{"Int8", Int8("k", 8), "k"},
		{"Int16", Int16("k", 16), "k"},
		{"Int32", Int32("k", 32), "k"},
		{"Int64", Int64("k", 64), "k"},
		{"Uint", Uint("k", 42), "k"},
		{"Uint8", Uint8("k", 8), "k"},
		{"Uint16", Uint16("k", 16), "k"},
		{"Uint32", Uint32("k", 32), "k"},
		{"Uint64", Uint64("k", 64), "k"},
		{"Bool", Bool("k", true), "k"},
		{"Float32", Float32("k", 3.14), "k"},
		{"Float64", Float64("k", 3.14), "k"},
		// Time types
		{"Duration", Duration("k", 5*time.Second), "k"},
		{"Time", Time("k", time.Now()), "k"},
		// Special types
		{"Any", Any("k", nil), "k"},
		{"Err", Err(nil), "error"},
		{"ErrWithValue", Err(errors.New("test error")), "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.field.Key != tt.key {
				t.Errorf("Field key = %q, want %q", tt.field.Key, tt.key)
			}
		})
	}
}

// TestFieldConstructorsWithLogging verifies that all field types work with actual logging
func TestFieldConstructorsWithLogging(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Format = FormatJSON
	cfg.Level = LevelDebug
	logger, _ := New(cfg)

	now := time.Now()
	tests := []struct {
		name   string
		fields []Field
	}{
		{"integer_types", []Field{
			Int("int", 42),
			Int8("int8", 8),
			Int16("int16", 16),
			Int32("int32", 32),
			Int64("int64", 64),
		}},
		{"unsigned_types", []Field{
			Uint("uint", 42),
			Uint8("uint8", 8),
			Uint16("uint16", 16),
			Uint32("uint32", 32),
			Uint64("uint64", 64),
		}},
		{"float_types", []Field{
			Float32("float32", 3.14),
			Float64("float64", 3.14159),
		}},
		{"time_types", []Field{
			Duration("duration", 5*time.Second),
			Time("time", now),
		}},
		{"special_types", []Field{
			Bool("bool", true),
			String("string", "value"),
			Err(errors.New("test")),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			logger.InfoWith("test", tt.fields...)
			if buf.Len() == 0 {
				t.Errorf("%s: expected output", tt.name)
			}
		})
	}
}

// ============================================================================
// FORMAT TESTS
// ============================================================================

// Note: TestLogFormatString and TestLogLevelString are in internal/types_test.go
// to avoid duplication and keep type tests with the type definitions.

func TestDefaultJSONOptions(t *testing.T) {
	opts := DefaultJSONOptions()

	if opts == nil {
		t.Fatal("DefaultJSONOptions() should not return nil")
	}
	if opts.PrettyPrint {
		t.Error("Default should not use pretty print")
	}
	if opts.Indent != defaultJSONIndent {
		t.Errorf("Default indent = %q, want %q", opts.Indent, defaultJSONIndent)
	}
	if opts.FieldNames == nil {
		t.Error("Default should have field names")
	}
}

// TestJSONFieldNamesDefaults was removed: internal/json_test.go
// TestDefaultJSONFieldNames asserts all five default names, co-located with
// the code under test.

func TestFullLoggingPipeline(t *testing.T) {
	tmpFile := t.TempDir() + "/test.log"

	fw, _ := NewFileWriter(tmpFile, DefaultFileWriterConfig())
	defer fw.Close()

	cfg := DefaultConfig()
	cfg.Level = LevelInfo
	cfg.IncludeTime = true
	cfg.IncludeLevel = true
	cfg.DynamicCaller = false
	cfg.Targets = []OutputTarget{CustomOutput(fw)}
	cfg.Security = DefaultSecurityConfig()
	logger, _ := New(cfg)

	// Test various logging methods
	logger.Info("simple message")
	logger.Infof("formatted %s", "message")
	logger.InfoWith("structured", String("key", "value"))

	// Verify file has content
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(data) == 0 {
		t.Error("Log file should contain data")
	}

	content := string(data)
	if !strings.Contains(content, "simple message") {
		t.Error("Log should contain simple message")
	}
	if !strings.Contains(content, "formatted message") {
		t.Error("Log should contain formatted message")
	}
	if !strings.Contains(content, "structured") {
		t.Error("Log should contain structured message")
	}
}

func TestJSONLoggingPipeline(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	cfg.Format = FormatJSON
	logger, _ := New(cfg)

	logger.InfoWith("test", String("key", "value"), Int("count", 42))

	output := buf.String()

	// Should be valid JSON
	var jsonData map[string]any
	if err := json.Unmarshal([]byte(output), &jsonData); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	// Should have required fields
	if jsonData["message"] == nil {
		t.Error("JSON should have message field")
	}
	if jsonData["level"] == nil {
		t.Error("JSON should have level field")
	}
}

// ============================================================================
// DEBUG VISUALIZATION TESTS
// ============================================================================

func TestDebugVisualization(t *testing.T) {
	tests := []struct {
		name     string
		call     func()
		contains string
	}{
		{"Text", func() { Text("test data") }, "test data"},
		{"Textf", func() { Textf("test: %s", "formatted") }, "test: formatted"},
		{"JSON", func() { JSON(map[string]string{"key": "value"}) }, `"key"`},
		{"JSONF", func() { JSONF("test: %s", "formatted") }, "test: formatted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w, _ := os.Pipe()
			oldStdout := os.Stdout
			os.Stdout = w

			tt.call()

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			if !strings.Contains(output, tt.contains) {
				t.Errorf("%s output = %q, want to contain %q", tt.name, output, tt.contains)
			}
		})
	}
}

// TestTypeConverter was removed: it re-tested internal.IsSimpleType and
// internal.FormatSimpleValue from the root package; internal/debug_test.go
// carries the complete tables for both.

func TestTypeConverterComplexTypes(t *testing.T) {
	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	// Test slice
	JSON([]int{1, 2, 3})

	// Test map
	JSON(map[string]int{"one": 1, "two": 2})

	// Test struct
	type TestStruct struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	JSON(TestStruct{Name: "John", Age: 30})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should be valid JSON output
	if !strings.Contains(output, "[") && !strings.Contains(output, "{") {
		t.Error("Complex types should be converted to JSON")
	}
}

// ============================================================================
// ERROR HANDLING TESTS
// ============================================================================

// TestErrorReturns was removed: each member is covered with a stronger
// assertion elsewhere — New() default (TestLogWithLazyMessageGate and many
// others), NewFileWriter invalid paths (coverage_test.go
// TestNewFileWriterInvalidPaths), NewBufferedWriter(nil) (boundary_test.go
// TestBufferedWriterBoundary).

// ============================================================================
// SECURITY FILTER TESTS (Consolidated from security_test.go)
// ============================================================================

func TestSensitiveDataFilter(t *testing.T) {
	tests := []struct {
		name     string
		filter   *SensitiveDataFilter
		input    string
		contains string
	}{
		// Full filter
		{"full/password", NewSensitiveDataFilter(), "password=secret123", "[REDACTED]"},
		{"full/api_key", NewSensitiveDataFilter(), "api_key=sk-1234567890", "[REDACTED]"},
		{"full/credit_card", NewSensitiveDataFilter(), "card number: 4532015112830366", "[REDACTED]"},
		{"full/email", NewSensitiveDataFilter(), "email: user@example.com", "[REDACTED]"},
		{"full/normal_text", NewSensitiveDataFilter(), "hello world", "hello world"},
		// Basic filter
		{"basic/password", newBasicSensitiveDataFilter(), "password=secret123", "[REDACTED]"},
		{"basic/token", newBasicSensitiveDataFilter(), "token=abc123xyz", "[REDACTED]"},
		{"basic/api_key", newBasicSensitiveDataFilter(), "api_key=sk-1234567890", "[REDACTED]"},
		{"basic/normal_text", newBasicSensitiveDataFilter(), "username=john_doe", "username=john_doe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.filter.Filter(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected %q in result, got: %s", tt.contains, result)
			}
		})
	}
}

// TestDefaultSecurityConfig was removed: security_test.go
// TestPresetConfigsHaveMaxWriters asserts the same fields for
// DefaultSecurityConfig and four other presets.

// ============================================================================
// CONCURRENT DEFAULT LOGGER TESTS
// ============================================================================

func TestDefaultLoggerConcurrent(t *testing.T) {
	// Reset the default logger for this test
	// This test verifies that concurrent access to Default() is safe

	var wg sync.WaitGroup
	const goroutines = 100

	// Concurrent calls to Default()
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger := Default()
			if logger == nil {
				t.Error("Default() should not return nil")
			}
		}()

		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cfg := DefaultConfig()
			logger, err := New(cfg)
			if err != nil {
				return
			}
			defer logger.Close()
			SetDefault(logger)
		}(i)
	}

	wg.Wait()

	// Verify final state is consistent
	logger := Default()
	if logger == nil {
		t.Error("Final Default() should not return nil")
	}
}

func TestDefaultLoggerCompareAndSwap(t *testing.T) {
	// Test that CompareAndSwap semantics work correctly
	cfg1 := DefaultConfig()
	logger1, err := New(cfg1)
	if err != nil {
		t.Fatalf("Failed to create logger1: %v", err)
	}
	defer logger1.Close()

	cfg2 := DefaultConfig()
	logger2, err := New(cfg2)
	if err != nil {
		t.Fatalf("Failed to create logger2: %v", err)
	}
	defer logger2.Close()

	// Set default to logger1
	SetDefault(logger1)

	// Verify Default() returns logger1
	if Default() != logger1 {
		t.Error("Default() should return logger1")
	}

	// Set default to logger2
	SetDefault(logger2)

	// Verify Default() returns logger2
	if Default() != logger2 {
		t.Error("Default() should return logger2")
	}
}

// ============================================================================
// CONFIG BUILD TESTS
// ============================================================================

func TestConfigBuild(t *testing.T) {
	// BasicBuild, WithFileOutput, WithJSONFormat, WithSecurityConfig, WithFatalHandler
	// are covered by: TestNewConfig, TestConfigIntegration (builder_test.go),
	// TestJSONLoggingPipeline, TestMessageSizeLimit, TestFatalLogging respectively.

	t.Run("WithMultipleOutputs", func(t *testing.T) {
		var buf1, buf2 bytes.Buffer
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf1), CustomOutput(&buf2)}
		cfg.Level = LevelInfo

		logger, _ := New(cfg)
		logger.Info("test")

		if buf1.Len() == 0 || buf2.Len() == 0 {
			t.Error("Message should be written to all outputs")
		}
	})
}

// TestConfigValidation was removed: builder_test.go TestConfigValidateErrors
// asserts the same invalid level/format values against their sentinels, and
// coverage_test.go boundary tables cover the New()/SetLevel() surfaces.

// ============================================================================
// IS LEVEL ENABLED TESTS
// ============================================================================

func TestIsLevelEnabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Level = LevelInfo
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	t.Run("IsLevelEnabled general method", func(t *testing.T) {
		// Debug is below Info, should be false
		if logger.IsLevelEnabled(LevelDebug) {
			t.Error("Debug should not be enabled when level is Info")
		}
		// Info is equal to Info, should be true
		if !logger.IsLevelEnabled(LevelInfo) {
			t.Error("Info should be enabled when level is Info")
		}
		// Warn is above Info, should be true
		if !logger.IsLevelEnabled(LevelWarn) {
			t.Error("Warn should be enabled when level is Info")
		}
		// Error is above Info, should be true
		if !logger.IsLevelEnabled(LevelError) {
			t.Error("Error should be enabled when level is Info")
		}
		// Fatal is above Info, should be true
		if !logger.IsLevelEnabled(LevelFatal) {
			t.Error("Fatal should be enabled when level is Info")
		}
	})

	t.Run("convenience methods", func(t *testing.T) {
		cases := []struct {
			level                           LogLevel
			debug, info, warn, lgErr, fatal bool
		}{
			{LevelInfo, false, true, true, true, true},
			{LevelDebug, true, true, true, true, true},
			{LevelError, false, false, false, true, true},
		}
		for _, tc := range cases {
			t.Run(tc.level.String(), func(t *testing.T) {
				logger.SetLevel(tc.level)
				if logger.IsDebugEnabled() != tc.debug {
					t.Errorf("IsDebugEnabled() at %s = %v, want %v", tc.level, !tc.debug, tc.debug)
				}
				if logger.IsInfoEnabled() != tc.info {
					t.Errorf("IsInfoEnabled() at %s = %v, want %v", tc.level, !tc.info, tc.info)
				}
				if logger.IsWarnEnabled() != tc.warn {
					t.Errorf("IsWarnEnabled() at %s = %v, want %v", tc.level, !tc.warn, tc.warn)
				}
				if logger.IsErrorEnabled() != tc.lgErr {
					t.Errorf("IsErrorEnabled() at %s = %v, want %v", tc.level, !tc.lgErr, tc.lgErr)
				}
				if logger.IsFatalEnabled() != tc.fatal {
					t.Errorf("IsFatalEnabled() at %s = %v, want %v", tc.level, !tc.fatal, tc.fatal)
				}
			})
		}
	})

	t.Run("thread safety", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				logger.IsLevelEnabled(LevelInfo)
			}()
			go func() {
				defer wg.Done()
				logger.SetLevel(LevelDebug)
			}()
		}
		wg.Wait()
	})
}

// ============================================================================
// NEW PACKAGE-LEVEL FUNCTIONS TESTS
// ============================================================================

func TestPackageLevelGenericLogFunctions(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelDebug
	logger, _ := New(cfg)
	SetDefault(logger)

	tests := []struct {
		name     string
		fn       func()
		expected string
	}{
		{
			name:     "Log",
			fn:       func() { Log(LevelInfo, "test log") },
			expected: "test log",
		},
		{
			name:     "Logf",
			fn:       func() { Logf(LevelInfo, "test %s", "logf") },
			expected: "test logf",
		},
		{
			name:     "LogWith",
			fn:       func() { LogWith(LevelInfo, "test logwith", String("key", "value")) },
			expected: "test logwith",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.fn()
			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("%s() output = %q, want to contain %q", tt.name, output, tt.expected)
			}
		})
	}
}

func TestPackageLevelIsEnabledFunctions(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	cfg := DefaultConfig()
	cfg.Level = LevelInfo
	logger, _ := New(cfg)
	SetDefault(logger)

	// Verify package-level functions delegate to the default logger.
	cases := []struct {
		level                           LogLevel
		debug, info, warn, lgErr, fatal bool
	}{
		{LevelInfo, false, true, true, true, true},
		{LevelError, false, false, false, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.level.String(), func(t *testing.T) {
			SetLevel(tc.level)
			if IsDebugEnabled() != tc.debug {
				t.Errorf("IsDebugEnabled() at %s = %v, want %v", tc.level, !tc.debug, tc.debug)
			}
			if IsInfoEnabled() != tc.info {
				t.Errorf("IsInfoEnabled() at %s = %v, want %v", tc.level, !tc.info, tc.info)
			}
			if IsWarnEnabled() != tc.warn {
				t.Errorf("IsWarnEnabled() at %s = %v, want %v", tc.level, !tc.warn, tc.warn)
			}
			if IsErrorEnabled() != tc.lgErr {
				t.Errorf("IsErrorEnabled() at %s = %v, want %v", tc.level, !tc.lgErr, tc.lgErr)
			}
			if IsFatalEnabled() != tc.fatal {
				t.Errorf("IsFatalEnabled() at %s = %v, want %v", tc.level, !tc.fatal, tc.fatal)
			}
		})
	}
}

func TestPackageLevelWithFields(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelDebug
	logger, _ := New(cfg)
	SetDefault(logger)

	t.Run("WithFields", func(t *testing.T) {
		buf.Reset()
		entry := WithFields(String("service", "api"), String("version", "1.0"))
		entry.Info("test message")
		output := buf.String()
		if !strings.Contains(output, "test message") {
			t.Error("WithFields entry should log message")
		}
		if !strings.Contains(output, "service") || !strings.Contains(output, "api") {
			t.Error("WithFields entry should contain fields")
		}
		if !strings.Contains(output, "version") || !strings.Contains(output, "1.0") {
			t.Error("WithFields entry should contain fields")
		}
	})

	t.Run("WithField", func(t *testing.T) {
		buf.Reset()
		entry := WithField("request_id", "abc123")
		entry.Info("test message")
		output := buf.String()
		if !strings.Contains(output, "test message") {
			t.Error("WithField entry should log message")
		}
		if !strings.Contains(output, "request_id") || !strings.Contains(output, "abc123") {
			t.Error("WithField entry should contain field")
		}
	})

	t.Run("Chained WithFields", func(t *testing.T) {
		buf.Reset()
		entry := WithFields(String("service", "api")).
			WithFields(String("version", "1.0")).
			WithField("request_id", "xyz789")
		entry.Info("chained message")
		output := buf.String()
		if !strings.Contains(output, "service") {
			t.Error("Chained entry should contain first field")
		}
		if !strings.Contains(output, "version") {
			t.Error("Chained entry should contain second field")
		}
		if !strings.Contains(output, "request_id") {
			t.Error("Chained entry should contain third field")
		}
	})
}

func TestPackageLevelFlush(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)
	SetDefault(logger)

	Info("test message")

	err := Flush()
	if err != nil {
		t.Errorf("Flush() returned error: %v", err)
	}
}

func TestPackageLevelWriterManagement(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	var buf1, buf2 bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf1)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)
	SetDefault(logger)

	initialCount := WriterCount()
	if initialCount < 1 {
		t.Errorf("Initial WriterCount() = %d, want at least 1", initialCount)
	}

	// Add writer
	err := AddWriter(&buf2)
	if err != nil {
		t.Errorf("AddWriter() returned error: %v", err)
	}

	newCount := WriterCount()
	if newCount != initialCount+1 {
		t.Errorf("WriterCount() after AddWriter = %d, want %d", newCount, initialCount+1)
	}

	// Log to both writers
	Info("test both writers")
	if buf1.String() == "" {
		t.Error("First writer should have output")
	}
	if buf2.String() == "" {
		t.Error("Second writer should have output")
	}

	// Remove writer
	err = RemoveWriter(&buf2)
	if err != nil {
		t.Errorf("RemoveWriter() returned error: %v", err)
	}

	finalCount := WriterCount()
	if finalCount != initialCount {
		t.Errorf("WriterCount() after RemoveWriter = %d, want %d", finalCount, initialCount)
	}
}

func TestPackageLevelSampling(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	cfg := DefaultConfig()
	cfg.Level = LevelInfo
	logger, _ := New(cfg)
	SetDefault(logger)

	// Test initial sampling is nil
	initialSampling := GetSampling()
	if initialSampling != nil {
		t.Error("Initial GetSampling() should be nil")
	}

	// Set sampling
	sampling := &SamplingConfig{
		Enabled:    true,
		Initial:    2,
		Thereafter: 5,
	}
	SetSampling(sampling)

	// Verify sampling was set
	newSampling := GetSampling()
	if newSampling == nil {
		t.Fatal("GetSampling() after SetSampling should not be nil")
	}
	if !newSampling.Enabled {
		t.Error("SamplingConfig.Enabled should be true")
	}
	if newSampling.Initial != 2 {
		t.Errorf("SamplingConfig.Initial = %d, want 2", newSampling.Initial)
	}
	if newSampling.Thereafter != 5 {
		t.Errorf("SamplingConfig.Thereafter = %d, want 5", newSampling.Thereafter)
	}
}

// ============================================================================
// ERROR TYPE TESTS (merged from errors_test.go)
// ============================================================================

func TestLoggerError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *LoggerError
		expected string
	}{
		{
			name: "error without cause",
			err: &LoggerError{
				Code:    errCodeInvalidLevel,
				Message: "level must be between 0 and 4",
			},
			expected: "[INVALID_LEVEL] level must be between 0 and 4",
		},
		{
			name: "error with cause",
			err: &LoggerError{
				Code:    errCodeConfigValidation,
				Message: "configuration validation failed",
				Cause:   errors.New("underlying error"),
			},
			expected: "[CONFIG_VALIDATION] configuration validation failed: underlying error",
		},
		{
			name: "error with context",
			err: &LoggerError{
				Code:    errCodeInvalidLevel,
				Message: "invalid level",
				Context: map[string]any{"level": 10},
			},
			expected: "[INVALID_LEVEL] invalid level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestLoggerError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &LoggerError{
		Code:    errCodeConfigValidation,
		Message: "validation failed",
		Cause:   cause,
	}

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}

	// Test nil cause
	errNoCause := &LoggerError{
		Code:    errCodeInvalidLevel,
		Message: "no cause",
	}
	if errNoCause.Unwrap() != nil {
		t.Errorf("Unwrap() for nil cause should return nil")
	}
}

func TestLoggerError_Is(t *testing.T) {
	tests := []struct {
		name        string
		err         *LoggerError
		target      error
		shouldMatch bool
	}{
		{
			name: "match ErrInvalidLevel",
			err: &LoggerError{
				Code:    errCodeInvalidLevel,
				Message: "invalid level provided",
			},
			target:      ErrInvalidLevel,
			shouldMatch: true,
		},
		{
			name: "match ErrLoggerClosed",
			err: &LoggerError{
				Code:    errCodeLoggerClosed,
				Message: "logger is closed",
			},
			target:      ErrLoggerClosed,
			shouldMatch: true,
		},
		{
			name: "no match different error",
			err: &LoggerError{
				Code:    errCodeInvalidLevel,
				Message: "invalid level",
			},
			target:      ErrLoggerClosed,
			shouldMatch: false,
		},
		{
			name: "no match non-sentinel error",
			err: &LoggerError{
				Code:    errCodeInvalidLevel,
				Message: "invalid level",
			},
			target:      errors.New("random error"),
			shouldMatch: false,
		},
		{
			name: "match ErrNilConfig",
			err: &LoggerError{
				Code:    errCodeNilConfig,
				Message: "config is nil",
			},
			target:      ErrNilConfig,
			shouldMatch: true,
		},
		{
			name: "match ErrNilWriter",
			err: &LoggerError{
				Code:    errCodeNilWriter,
				Message: "writer is nil",
			},
			target:      ErrNilWriter,
			shouldMatch: true,
		},
		{
			name: "match ErrPatternTooLong",
			err: &LoggerError{
				Code:    errCodePatternTooLong,
				Message: "pattern exceeds maximum length",
			},
			target:      ErrPatternTooLong,
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if errors.Is(tt.err, tt.target) != tt.shouldMatch {
				t.Errorf("errors.Is(%v, %v) should be %v", tt.err, tt.target, tt.shouldMatch)
			}
		})
	}
}

func TestNewLoggerError(t *testing.T) {
	err := newError(errCodeInvalidLevel, "invalid level value")

	if err.Code != errCodeInvalidLevel {
		t.Errorf("Code = %q, want %q", err.Code, errCodeInvalidLevel)
	}
	if err.Message != "invalid level value" {
		t.Errorf("Message = %q, want %q", err.Message, "invalid level value")
	}
	if err.Cause != nil {
		t.Errorf("Cause should be nil")
	}
	if err.Context != nil {
		t.Errorf("Context should be nil")
	}
}

func TestWrapError(t *testing.T) {
	cause := errors.New("underlying error")
	err := wrapError(errCodeConfigValidation, "validation failed", cause)

	if err.Code != errCodeConfigValidation {
		t.Errorf("Code = %q, want %q", err.Code, errCodeConfigValidation)
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}

	// Test nil cause returns nil
	nilErr := wrapError(errCodeInvalidLevel, "test", nil)
	if nilErr != nil {
		t.Errorf("WrapError with nil cause should return nil")
	}
}

func TestLoggerError_WithContext(t *testing.T) {
	err := &LoggerError{
		Code:    errCodeInvalidLevel,
		Message: "invalid level",
	}

	// Add context
	errWithContext := err.WithContext("level", 10)

	if errWithContext.Context == nil {
		t.Fatal("Context should not be nil")
	}
	if errWithContext.Context["level"] != 10 {
		t.Errorf("Context[level] = %v, want 10", errWithContext.Context["level"])
	}

	// Original error should not be modified
	if err.Context != nil {
		t.Errorf("Original error context should be nil")
	}

	// Test multiple context values
	errWithContext2 := errWithContext.WithContext("max", 4)
	if errWithContext2.Context["level"] != 10 {
		t.Errorf("Context[level] should still be 10")
	}
	if errWithContext2.Context["max"] != 4 {
		t.Errorf("Context[max] = %v, want 4", errWithContext2.Context["max"])
	}

	// Test nil receiver
	var nilErr *LoggerError
	if nilErr.WithContext("key", "value") != nil {
		t.Errorf("WithContext on nil should return nil")
	}
}

func TestErrorsIsWithWrappedError(t *testing.T) {
	// Test that errors.Is works through the chain
	cause := errors.New("root cause")
	wrapped := wrapError(errCodeConfigValidation, "config error", cause)

	// The wrapped error should match ErrConfigValidation sentinel (if we had one)
	// But we can test that Unwrap returns the cause
	unwrapped := errors.Unwrap(wrapped)
	if unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}
}

func TestErrorsAs(t *testing.T) {
	err := &LoggerError{
		Code:    errCodeInvalidLevel,
		Message: "invalid level provided",
		Context: map[string]any{
			"provided": 10,
			"max":      4,
		},
	}

	var loggerErr *LoggerError
	if !errors.As(err, &loggerErr) {
		t.Fatal("errors.As should return true")
	}

	if loggerErr.Code != errCodeInvalidLevel {
		t.Errorf("Code = %q, want %q", loggerErr.Code, errCodeInvalidLevel)
	}
	if loggerErr.Context["provided"] != 10 {
		t.Errorf("Context[provided] = %v, want 10", loggerErr.Context["provided"])
	}
}

// TestAllErrorCodesMatchSentinels was removed: boundary_test.go
// TestErrorIsAllSentinels is a superset (28 sentinels incl. ErrNilHook,
// ErrNilExtractor, ErrConfigValidation).

// ============================================================================
// EDGE CASE TESTS (merged from edge_cases_test.go)
// ============================================================================

func TestEmptyMessageWithFields(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	// Empty message with fields should still log
	logger.InfoWith("", String("key", "value"))

	output := buf.String()
	if !strings.Contains(output, "key=value") {
		t.Errorf("Expected 'key=value' in output, got: %s", output)
	}
}

func TestVeryLargeFieldCount(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	// Create 1000 fields
	fields := make([]Field, 1000)
	for i := 0; i < 1000; i++ {
		fields[i] = Int(fmt.Sprintf("field_%d", i), i)
	}

	logger.InfoWith("many fields", fields...)

	output := buf.String()
	if !strings.Contains(output, "many fields") {
		t.Error("Should contain message")
	}
	// Verify first and last fields are present
	if !strings.Contains(output, "field_0") {
		t.Error("Should contain first field")
	}
	if !strings.Contains(output, "field_999") {
		t.Error("Should contain last field")
	}
}

// ============================================================================
// CONCURRENCY EDGE CASE TESTS
// ============================================================================

func TestConcurrentAddRemoveWriter(t *testing.T) {
	logger, _ := New()
	const goroutines = 50
	const iterations = 20
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			buf := &threadSafeBuffer{Buffer: &bytes.Buffer{}}
			for j := 0; j < iterations; j++ {
				logger.AddWriter(buf)
				logger.Info("test message")
				logger.RemoveWriter(buf)
			}
		}(i)
	}
	wg.Wait()
}

// ============================================================================
// WRITER ERROR TESTS
// ============================================================================

// errorWriter is a writer that always returns an error
type errorWriter struct {
	err error
}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	return 0, w.err
}

// TestPartialWriteHandled pins the short-write contract: a writer that
// reports a short write with io.ErrShortWrite must surface that error to
// the write error handler. Failing-writer dispatch is covered by
// coverage_test.go TestWriterErrorWithHandler.
func TestPartialWriteHandled(t *testing.T) {
	var captured error
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&partialWriter{})}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	logger.SetWriteErrorHandler(func(w io.Writer, err error) { captured = err })
	logger.Info("test message that is longer than the buffer")

	if !errors.Is(captured, io.ErrShortWrite) {
		t.Errorf("write error handler captured %v, want io.ErrShortWrite", captured)
	}
}

// partialWriter writes only half the data
type partialWriter struct{}

// Write is io.Writer-conformant: a short write is reported with an error,
// as the io.Writer contract requires.
func (w *partialWriter) Write(p []byte) (n int, err error) {
	return len(p) / 2, io.ErrShortWrite
}

// ============================================================================
// SAMPLING EDGE CASE TESTS
// ============================================================================

// TestSamplingIntegration was removed: improvements_test.go TestSampling
// covers the same disabled case with exact counts and pins Initial-only,
// Thereafter, and runtime SetSampling behavior.

// ============================================================================
// NIL AND EMPTY VALUE TESTS
// ============================================================================

// TestEdgeShapedFields consolidates the edge-shaped-field cases that used to
// be four near-identical functions (nil value / empty key / no fields /
// 1000-char key): each row must log its message and any well-formed
// companion field without derailing the pipeline.
func TestEdgeShapedFields(t *testing.T) {
	cases := []struct {
		name   string
		msg    string
		fields []Field
		extra  string // additional substring expected in the output, "" for none
	}{
		{"nil field value", "nil test",
			[]Field{Any("nil_value", nil), String("string_value", "test")},
			"string_value=test"},
		{"empty field key", "empty key test",
			[]Field{String("", "value_with_empty_key"), String("valid_key", "valid_value")},
			"valid_key=valid_value"},
		{"no fields", "message", nil, ""},
		{"very long field name", "message",
			[]Field{String(strings.Repeat("a", 1000), "value")},
			""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := DefaultConfig()
			cfg.Targets = []OutputTarget{CustomOutput(&buf)}
			cfg.Level = LevelInfo
			logger, _ := New(cfg)
			defer logger.Close()

			logger.InfoWith(tc.msg, tc.fields...)

			output := buf.String()
			if !strings.Contains(output, tc.msg) {
				t.Errorf("output %q should contain message %q", output, tc.msg)
			}
			if tc.extra != "" && !strings.Contains(output, tc.extra) {
				t.Errorf("output %q should contain %q", output, tc.extra)
			}
		})
	}
}

// ============================================================================
// TIME FORMAT EDGE CASES
// ============================================================================

func TestTimeFormatConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		includeTime bool
		timeFormat  string
		expected    string
	}{
		{"CustomFormat", true, "2006-01-02", "-"},
		{"Disabled", false, "", "test message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := DefaultConfig()
			cfg.Targets = []OutputTarget{CustomOutput(&buf)}
			cfg.Level = LevelInfo
			cfg.IncludeTime = tt.includeTime
			if tt.timeFormat != "" {
				cfg.TimeFormat = tt.timeFormat
			}
			logger, _ := New(cfg)

			logger.Info("test message")

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("%s output should contain %q, got: %s", tt.name, tt.expected, output)
			}
		})
	}
}

// ============================================================================
// LOGGER CLOSE EDGE CASES
// ============================================================================
