package dd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
)

// ============================================================================
// CORE LOGGER TESTS
// ============================================================================

func TestLoggerCreation(t *testing.T) {
	tests := []struct {
		name    string
		config  *LoggerConfig
		wantErr bool
	}{
		{
			name:    "default config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: false, // Should use default
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

func TestBasicLogging(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "INFO") {
		t.Errorf("Expected output to contain 'INFO', got: %s", output)
	}
}

func TestLogLevels(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Level = LevelWarn
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()

	// Debug and Info should be filtered out
	if strings.Contains(output, "debug message") {
		t.Errorf("Debug message should be filtered out")
	}
	if strings.Contains(output, "info message") {
		t.Errorf("Info message should be filtered out")
	}

	// Warn and Error should be present
	if !strings.Contains(output, "warn message") {
		t.Errorf("Warn message should be present")
	}
	if !strings.Contains(output, "error message") {
		t.Errorf("Error message should be present")
	}
}

func TestStructuredLogging(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.InfoWith("test message", String("key", "value"), Int("number", 42))

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Expected output to contain 'key=value', got: %s", output)
	}
	if !strings.Contains(output, "number=42") {
		t.Errorf("Expected output to contain 'number=42', got: %s", output)
	}
}

func TestJSONLogging(t *testing.T) {
	var buf bytes.Buffer
	config := JSONConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, `"message":"test message"`) {
		t.Errorf("Expected JSON output to contain message field, got: %s", output)
	}
	if !strings.Contains(output, `"level":"INFO"`) {
		t.Errorf("Expected JSON output to contain level field, got: %s", output)
	}
}

func TestFormattedLogging(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Infof("User %s has %d items", "john", 42)

	output := buf.String()
	if !strings.Contains(output, "User john has 42 items") {
		t.Errorf("Expected formatted output, got: %s", output)
	}
}

// ============================================================================
// LOGGER STATE MANAGEMENT TESTS
// ============================================================================

func TestLoggerLevelManagement(t *testing.T) {
	logger, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test initial level
	if logger.GetLevel() != LevelInfo {
		t.Errorf("Expected initial level Info, got %v", logger.GetLevel())
	}

	// Test level change
	err = logger.SetLevel(LevelWarn)
	if err != nil {
		t.Errorf("SetLevel failed: %v", err)
	}

	if logger.GetLevel() != LevelWarn {
		t.Errorf("Expected level Warn, got %v", logger.GetLevel())
	}

	// Test invalid level
	err = logger.SetLevel(LogLevel(99))
	if err == nil {
		t.Error("Expected error for invalid level")
	}
}

func TestLoggerWriterManagement(t *testing.T) {
	logger, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	var buf bytes.Buffer

	// Test add writer
	err = logger.AddWriter(&buf)
	if err != nil {
		t.Errorf("AddWriter failed: %v", err)
	}

	logger.Info("test")
	if buf.Len() == 0 {
		t.Error("Writer should have received message")
	}

	// Test remove writer
	err = logger.RemoveWriter(&buf)
	if err != nil {
		t.Errorf("RemoveWriter failed: %v", err)
	}

	buf.Reset()
	logger.Info("test2")
	if buf.Len() > 0 {
		t.Error("Removed writer should not receive messages")
	}

	// Test nil writer
	err = logger.AddWriter(nil)
	if err == nil {
		t.Error("Expected error for nil writer")
	}
}

func TestLoggerClose(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.Info("before close")
	initialLen := buf.Len()

	err = logger.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	logger.Info("after close")
	if buf.Len() != initialLen {
		t.Error("Should not log after close")
	}

	// Test operations after close
	err = logger.AddWriter(&bytes.Buffer{})
	if err == nil {
		t.Error("Should return error when adding writer after close")
	}

	// Multiple closes should not panic
	logger.Close()
	logger.Close()
}

// ============================================================================
// CONVENIENCE FUNCTIONS TESTS
// ============================================================================

func TestConvenienceFunctions(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Store original default
	originalDefault := Default()
	defer SetDefault(originalDefault)

	// Set test logger as default
	SetDefault(logger)

	Info("convenience test")

	output := buf.String()
	if !strings.Contains(output, "convenience test") {
		t.Errorf("Expected output to contain 'convenience test', got: %s", output)
	}
}

func TestGlobalLevelSetting(t *testing.T) {
	originalDefault := Default()
	defer SetDefault(originalDefault)

	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	SetDefault(logger)
	SetLevel(LevelWarn)

	Debug("debug message")
	Info("info message")
	Warn("warn message")

	output := buf.String()
	if strings.Contains(output, "debug message") || strings.Contains(output, "info message") {
		t.Error("Debug and Info messages should be filtered")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("Warn message should be present")
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestConcurrentLogging(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	var wg sync.WaitGroup
	numGoroutines := 100
	messagesPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				logger.Infof("goroutine %d message %d", id, j)
			}
		}(i)
	}

	wg.Wait()
}

func TestConcurrentWriterOperations(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}
	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	var wg sync.WaitGroup

	// Concurrent add/remove writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.AddWriter(io.Discard)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.RemoveWriter(io.Discard)
		}()
	}

	// Concurrent logging
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("test message")
		}()
	}

	wg.Wait()
}

func TestConcurrentLevelChanges(t *testing.T) {
	config := DefaultConfig()
	config.Writers = []io.Writer{io.Discard}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	var wg sync.WaitGroup

	// Rapidly change levels
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			logger.SetLevel(LogLevel(i % 5))
		}
	}()

	// Log while levels are changing
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			logger.Info("test message")
		}
	}()

	wg.Wait()
}

// ============================================================================
// EDGE CASES AND ERROR CONDITIONS
// ============================================================================

func TestEdgeCases(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	t.Run("empty_message", func(t *testing.T) {
		buf.Reset()
		logger.Info("")
		if buf.Len() == 0 {
			t.Error("Should log empty message")
		}
	})

	t.Run("nil_fields", func(t *testing.T) {
		buf.Reset()
		logger.InfoWith("test", Any("key", nil))
		if buf.Len() == 0 {
			t.Error("Should log message with nil field")
		}
	})

	t.Run("special_characters", func(t *testing.T) {
		buf.Reset()
		logger.Info("test\nmessage\rwith\tspecial\x00chars")
		output := buf.String()
		if strings.Contains(output, "\x00") {
			t.Error("Should sanitize null bytes")
		}
	})

	t.Run("unicode_message", func(t *testing.T) {
		buf.Reset()
		logger.Info("测试消息 🚀 тест")
		if buf.Len() == 0 {
			t.Error("Should log unicode message")
		}
	})

	t.Run("very_long_field_key", func(t *testing.T) {
		buf.Reset()
		longKey := strings.Repeat("a", 1000)
		logger.InfoWith("test", String(longKey, "value"))
		if buf.Len() == 0 {
			t.Error("Should log message with long field key")
		}
	})
}

func TestLargeMessages(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}
	config.SecurityConfig = &SecurityConfig{
		MaxMessageSize: 10 * 1024 * 1024, // 10MB
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Create 1MB message
	largeMsg := strings.Repeat("A", 1024*1024)
	logger.Info(largeMsg)

	if buf.Len() == 0 {
		t.Error("Logger should handle large messages")
	}
}

func TestManyFields(t *testing.T) {
	var buf bytes.Buffer
	config := JSONConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	fields := make([]Field, 100)
	for i := 0; i < 100; i++ {
		fields[i] = Int("field"+string(rune(i)), i)
	}

	logger.InfoWith("test message", fields...)

	if buf.Len() == 0 {
		t.Error("Logger should handle many fields")
	}
}

// ============================================================================
// SECURITY TESTS
// ============================================================================

func TestSecurityConfig(t *testing.T) {
	config := DefaultConfig()
	config.SecurityConfig = &SecurityConfig{
		MaxMessageSize:  MaxMessageSize,
		MaxWriters:      MaxWriterCount,
		SensitiveFilter: nil,
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	secConfig := logger.GetSecurityConfig()
	if secConfig == nil {
		t.Error("Security config should not be nil")
	}

	// Test setting security config
	newConfig := &SecurityConfig{
		MaxMessageSize: 2048,
		MaxWriters:     50,
	}
	logger.SetSecurityConfig(newConfig)

	updatedConfig := logger.GetSecurityConfig()
	if updatedConfig.MaxMessageSize != 2048 {
		t.Error("Security config should be updated")
	}
}

func TestBasicFiltering(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}
	config.SecurityConfig = &SecurityConfig{
		SensitiveFilter: NewBasicSensitiveDataFilter(),
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.Info("password=secret123")

	output := buf.String()
	if !strings.Contains(output, "[REDACTED]") {
		t.Errorf("Password should be filtered, got: %s", output)
	}
	if strings.Contains(output, "secret123") {
		t.Errorf("Password value should not appear in output, got: %s", output)
	}
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// failingWriter is a writer that fails after N writes
type failingWriter struct {
	failAfter int
	count     int
	mu        sync.Mutex
}

func (fw *failingWriter) Write(p []byte) (int, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	fw.count++
	if fw.count > fw.failAfter {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}
