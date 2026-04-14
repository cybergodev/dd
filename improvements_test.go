package dd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestFatalTimeout tests that Fatal logs exit even when Close() blocks
// This test verifies:
// 1. handleFatal completes within the expected timeout
// 2. The FatalHandler is called even when Close() blocks
// Note: Testing stderr output is skipped under race detector due to
// inherent race in capturing os.Stderr from concurrent goroutines.
func TestFatalTimeout(t *testing.T) {
	t.Run("handleFatal completes and calls handler when close blocks", func(t *testing.T) {
		exited := false
		var mu sync.Mutex

		cfg := DefaultConfig()
		cfg.FatalHandler = func() {
			mu.Lock()
			exited = true
			mu.Unlock()
		}
		logger, _ := New(cfg)

		// Replace writers with an interruptible blocking writer
		blockingWriter := newInterruptibleBlockingWriter()
		logger.writersPtr.Store(&[]io.Writer{blockingWriter})

		// Channel to signal when handleFatal completes
		handleFatalDone := make(chan struct{})

		// Call handleFatal in a goroutine
		go func() {
			defer close(handleFatalDone)
			logger.handleFatal()
		}()

		// Wait for handleFatal to complete with a timeout
		// The DefaultFatalFlushTimeout is 5 seconds, so we wait up to 10 seconds
		select {
		case <-handleFatalDone:
			// Good, completed
		case <-time.After(10 * time.Second):
			blockingWriter.cancel()
			t.Fatal("handleFatal should have completed within timeout")
		}

		mu.Lock()
		if !exited {
			t.Error("FatalHandler should have been called")
		}
		mu.Unlock()

		// Cancel the blocking writer to allow the Close() goroutine to complete
		// This ensures test isolation and prevents race conditions with subsequent tests
		blockingWriter.cancel()
	})
}

// interruptibleBlockingWriter is a writer that blocks on Write and Close but can be interrupted
type interruptibleBlockingWriter struct {
	cancelFunc context.CancelFunc
	ctx        context.Context
}

func newInterruptibleBlockingWriter() *interruptibleBlockingWriter {
	ctx, cancel := context.WithCancel(context.Background())
	return &interruptibleBlockingWriter{cancelFunc: cancel, ctx: ctx}
}

func (w *interruptibleBlockingWriter) Write(p []byte) (n int, err error) {
	select {
	case <-time.After(10 * time.Second):
		return len(p), nil
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	}
}

func (w *interruptibleBlockingWriter) Close() error {
	select {
	case <-time.After(10 * time.Second):
		return nil
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func (w *interruptibleBlockingWriter) cancel() {
	if w.cancelFunc != nil {
		w.cancelFunc()
	}
}

// TestErrorHandling tests the unified error handling for hooks and extractors
func TestErrorHandling(t *testing.T) {
	t.Run("AddHook returns error for nil hook", func(t *testing.T) {
		logger, _ := New()
		err := logger.AddHook(HookBeforeLog, nil)
		if !errors.Is(err, ErrNilHook) {
			t.Errorf("Expected ErrNilHook, got: %v", err)
		}
	})

	t.Run("AddHook returns error when logger closed", func(t *testing.T) {
		logger, _ := New()
		logger.Close()
		err := logger.AddHook(HookBeforeLog, func(ctx context.Context, h *HookContext) error {
			return nil
		})
		if !errors.Is(err, ErrLoggerClosed) {
			t.Errorf("Expected ErrLoggerClosed, got: %v", err)
		}
	})

	t.Run("AddContextExtractor returns error for nil extractor", func(t *testing.T) {
		logger, _ := New()
		err := logger.AddContextExtractor(nil)
		if !errors.Is(err, ErrNilExtractor) {
			t.Errorf("Expected ErrNilExtractor, got: %v", err)
		}
	})

	t.Run("AddContextExtractor returns error when logger closed", func(t *testing.T) {
		logger, _ := New()
		logger.Close()
		err := logger.AddContextExtractor(func(ctx context.Context) []Field {
			return nil
		})
		if !errors.Is(err, ErrLoggerClosed) {
			t.Errorf("Expected ErrLoggerClosed, got: %v", err)
		}
	})

	t.Run("SetHooks returns error when logger closed", func(t *testing.T) {
		logger, _ := New()
		logger.Close()
		registry := NewHookRegistry()
		err := logger.SetHooks(registry)
		if !errors.Is(err, ErrLoggerClosed) {
			t.Errorf("Expected ErrLoggerClosed, got: %v", err)
		}
	})

	t.Run("SetContextExtractors returns error when logger closed", func(t *testing.T) {
		logger, _ := New()
		logger.Close()
		err := logger.SetContextExtractors(func(ctx context.Context) []Field {
			return nil
		})
		if !errors.Is(err, ErrLoggerClosed) {
			t.Errorf("Expected ErrLoggerClosed, got: %v", err)
		}
	})
}

// TestDefaultLoggerWarning tests that Default() returns a usable logger
func TestDefaultLoggerWarning(t *testing.T) {
	// The default logger should work without error
	logger := Default()
	if logger == nil {
		t.Fatal("Default() should return a logger")
	}

	// Verify the logger is functional by checking it can log
	// (without actually writing to stdout)
	var buf bytes.Buffer
	testLogger, err := New(NewTestConfigWithBuffer(&buf))
	if err != nil {
		t.Fatalf("Failed to create test logger: %v", err)
	}
	testLogger.Info("test message")

	if !strings.Contains(buf.String(), "test message") {
		t.Error("Test logger should log messages")
	}

	// Check if we can get the init error (should be nil for normal operation)
	initErr := DefaultInitError()
	// The error should be nil in normal operation
	if initErr != nil {
		t.Logf("DefaultInitError returned: %v (may be expected in some environments)", initErr)
	}

	// Verify the default logger has essential methods
	t.Run("Default logger has required methods", func(t *testing.T) {
		// These should not panic
		_ = logger.IsClosed()
		_ = logger.GetLevel()
	})
}

// TestWithFields tests the WithFields context propagation
func TestWithFields(t *testing.T) {
	t.Run("WithFields creates entry with fields", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Format = FormatJSON
		logger, _ := New(cfg)

		entry := logger.WithFields(String("service", "api"))
		entry.Info("test message")

		output := buf.String()
		if !strings.Contains(output, `"service":"api"`) {
			t.Errorf("Expected field 'service:api' in output, got: %s", output)
		}
		if !strings.Contains(output, `"message":"test message"`) {
			t.Errorf("Expected message in output, got: %s", output)
		}
	})

	t.Run("WithField creates entry with single field", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Format = FormatJSON
		logger, _ := New(cfg)

		entry := logger.WithField("request_id", "abc123")
		entry.Info("test message")

		output := buf.String()
		if !strings.Contains(output, `"request_id":"abc123"`) {
			t.Errorf("Expected field 'request_id:abc123' in output, got: %s", output)
		}
	})

	t.Run("nested WithFields merges fields", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Format = FormatJSON
		logger, _ := New(cfg)

		entry := logger.WithFields(String("service", "api"), String("version", "1.0"))
		entry2 := entry.WithFields(String("user", "john"))
		entry2.Info("test message")

		output := buf.String()
		if !strings.Contains(output, `"service":"api"`) {
			t.Errorf("Expected field 'service:api' in output, got: %s", output)
		}
		if !strings.Contains(output, `"version":"1.0"`) {
			t.Errorf("Expected field 'version:1.0' in output, got: %s", output)
		}
		if !strings.Contains(output, `"user":"john"`) {
			t.Errorf("Expected field 'user:john' in output, got: %s", output)
		}
	})

	t.Run("WithFields override", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Format = FormatJSON
		logger, _ := New(cfg)

		entry := logger.WithFields(String("service", "api"))
		entry2 := entry.WithFields(String("service", "web")) // Override
		entry2.Info("test message")

		output := buf.String()
		// Should have the overridden value
		if !strings.Contains(output, `"service":"web"`) {
			t.Errorf("Expected field 'service:web' in output, got: %s", output)
		}
		// Count occurrences - should only appear once
		count := strings.Count(output, `"service"`)
		if count != 1 {
			t.Errorf("Expected 'service' to appear once, got %d times", count)
		}
	})

	t.Run("entry immutability", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Format = FormatJSON
		logger, _ := New(cfg)

		entry1 := logger.WithFields(String("service", "api"))
		entry2 := entry1.WithFields(String("user", "john"))

		// Clear buffer
		buf.Reset()

		// entry1 should still only have service field
		entry1.Info("message1")
		output1 := buf.String()
		if strings.Contains(output1, `"user"`) {
			t.Errorf("entry1 should not have user field, got: %s", output1)
		}

		// Clear buffer
		buf.Reset()

		// entry2 should have both fields
		entry2.Info("message2")
		output2 := buf.String()
		if !strings.Contains(output2, `"service"`) || !strings.Contains(output2, `"user"`) {
			t.Errorf("entry2 should have both fields, got: %s", output2)
		}
	})

	t.Run("entry WithField method", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Format = FormatJSON
		logger, _ := New(cfg)

		entry := logger.WithField("service", "api")
		entry2 := entry.WithField("user", "john")
		entry2.Info("test message")

		output := buf.String()
		if !strings.Contains(output, `"service":"api"`) {
			t.Errorf("Expected field 'service:api' in output, got: %s", output)
		}
		if !strings.Contains(output, `"user":"john"`) {
			t.Errorf("Expected field 'user:john' in output, got: %s", output)
		}
	})

	t.Run("entry LogWith merges fields", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Format = FormatJSON
		logger, _ := New(cfg)

		entry := logger.WithFields(String("service", "api"))
		entry.InfoWith("test message", String("extra", "value"))

		output := buf.String()
		if !strings.Contains(output, `"service":"api"`) {
			t.Errorf("Expected field 'service:api' in output, got: %s", output)
		}
		if !strings.Contains(output, `"extra":"value"`) {
			t.Errorf("Expected field 'extra:value' in output, got: %s", output)
		}
	})
}

// TestSampling tests the log sampling functionality
func TestSampling(t *testing.T) {
	t.Run("disabled sampling logs everything", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		logger, _ := New(cfg)

		for i := 0; i < 100; i++ {
			logger.Info("test")
		}

		lines := strings.Count(buf.String(), "\n")
		if lines != 100 {
			t.Errorf("Expected 100 lines, got %d", lines)
		}
	})

	t.Run("sampling with Initial only", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Sampling = &SamplingConfig{Enabled: true, Initial: 10, Thereafter: 0}
		logger, _ := New(cfg)

		for i := 0; i < 100; i++ {
			logger.Info("test")
		}

		lines := strings.Count(buf.String(), "\n")
		if lines != 10 {
			t.Errorf("Expected 10 lines (Initial only), got %d", lines)
		}
	})

	t.Run("sampling with Thereafter", func(t *testing.T) {
		var buf bytes.Buffer
		// Initial=5, Thereafter=5 means: first 5 always, then 1 out of every 5
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Sampling = &SamplingConfig{Enabled: true, Initial: 5, Thereafter: 5}
		logger, _ := New(cfg)

		for i := 0; i < 25; i++ {
			logger.Info("test")
		}

		lines := strings.Count(buf.String(), "\n")
		// Expected: 5 (initial) + 4 (20/5 thereafter) = 9
		if lines != 9 {
			t.Errorf("Expected 9 lines (5 initial + 4 thereafter), got %d", lines)
		}
	})

	t.Run("SetSampling at runtime", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		logger, _ := New(cfg)

		// No sampling initially
		for i := 0; i < 10; i++ {
			logger.Info("test")
		}
		if lines := strings.Count(buf.String(), "\n"); lines != 10 {
			t.Errorf("Expected 10 lines before sampling, got %d", lines)
		}

		buf.Reset()

		// Enable sampling
		logger.SetSampling(&SamplingConfig{
			Enabled:    true,
			Initial:    2,
			Thereafter: 2,
		})

		for i := 0; i < 10; i++ {
			logger.Info("test")
		}

		lines := strings.Count(buf.String(), "\n")
		// Expected: 2 (initial) + 4 (8/2 thereafter) = 6
		if lines != 6 {
			t.Errorf("Expected 6 lines with sampling, got %d", lines)
		}
	})

	t.Run("GetSampling returns config", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Sampling = &SamplingConfig{Enabled: true, Initial: 10, Thereafter: 5, Tick: time.Minute}
		logger, _ := New(cfg)

		config := logger.GetSampling()
		if config == nil {
			t.Fatal("Expected sampling config, got nil")
		}
		if config.Initial != 10 {
			t.Errorf("Expected Initial=10, got %d", config.Initial)
		}
		if config.Thereafter != 5 {
			t.Errorf("Expected Thereafter=5, got %d", config.Thereafter)
		}
		if config.Tick != time.Minute {
			t.Errorf("Expected Tick=1m, got %v", config.Tick)
		}
	})

	t.Run("SetSampling nil disables sampling", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Sampling = &SamplingConfig{Enabled: true, Initial: 1, Thereafter: 100}
		logger, _ := New(cfg)

		logger.SetSampling(nil)

		for i := 0; i < 10; i++ {
			logger.Info("test")
		}

		lines := strings.Count(buf.String(), "\n")
		if lines != 10 {
			t.Errorf("Expected 10 lines after disabling sampling, got %d", lines)
		}
	})
}

// TestHookContextOriginalFields tests that OriginalFields are passed to hooks
func TestHookContextOriginalFields(t *testing.T) {
	t.Run("OriginalFields contains unfiltered fields", func(t *testing.T) {
		var buf bytes.Buffer
		var capturedOriginal []Field
		var capturedFiltered []Field

		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Security = DefaultSecurityConfig()
		cfg.Hooks = NewHookRegistry()
		cfg.Hooks.Add(HookBeforeLog, func(ctx context.Context, h *HookContext) error {
			capturedOriginal = h.OriginalFields
			capturedFiltered = h.Fields
			return nil
		})

		logger, _ := New(cfg)

		// Log with a field that might be filtered
		logger.InfoWith("test", String("password", "secret123"), String("name", "john"))

		if len(capturedOriginal) != 2 {
			t.Errorf("Expected 2 original fields, got %d", len(capturedOriginal))
		}
		if len(capturedFiltered) != 2 {
			t.Errorf("Expected 2 filtered fields, got %d", len(capturedFiltered))
		}

		// Find the password field in original
		var originalPassword, filteredPassword string
		for _, f := range capturedOriginal {
			if f.Key == "password" {
				originalPassword = fmt.Sprintf("%v", f.Value)
			}
		}
		for _, f := range capturedFiltered {
			if f.Key == "password" {
				filteredPassword = fmt.Sprintf("%v", f.Value)
			}
		}

		// Original should have the actual value, filtered might be redacted
		if originalPassword != "secret123" {
			t.Errorf("Expected original password 'secret123', got '%s'", originalPassword)
		}

		// Filtered should be redacted (since password is a sensitive key)
		if filteredPassword == "secret123" {
			t.Error("Expected filtered password to be redacted, got original value")
		}
	})
}

// TestFilterPerformanceMetrics tests the filter performance monitoring
func TestFilterPerformanceMetrics(t *testing.T) {
	t.Run("GetFilterStats returns metrics", func(t *testing.T) {
		filter := newBasicSensitiveDataFilter()

		// Process some inputs
		filter.Filter("hello world")
		filter.Filter("password=secret123")
		filter.Filter("normal text")

		stats := filter.GetFilterStats()

		if stats.TotalFiltered != 3 {
			t.Errorf("Expected TotalFiltered=3, got %d", stats.TotalFiltered)
		}

		if stats.PatternCount == 0 {
			t.Error("Expected PatternCount > 0")
		}

		if !stats.Enabled {
			t.Error("Expected Enabled=true")
		}
	})

	t.Run("redactions are tracked", func(t *testing.T) {
		filter := newBasicSensitiveDataFilter()

		// Log something with a password pattern that should be redacted
		filter.Filter("password=test123")
		filter.Filter("normal text without sensitive data")

		stats := filter.GetFilterStats()

		if stats.TotalFiltered != 2 {
			t.Errorf("Expected TotalFiltered=2, got %d", stats.TotalFiltered)
		}

		// At least one redaction should have occurred
		if stats.TotalRedactions < 1 {
			t.Errorf("Expected TotalRedactions >= 1, got %d", stats.TotalRedactions)
		}
	})

	t.Run("average latency is calculated", func(t *testing.T) {
		filter := newBasicSensitiveDataFilter()

		// Process many inputs to get meaningful average
		for i := 0; i < 100; i++ {
			filter.Filter("test message with some content")
		}

		stats := filter.GetFilterStats()

		if stats.TotalFiltered != 100 {
			t.Errorf("Expected TotalFiltered=100, got %d", stats.TotalFiltered)
		}

		if stats.AverageLatency == 0 {
			t.Error("Expected AverageLatency > 0")
		}
	})
}

// TestLevelResolver tests the dynamic level resolver functionality
func TestLevelResolver(t *testing.T) {
	t.Run("SetLevelResolver stores resolver", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Level = LevelDebug
		logger, _ := New(cfg)

		resolver := func(ctx context.Context) LogLevel {
			return LevelWarn
		}
		logger.SetLevelResolver(resolver)

		if logger.GetLevelResolver() == nil {
			t.Error("Expected resolver to be set")
		}
	})

	t.Run("resolver affects logging without context", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Level = LevelDebug
		logger, _ := New(cfg)

		// Set resolver that only allows Warn and above
		logger.SetLevelResolver(func(ctx context.Context) LogLevel {
			return LevelWarn
		})

		logger.Debug("debug message") // Should be filtered
		logger.Info("info message")   // Should be filtered
		logger.Warn("warn message")   // Should appear
		logger.Error("error message") // Should appear

		output := buf.String()
		if strings.Contains(output, "debug message") {
			t.Error("Debug should be filtered by resolver")
		}
		if strings.Contains(output, "info message") {
			t.Error("Info should be filtered by resolver")
		}
		if !strings.Contains(output, "warn message") {
			t.Error("Warn should appear")
		}
		if !strings.Contains(output, "error message") {
			t.Error("Error should appear")
		}
	})

	t.Run("nil resolver uses static level", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Level = LevelInfo
		logger, _ := New(cfg)

		// Set resolver then clear it
		logger.SetLevelResolver(func(ctx context.Context) LogLevel {
			return LevelWarn
		})
		logger.SetLevelResolver(nil)

		logger.Debug("debug message") // Should be filtered (static level is Info)
		logger.Info("info message")   // Should appear

		output := buf.String()
		if strings.Contains(output, "debug message") {
			t.Error("Debug should be filtered by static level")
		}
		if !strings.Contains(output, "info message") {
			t.Error("Info should appear")
		}
	})

	t.Run("dynamic level changes at runtime", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Level = LevelDebug
		logger, _ := New(cfg)

		highLoad := false

		logger.SetLevelResolver(func(ctx context.Context) LogLevel {
			if highLoad {
				return LevelWarn
			}
			return LevelDebug
		})

		// Normal load - debug should work
		logger.Debug("debug 1")
		if !strings.Contains(buf.String(), "debug 1") {
			t.Error("Debug should appear under normal load")
		}

		buf.Reset()

		// Simulate high load
		highLoad = true
		logger.Debug("debug 2") // Should be filtered
		logger.Warn("warn 1")   // Should appear

		output := buf.String()
		if strings.Contains(output, "debug 2") {
			t.Error("Debug should be filtered under high load")
		}
		if !strings.Contains(output, "warn 1") {
			t.Error("Warn should appear under high load")
		}
	})

}

// TestFieldValidation tests the field key validation functionality
func TestFieldValidation(t *testing.T) {
	t.Run("naming convention validators", func(t *testing.T) {
		tests := []struct {
			convention FieldNamingConvention
			valid      []string
			invalid    []string
		}{
			{
				convention: NamingConventionSnakeCase,
				valid:      []string{"user_id", "first_name", "created_at", "id", "url"},
				invalid:    []string{"userId", "UserID", "user-id", "user__id", "_user_id", "user_id_"},
			},
			{
				convention: NamingConventionCamelCase,
				valid:      []string{"userId", "firstName", "createdAt", "id", "url"},
				invalid:    []string{"user_id", "UserID", "user-id", "user__id"},
			},
			{
				convention: NamingConventionPascalCase,
				valid:      []string{"UserId", "FirstName", "CreatedAt", "ID", "URL"},
				invalid:    []string{"user_id", "userId", "user-id", "user__id"},
			},
			{
				convention: NamingConventionKebabCase,
				valid:      []string{"user-id", "first-name", "created-at", "id", "url"},
				invalid:    []string{"userId", "user_id", "UserID", "user--id", "-user-id", "user-id-"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.convention.String(), func(t *testing.T) {
				config := &FieldValidationConfig{
					Mode:                     FieldValidationWarn,
					Convention:               tt.convention,
					AllowCommonAbbreviations: true,
				}

				for _, key := range tt.valid {
					err := config.ValidateFieldKey(key)
					if err != nil {
						t.Errorf("Expected %q to be valid for %s, got error: %v", key, tt.convention, err)
					}
				}

				for _, key := range tt.invalid {
					// Skip common abbreviations
					if isCommonAbbreviation(key) {
						continue
					}
					err := config.ValidateFieldKey(key)
					if err == nil {
						t.Errorf("Expected %q to be invalid for %s", key, tt.convention)
					}
				}
			})
		}
	})

	t.Run("empty key is always invalid", func(t *testing.T) {
		config := &FieldValidationConfig{
			Mode:       FieldValidationWarn,
			Convention: NamingConventionSnakeCase,
		}
		err := config.ValidateFieldKey("")
		if err == nil {
			t.Error("Expected empty key to be invalid")
		}
	})

	t.Run("none mode skips validation", func(t *testing.T) {
		config := &FieldValidationConfig{
			Mode:       FieldValidationNone,
			Convention: NamingConventionSnakeCase,
		}
		// Any key should be valid when validation is disabled
		for _, key := range []string{"userId", "user_id", "user-id", "UserID"} {
			err := config.ValidateFieldKey(key)
			if err != nil {
				t.Errorf("Expected %q to be valid when validation is disabled", key)
			}
		}
	})

	t.Run("any convention accepts all keys", func(t *testing.T) {
		config := &FieldValidationConfig{
			Mode:       FieldValidationWarn,
			Convention: NamingConventionAny,
		}
		for _, key := range []string{"userId", "user_id", "user-id", "UserID"} {
			err := config.ValidateFieldKey(key)
			if err != nil {
				t.Errorf("Expected %q to be valid with any convention", key)
			}
		}
	})

	t.Run("common abbreviations allowed", func(t *testing.T) {
		config := &FieldValidationConfig{
			Mode:                     FieldValidationStrict,
			Convention:               NamingConventionSnakeCase,
			AllowCommonAbbreviations: true,
		}
		for _, key := range []string{"id", "ID", "url", "URL", "api", "API", "user_id", "request_url"} {
			err := config.ValidateFieldKey(key)
			if err != nil {
				t.Errorf("Expected common abbreviation %q to be valid", key)
			}
		}
	})

	t.Run("validation with logger", func(t *testing.T) {
		// Capture stderr
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Format = FormatJSON
		cfg.FieldValidation = &FieldValidationConfig{
			Mode:                     FieldValidationWarn,
			Convention:               NamingConventionSnakeCase,
			AllowCommonAbbreviations: true,
		}
		logger, _ := New(cfg)

		// Valid key - should not produce warning
		logger.InfoWith("test", String("user_id", "123"))

		// Invalid key - should produce warning on stderr
		logger.InfoWith("test", String("userId", "123"))

		w.Close()
		os.Stderr = oldStderr

		var stderrBuf bytes.Buffer
		io.Copy(&stderrBuf, r)
		stderrOutput := stderrBuf.String()

		// Should have warning for invalid key
		if !strings.Contains(stderrOutput, "userId") {
			t.Errorf("Expected warning for 'userId' in stderr, got: %s", stderrOutput)
		}

		// Log output should still contain both fields
		output := buf.String()
		if !strings.Contains(output, "user_id") {
			t.Error("Expected user_id in output")
		}
		if !strings.Contains(output, "userId") {
			t.Error("Expected userId in output (validation doesn't block logging)")
		}
	})

	t.Run("SetFieldValidation at runtime", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		logger, _ := New(cfg)

		// No validation initially
		logger.InfoWith("test", String("userId", "123"))

		// Enable validation
		logger.SetFieldValidation(&FieldValidationConfig{
			Mode:       FieldValidationWarn,
			Convention: NamingConventionSnakeCase,
		})

		if logger.GetFieldValidation() == nil {
			t.Error("Expected field validation to be set")
		}

		// Clear validation
		logger.SetFieldValidation(nil)
		if logger.GetFieldValidation() != nil {
			t.Error("Expected field validation to be nil after clearing")
		}
	})
}

// ============================================================================
// FIELD VALIDATION MODE STRING TESTS (merged from field_validation_test.go)
// ============================================================================

func TestFieldValidationMode_String(t *testing.T) {
	tests := []struct {
		mode     FieldValidationMode
		expected string
	}{
		{FieldValidationNone, "none"},
		{FieldValidationWarn, "warn"},
		{FieldValidationStrict, "strict"},
		{FieldValidationMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.mode.String()
			if result != tt.expected {
				t.Errorf("FieldValidationMode(%d).String() = %q, want %q", tt.mode, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// FIELD NAMING CONVENTION STRING TESTS
// ============================================================================

func TestFieldNamingConvention_String(t *testing.T) {
	tests := []struct {
		convention FieldNamingConvention
		expected   string
	}{
		{NamingConventionAny, "any"},
		{NamingConventionSnakeCase, "snake_case"},
		{NamingConventionCamelCase, "camelCase"},
		{NamingConventionPascalCase, "PascalCase"},
		{NamingConventionKebabCase, "kebab-case"},
		{FieldNamingConvention(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.convention.String()
			if result != tt.expected {
				t.Errorf("FieldNamingConvention(%d).String() = %q, want %q", tt.convention, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// FIELD VALIDATION CONFIG TESTS
// ============================================================================

func TestDefaultFieldValidationConfig(t *testing.T) {
	cfg := DefaultFieldValidationConfig()

	if cfg.Mode != FieldValidationNone {
		t.Errorf("Expected Mode FieldValidationNone, got %v", cfg.Mode)
	}

	if cfg.Convention != NamingConventionAny {
		t.Errorf("Expected Convention NamingConventionAny, got %v", cfg.Convention)
	}

	if !cfg.AllowCommonAbbreviations {
		t.Error("Expected AllowCommonAbbreviations to be true")
	}

	if !cfg.EnableSecurityValidation {
		t.Error("Expected EnableSecurityValidation to be true")
	}
}

func TestStrictSnakeCaseConfig(t *testing.T) {
	cfg := StrictSnakeCaseConfig()

	if cfg.Mode != FieldValidationStrict {
		t.Errorf("Expected Mode FieldValidationStrict, got %v", cfg.Mode)
	}

	if cfg.Convention != NamingConventionSnakeCase {
		t.Errorf("Expected Convention NamingConventionSnakeCase, got %v", cfg.Convention)
	}
}

func TestStrictCamelCaseConfig(t *testing.T) {
	cfg := StrictCamelCaseConfig()

	if cfg.Mode != FieldValidationStrict {
		t.Errorf("Expected Mode FieldValidationStrict, got %v", cfg.Mode)
	}

	if cfg.Convention != NamingConventionCamelCase {
		t.Errorf("Expected Convention NamingConventionCamelCase, got %v", cfg.Convention)
	}
}

// ============================================================================
// VALIDATE FIELD KEY TESTS
// ============================================================================

func TestValidateFieldKey(t *testing.T) {
	t.Run("nil config returns nil", func(t *testing.T) {
		var cfg *FieldValidationConfig
		err := cfg.ValidateFieldKey("any_key")
		if err != nil {
			t.Errorf("Expected nil error for nil config, got: %v", err)
		}
	})

	t.Run("FieldValidationNone returns nil", func(t *testing.T) {
		cfg := DefaultFieldValidationConfig()
		err := cfg.ValidateFieldKey("any_key")
		if err != nil {
			t.Errorf("Expected nil error for FieldValidationNone, got: %v", err)
		}
	})

	t.Run("empty key returns error", func(t *testing.T) {
		cfg := StrictSnakeCaseConfig()
		err := cfg.ValidateFieldKey("")
		if err == nil {
			t.Error("Expected error for empty key")
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("Expected error to mention 'empty', got: %v", err)
		}
	})
}

func TestValidateFieldKey_SnakeCase(t *testing.T) {
	cfg := StrictSnakeCaseConfig()

	tests := []struct {
		key       string
		shouldErr bool
	}{
		{"user_id", false},
		{"first_name", false},
		{"created_at", false},
		{"user_id_123", false},
		{"UserID", true},    // uppercase not allowed
		{"userId", true},    // camelCase not allowed
		{"user-id", true},   // hyphen not allowed
		{"_user_id", false}, // allowed due to _id suffix being a common abbreviation
		{"user_id_", true},  // trailing underscore not allowed
		{"user__id", false}, // allowed due to _id suffix being a common abbreviation
		{"123_user", true},  // leading digit not allowed
		{"", true},          // empty not allowed
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			err := cfg.ValidateFieldKey(tt.key)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected error for key %q", tt.key)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Unexpected error for key %q: %v", tt.key, err)
			}
		})
	}
}

func TestValidateFieldKey_CamelCase(t *testing.T) {
	cfg := StrictCamelCaseConfig()

	tests := []struct {
		key       string
		shouldErr bool
	}{
		{"userId", false},
		{"firstName", false},
		{"createdAt", false},
		{"userID123", false},
		{"user_id", false}, // allowed due to _id suffix being a common abbreviation
		{"UserId", true},   // PascalCase not allowed (must start lowercase)
		{"user-id", true},  // hyphen not allowed
		{"123user", true},  // leading digit allowed in camelCase but starts with digit
		{"", true},         // empty not allowed
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			err := cfg.ValidateFieldKey(tt.key)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected error for key %q", tt.key)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Unexpected error for key %q: %v", tt.key, err)
			}
		})
	}
}

func TestValidateFieldKey_PascalCase(t *testing.T) {
	cfg := &FieldValidationConfig{
		Mode:                     FieldValidationStrict,
		Convention:               NamingConventionPascalCase,
		AllowCommonAbbreviations: false,
		EnableSecurityValidation: false,
	}

	tests := []struct {
		key       string
		shouldErr bool
	}{
		{"UserId", false},
		{"FirstName", false},
		{"CreatedAt", false},
		{"UserID123", false},
		{"userId", true},  // must start uppercase
		{"user_id", true}, // underscore not allowed
		{"User-Id", true}, // hyphen not allowed
		{"", true},        // empty not allowed
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			err := cfg.ValidateFieldKey(tt.key)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected error for key %q", tt.key)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Unexpected error for key %q: %v", tt.key, err)
			}
		})
	}
}

func TestValidateFieldKey_KebabCase(t *testing.T) {
	cfg := &FieldValidationConfig{
		Mode:                     FieldValidationStrict,
		Convention:               NamingConventionKebabCase,
		AllowCommonAbbreviations: false,
		EnableSecurityValidation: false,
	}

	tests := []struct {
		key       string
		shouldErr bool
	}{
		{"user-id", false},
		{"first-name", false},
		{"created-at", false},
		{"user-id-123", false},
		{"UserID", true},   // uppercase not allowed
		{"userId", true},   // camelCase not allowed
		{"user_id", true},  // underscore not allowed
		{"-user-id", true}, // leading hyphen not allowed
		{"user-id-", true}, // trailing hyphen not allowed
		{"user--id", true}, // consecutive hyphens not allowed
		{"123-user", true}, // leading digit not allowed
		{"", true},         // empty not allowed
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			err := cfg.ValidateFieldKey(tt.key)
			if tt.shouldErr && err == nil {
				t.Errorf("Expected error for key %q", tt.key)
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Unexpected error for key %q: %v", tt.key, err)
			}
		})
	}
}

// ============================================================================
// COMMON ABBREVIATION TESTS
// ============================================================================

func TestIsCommonAbbreviation(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		// Exact matches
		{"id", true},
		{"ID", true},
		{"url", true},
		{"URL", true},
		{"uri", true},
		{"URI", true},
		{"http", true},
		{"HTTP", true},
		{"https", true},
		{"HTTPS", true},
		{"api", true},
		{"API", true},
		{"json", true},
		{"JSON", true},
		{"xml", true},
		{"XML", true},
		{"html", true},
		{"HTML", true},
		{"sql", true},
		{"SQL", true},
		{"ip", true},
		{"IP", true},
		{"tcp", true},
		{"TCP", true},
		{"udp", true},
		{"UDP", true},
		{"ssl", true},
		{"SSL", true},
		{"tls", true},
		{"TLS", true},
		{"jwt", true},
		{"JWT", true},
		{"oauth", true},
		{"OAuth", true},
		// Suffixes (case-insensitive check)
		{"user_id", true},
		{"request_url", true},
		{"redirect_uri", true},
		{"client_ip", true},
		{"apiKey", false}, // not a suffix match, ends with "ey" not "_api"
		// Non-abbreviations
		{"username", false},
		{"password", false},
		{"firstName", false},
		{"random_key", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := isCommonAbbreviation(tt.key)
			if result != tt.expected {
				t.Errorf("isCommonAbbreviation(%q) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}

func TestValidateFieldKey_WithCommonAbbreviations(t *testing.T) {
	cfg := StrictSnakeCaseConfig()

	// Common abbreviations should pass even if they don't match snake_case
	allowedKeys := []string{"ID", "URL", "API", "HTTP", "user_id", "request_url"}
	for _, key := range allowedKeys {
		err := cfg.ValidateFieldKey(key)
		if err != nil {
			t.Errorf("Key %q should be allowed as common abbreviation: %v", key, err)
		}
	}
}

func TestValidateFieldKey_WithoutCommonAbbreviations(t *testing.T) {
	cfg := &FieldValidationConfig{
		Mode:                     FieldValidationStrict,
		Convention:               NamingConventionSnakeCase,
		AllowCommonAbbreviations: false,
		EnableSecurityValidation: false,
	}

	// Without common abbreviations, these should fail snake_case validation
	disallowedKeys := []string{"ID", "URL", "API"}
	for _, key := range disallowedKeys {
		err := cfg.ValidateFieldKey(key)
		if err == nil {
			t.Errorf("Key %q should fail snake_case validation when abbreviations disabled", key)
		}
	}
}

// ============================================================================
// NAMING CONVENTION ANY TESTS
// ============================================================================

func TestValidateFieldKey_NamingConventionAny(t *testing.T) {
	cfg := &FieldValidationConfig{
		Mode:                     FieldValidationStrict,
		Convention:               NamingConventionAny,
		AllowCommonAbbreviations: false,
		EnableSecurityValidation: false,
	}

	// Any convention should accept all valid keys
	keys := []string{"user_id", "userId", "UserId", "user-id", "ID", "URL"}
	for _, key := range keys {
		err := cfg.ValidateFieldKey(key)
		if err != nil {
			t.Errorf("Key %q should be allowed with NamingConventionAny: %v", key, err)
		}
	}
}

// ============================================================================
// SECURITY VALIDATION TESTS
// ============================================================================

func TestValidateFieldKey_SecurityValidation(t *testing.T) {
	cfg := &FieldValidationConfig{
		Mode:                     FieldValidationStrict,
		Convention:               NamingConventionAny,
		AllowCommonAbbreviations: false,
		EnableSecurityValidation: true,
	}

	// These should be caught by security validation
	dangerousKeys := []string{
		"${env:PASSWORD}",  // Log4Shell style
		"jndi:ldap://evil", // JNDI injection
	}

	for _, key := range dangerousKeys {
		err := cfg.ValidateFieldKey(key)
		if err == nil {
			t.Errorf("Key %q should be rejected by security validation", key)
		}
	}
}

func TestValidateFieldKey_SecurityValidationDisabled(t *testing.T) {
	cfg := &FieldValidationConfig{
		Mode:                     FieldValidationStrict,
		Convention:               NamingConventionAny,
		AllowCommonAbbreviations: false,
		EnableSecurityValidation: false,
	}

	// Without security validation, these might pass (depending on convention)
	key := "normal_key"
	err := cfg.ValidateFieldKey(key)
	if err != nil {
		t.Errorf("Normal key should pass: %v", err)
	}
}

// ============================================================================
// LOGGER HOOK MANAGEMENT TESTS
// ============================================================================

func TestLogger_GetHooks(t *testing.T) {
	t.Run("returns nil when no hooks", func(t *testing.T) {
		logger, _ := New()
		hooks := logger.GetHooks()
		if hooks != nil {
			t.Error("Expected nil hooks for new logger")
		}
	})

	t.Run("returns copy of hooks", func(t *testing.T) {
		logger, _ := New()
		logger.AddHook(HookBeforeLog, func(ctx context.Context, h *HookContext) error {
			return nil
		})

		hooks := logger.GetHooks()
		if hooks == nil {
			t.Fatal("Expected non-nil hooks")
		}
		if hooks.countFor(HookBeforeLog) != 1 {
			t.Errorf("Expected 1 BeforeLog hook, got %d", hooks.countFor(HookBeforeLog))
		}

		// Verify it's a copy
		hooks.Add(HookBeforeLog, func(ctx context.Context, h *HookContext) error {
			return nil
		})
		if logger.GetHooks().countFor(HookBeforeLog) != 1 {
			t.Error("Modifying returned hooks should not affect logger")
		}
	})
}

// ============================================================================
// CONTEXT EXTRACTOR TESTS
// ============================================================================

func TestLogger_GetContextExtractors(t *testing.T) {
	t.Run("returns nil when no extractors", func(t *testing.T) {
		logger, _ := New()
		extractors := logger.GetContextExtractors()
		if extractors != nil {
			t.Error("Expected nil extractors for new logger")
		}
	})

	t.Run("returns extractors after add", func(t *testing.T) {
		logger, _ := New()
		extractor := func(ctx context.Context) []Field {
			return []Field{String("key", "value")}
		}

		err := logger.AddContextExtractor(extractor)
		if err != nil {
			t.Fatalf("AddContextExtractor failed: %v", err)
		}

		extractors := logger.GetContextExtractors()
		if extractors == nil {
			t.Fatal("Expected non-nil extractors")
		}
		if len(extractors) != 1 {
			t.Errorf("Expected 1 extractor, got %d", len(extractors))
		}
	})
}

func TestLogger_SetContextExtractors(t *testing.T) {
	t.Run("sets extractors", func(t *testing.T) {
		logger, _ := New()
		extractor := func(ctx context.Context) []Field {
			return []Field{String("key", "value")}
		}

		err := logger.SetContextExtractors(extractor)
		if err != nil {
			t.Fatalf("SetContextExtractors failed: %v", err)
		}

		if logger.GetContextExtractors() == nil {
			t.Error("Expected extractors to be set")
		}
	})

	t.Run("replaces existing extractors", func(t *testing.T) {
		logger, _ := New()

		logger.AddContextExtractor(func(ctx context.Context) []Field {
			return []Field{String("first", "value")}
		})

		err := logger.SetContextExtractors(func(ctx context.Context) []Field {
			return []Field{String("second", "value")}
		})
		if err != nil {
			t.Fatalf("SetContextExtractors failed: %v", err)
		}

		extractors := logger.GetContextExtractors()
		if len(extractors) != 1 {
			t.Errorf("Expected 1 extractor after replace, got %d", len(extractors))
		}
	})

	t.Run("sets multiple extractors at once", func(t *testing.T) {
		logger, _ := New()

		err := logger.SetContextExtractors(
			func(ctx context.Context) []Field { return []Field{String("first", "1")} },
			func(ctx context.Context) []Field { return []Field{String("second", "2")} },
		)
		if err != nil {
			t.Fatalf("SetContextExtractors failed: %v", err)
		}

		extractors := logger.GetContextExtractors()
		if len(extractors) != 2 {
			t.Errorf("Expected 2 extractors, got %d", len(extractors))
		}
	})
}

// ============================================================================
// TRIGGER HOOKS INTEGRATION TESTS
// ============================================================================

func TestLogger_TriggerHooksViaLogging(t *testing.T) {
	t.Run("hooks are triggered during logging", func(t *testing.T) {
		var buf bytes.Buffer
		var hookCalled bool

		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Hooks = NewHookRegistry()
		cfg.Hooks.Add(HookBeforeLog, func(ctx context.Context, h *HookContext) error {
			hookCalled = true
			return nil
		})

		logger, _ := New(cfg)
		logger.Info("test message")

		if !hookCalled {
			t.Error("Expected hook to be called during logging")
		}
	})

	t.Run("hook error stops logging", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Hooks = NewHookRegistry()
		cfg.Hooks.Add(HookBeforeLog, func(ctx context.Context, h *HookContext) error {
			return errors.New("hook error")
		})

		logger, _ := New(cfg)
		logger.Info("test message")

		// Message should not be logged when hook returns error
		if buf.Len() > 0 {
			t.Error("Expected no output when hook returns error")
		}
	})
}

// ============================================================================
// CONTEXT EXTRACTOR REGISTRY INTEGRATION TESTS
// ============================================================================

func TestContextExtractor_RegistryIntegration(t *testing.T) {
	t.Run("extractor is stored and can be extracted", func(t *testing.T) {
		logger, _ := New()

		logger.AddContextExtractor(func(ctx context.Context) []Field {
			return []Field{String("extracted", "value")}
		})

		extractors := logger.GetContextExtractors()
		if len(extractors) != 1 {
			t.Fatalf("Expected 1 extractor, got %d", len(extractors))
		}

		// Extract fields using the extractor
		fields := extractors[0](context.Background())
		if len(fields) != 1 || fields[0].Key != "extracted" {
			t.Errorf("Expected extracted field, got %v", fields)
		}
	})

	t.Run("registry can extract from context", func(t *testing.T) {
		registry := NewContextExtractorRegistry()
		registry.Add(func(ctx context.Context) []Field {
			traceID := GetTraceID(ctx)
			if traceID != "" {
				return []Field{String("trace_id", traceID)}
			}
			return nil
		})

		ctx := WithTraceID(context.Background(), "trace-123")
		fields := registry.Extract(ctx)

		if len(fields) != 1 {
			t.Errorf("Expected 1 field, got %d", len(fields))
		}
		if fields[0].Key != "trace_id" || fields[0].Value != "trace-123" {
			t.Errorf("Expected trace_id=trace-123, got %v", fields)
		}
	})

	t.Run("multiple extractors work together", func(t *testing.T) {
		registry := NewContextExtractorRegistry()
		registry.Add(func(ctx context.Context) []Field {
			return []Field{String("first", "1")}
		})
		registry.Add(func(ctx context.Context) []Field {
			return []Field{String("second", "2")}
		})

		fields := registry.Extract(context.Background())
		if len(fields) != 2 {
			t.Errorf("Expected 2 fields, got %d", len(fields))
		}
	})
}

// ============================================================================
// ERROR FIELD CONSTRUCTORS TESTS
// ============================================================================

func TestErrorFieldConstructors(t *testing.T) {
	t.Run("Err with error", func(t *testing.T) {
		field := Err(errors.New("test error"))
		if field.Key != "error" {
			t.Errorf("Expected key 'error', got %q", field.Key)
		}
		if field.Value != "test error" {
			t.Errorf("Expected value 'test error', got %v", field.Value)
		}
	})

	t.Run("Err with nil", func(t *testing.T) {
		field := Err(nil)
		if field.Key != "error" {
			t.Errorf("Expected key 'error', got %q", field.Key)
		}
		if field.Value != nil {
			t.Errorf("Expected nil value, got %v", field.Value)
		}
	})

	t.Run("ErrWithKey with custom key", func(t *testing.T) {
		field := ErrWithKey("custom_error", errors.New("test error"))
		if field.Key != "custom_error" {
			t.Errorf("Expected key 'custom_error', got %q", field.Key)
		}
		if field.Value != "test error" {
			t.Errorf("Expected value 'test error', got %v", field.Value)
		}
	})

	t.Run("ErrWithKey with nil error", func(t *testing.T) {
		field := ErrWithKey("custom_error", nil)
		if field.Key != "custom_error" {
			t.Errorf("Expected key 'custom_error', got %q", field.Key)
		}
		if field.Value != nil {
			t.Errorf("Expected nil value, got %v", field.Value)
		}
	})

	t.Run("ErrWithKey creates field with custom key", func(t *testing.T) {
		field := ErrWithKey("named_error", errors.New("test error"))
		if field.Key != "named_error" {
			t.Errorf("Expected key 'named_error', got %q", field.Key)
		}
		if field.Value != "test error" {
			t.Errorf("Expected value 'test error', got %v", field.Value)
		}
	})

	t.Run("ErrWithStack captures stack trace", func(t *testing.T) {
		field := ErrWithStack(errors.New("test error"))
		if field.Key != "error" {
			t.Errorf("Expected key 'error', got %q", field.Key)
		}
		value, ok := field.Value.(string)
		if !ok {
			t.Fatalf("Expected string value, got %T", field.Value)
		}
		if !strings.Contains(value, "test error") {
			t.Errorf("Expected 'test error' in value, got %s", value)
		}
		if !strings.Contains(value, "Stack:") {
			t.Errorf("Expected stack trace in value, got %s", value)
		}
	})

	t.Run("ErrWithStack with nil error", func(t *testing.T) {
		field := ErrWithStack(nil)
		if field.Key != "error" {
			t.Errorf("Expected key 'error', got %q", field.Key)
		}
		if field.Value != nil {
			t.Errorf("Expected nil value, got %v", field.Value)
		}
	})
}

// ============================================================================
// ENTRY PRINT METHODS TESTS
// ============================================================================

func TestEntry_PrintMethods(t *testing.T) {
	t.Run("Entry Print", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Level = LevelInfo
		logger, _ := New(cfg)

		entry := logger.WithFields(String("key", "value"))
		entry.Print("test", "print")

		if !strings.Contains(buf.String(), "test print") {
			t.Errorf("Expected 'test print' in output, got: %s", buf.String())
		}
	})

	t.Run("Entry Println", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Level = LevelInfo
		logger, _ := New(cfg)

		entry := logger.WithFields(String("key", "value"))
		entry.Println("test", "println")

		if !strings.Contains(buf.String(), "test println") {
			t.Errorf("Expected 'test println' in output, got: %s", buf.String())
		}
	})

	t.Run("Entry Printf", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Level = LevelInfo
		logger, _ := New(cfg)

		entry := logger.WithFields(String("key", "value"))
		entry.Printf("formatted %s", "message")

		if !strings.Contains(buf.String(), "formatted message") {
			t.Errorf("Expected 'formatted message' in output, got: %s", buf.String())
		}
	})

	t.Run("Entry carries fields to print methods", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Output = &buf
		cfg.Format = FormatJSON
		cfg.Level = LevelInfo
		logger, _ := New(cfg)

		entry := logger.WithFields(String("request_id", "abc123"))
		entry.Print("test message")

		output := buf.String()
		if !strings.Contains(output, "request_id") || !strings.Contains(output, "abc123") {
			t.Errorf("Expected field in output, got: %s", output)
		}
	})
}
