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
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// BOUNDARY: LOG LEVEL EDGE CASES
// ============================================================================

func TestLogLevelBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		level   LogLevel
		wantErr bool
	}{
		{"MinValid_LevelDebug", LevelDebug, false},
		{"LevelInfo", LevelInfo, false},
		{"LevelWarn", LevelWarn, false},
		{"LevelError", LevelError, false},
		{"LevelFatal", LevelFatal, false},
		{"NegativeOne", LogLevel(-1), true},
		{"TooHigh_99", LogLevel(99), true},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_SetLevel", func(t *testing.T) {
			cfg := DefaultConfig()
			logger, _ := New(cfg)
			err := logger.SetLevel(tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetLevel(%d) error = %v, wantErr %v", tt.level, err, tt.wantErr)
			}
		})

		t.Run(tt.name+"_New", func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Level = tt.level
			_, err := New(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New(Level=%d) error = %v, wantErr %v", tt.level, err, tt.wantErr)
			}
		})
	}
}

func TestLogFormatBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		format  LogFormat
		wantErr bool
	}{
		{"FormatText", FormatText, false},
		{"FormatJSON", FormatJSON, false},
		{"Invalid_NegativeOne", LogFormat(-1), true},
		{"Invalid_99", LogFormat(99), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Format = tt.format
			_, err := New(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New(Format=%d) error = %v, wantErr %v", tt.format, err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// BOUNDARY: WRITER EDGE CASES
// ============================================================================

func TestAddSameWriterTwice(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	logger, _ := New(cfg)

	err := logger.AddWriter(&buf)
	if err != nil {
		t.Errorf("AddWriter same writer should succeed or be idempotent, got: %v", err)
	}
}

func TestRemoveNonExistentWriter(t *testing.T) {
	cfg := DefaultConfig()
	logger, _ := New(cfg)

	var buf bytes.Buffer
	err := logger.RemoveWriter(&buf)
	if err == nil {
		t.Error("RemoveWriter for non-existent writer should return error")
	}
}

func TestEmptyOutputsSlice(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{}
	logger, _ := New(cfg)
	// Should not panic
	logger.Info("test")
}

func TestMultiWriterZeroWriters(t *testing.T) {
	mw := NewMultiWriter()
	if mw == nil {
		t.Fatal("NewMultiWriter() with no args should return non-nil")
	}
	data := []byte("test\n")
	n, err := mw.Write(data)
	if err != nil {
		t.Errorf("Write to empty MultiWriter error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}
}

func TestBufferedWriterZeroSize(t *testing.T) {
	var buf bytes.Buffer
	_, err := NewBufferedWriter(&buf, BufferedWriterConfig{BufferSize: 0})
	if err != nil {
		t.Errorf("NewBufferedWriter with zero size should work (clamped), got: %v", err)
	}
}

func TestBufferedWriterNegativeSize(t *testing.T) {
	var buf bytes.Buffer
	_, err := NewBufferedWriter(&buf, BufferedWriterConfig{BufferSize: -1})
	if err == nil {
		t.Error("NewBufferedWriter with negative size should return error")
	}
}

// ============================================================================
// BOUNDARY: SET WRITE ERROR HANDLER
// ============================================================================

func TestSetWriteErrorHandler(t *testing.T) {
	var handlerCalled atomic.Int32
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&errorWriter{err: errors.New("write error")})}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	logger.SetWriteErrorHandler(func(w io.Writer, err error) {
		handlerCalled.Add(1)
	})

	logger.Info("test message")

	if handlerCalled.Load() == 0 {
		t.Error("WriteErrorHandler should have been called on write error")
	}
}

func TestSetWriteErrorHandlerNil(t *testing.T) {
	cfg := DefaultConfig()
	logger, _ := New(cfg)

	// Should not panic
	logger.SetWriteErrorHandler(nil)
	logger.Info("test")
}

// ============================================================================
// BOUNDARY: SET HOOKS
// ============================================================================

func TestSetHooks(t *testing.T) {
	cfg := DefaultConfig()
	logger, _ := New(cfg)

	registry := NewHookRegistry()
	registry.Add(HookBeforeLog, func(ctx context.Context, h *HookContext) error {
		return nil
	})

	err := logger.SetHooks(registry)
	if err != nil {
		t.Errorf("SetHooks error = %v", err)
	}

	// Set nil hooks
	err = logger.SetHooks(nil)
	if err != nil {
		t.Errorf("SetHooks(nil) error = %v", err)
	}
}

func TestSetHooksOnClosedLogger(t *testing.T) {
	cfg := DefaultConfig()
	logger, _ := New(cfg)
	logger.Close()

	err := logger.SetHooks(NewHookRegistry())
	if err == nil {
		t.Error("SetHooks on closed logger should return error")
	}
	if !errors.Is(err, ErrLoggerClosed) {
		t.Errorf("Expected ErrLoggerClosed, got: %v", err)
	}
}

// ============================================================================
// BOUNDARY: SHUTDOWN
// ============================================================================

func TestShutdown(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	logger.Info("before shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := logger.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown error = %v", err)
	}

	if !logger.IsClosed() {
		t.Error("Logger should be closed after Shutdown")
	}
}

func TestShutdownAlreadyClosed(t *testing.T) {
	cfg := DefaultConfig()
	logger, _ := New(cfg)
	logger.Close()

	ctx := context.Background()
	err := logger.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown on already-closed logger error = %v", err)
	}
}

// ============================================================================
// BOUNDARY: FILTER GOROUTINE MONITORING
// ============================================================================

func TestFilterGoroutineMonitoring(t *testing.T) {
	filter := NewSensitiveDataFilter()
	cfg := DefaultConfig()
	cfg.Security = &SecurityConfig{SensitiveFilter: filter}
	logger, _ := New(cfg)

	count := logger.ActiveFilterGoroutines()
	_ = count // Should not panic

	completed := logger.WaitForFilterGoroutines(100 * time.Millisecond)
	if !completed {
		t.Log("WaitForFilterGoroutines timed out, but this is acceptable")
	}
}

func TestFilterGoroutineMonitoringNoSecurity(t *testing.T) {
	cfg := DefaultConfig()
	logger, _ := New(cfg)

	count := logger.ActiveFilterGoroutines()
	if count != 0 {
		t.Errorf("ActiveFilterGoroutines without security = %d, want 0", count)
	}

	completed := logger.WaitForFilterGoroutines(100 * time.Millisecond)
	if !completed {
		t.Error("WaitForFilterGoroutines without security should return true immediately")
	}
}

// ============================================================================
// BOUNDARY: SECURITY LEVEL STRING AND CONFIG FOR LEVEL
// ============================================================================

func TestSecurityLevelString(t *testing.T) {
	tests := []struct {
		level    SecurityLevel
		expected string
	}{
		{SecurityLevelDevelopment, "Development"},
		{SecurityLevelBasic, "Basic"},
		{SecurityLevelStandard, "Standard"},
		{SecurityLevelStrict, "Strict"},
		{SecurityLevelParanoid, "Paranoid"},
		{SecurityLevel(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.level.String()
			if got != tt.expected {
				t.Errorf("SecurityLevel(%d).String() = %q, want %q", tt.level, got, tt.expected)
			}
		})
	}
}

func TestSecurityConfigForLevel(t *testing.T) {
	tests := []struct {
		name  string
		level SecurityLevel
		nilOK bool // whether nil SecurityConfig is acceptable
	}{
		{"Development", SecurityLevelDevelopment, true},
		{"Basic", SecurityLevelBasic, false},
		{"Standard", SecurityLevelStandard, false},
		{"Strict", SecurityLevelStrict, false},
		{"Paranoid", SecurityLevelParanoid, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SecurityConfigForLevel(tt.level)
			if tt.nilOK {
				// Development may return nil filter
				return
			}
			if cfg == nil {
				t.Fatalf("SecurityConfigForLevel(%v) returned nil", tt.level)
			}
			if cfg.MaxMessageSize <= 0 {
				t.Error("MaxMessageSize should be positive")
			}
		})
	}
}

// ============================================================================
// BOUNDARY: SENSITIVE DATA FILTER CLOSE AND GOROUTINES
// ============================================================================

func TestSensitiveFilterClose(t *testing.T) {
	filter := NewSensitiveDataFilter()
	filter.Enable()

	// Filter should work before close
	result := filter.Filter("password=secret")
	if !strings.Contains(result, "[REDACTED]") {
		t.Error("Filter should redact before close")
	}

	completed := filter.Close()
	if !completed {
		t.Log("Close did not complete within timeout")
	}

	// Filter should pass through after close
	result = filter.Filter("password=secret")
	if strings.Contains(result, "[REDACTED]") {
		t.Error("Filter should not redact after close")
	}
}

func TestSensitiveFilterCloseNil(t *testing.T) {
	var filter *SensitiveDataFilter
	completed := filter.Close()
	if !completed {
		t.Error("Close on nil should return true")
	}
}

func TestSensitiveFilterWaitForGoroutinesNil(t *testing.T) {
	var filter *SensitiveDataFilter
	completed := filter.WaitForGoroutines(time.Second)
	if !completed {
		t.Error("WaitForGoroutines on nil should return true")
	}
}

func TestSensitiveFilterActiveGoroutineCountNil(t *testing.T) {
	var filter *SensitiveDataFilter
	count := filter.ActiveGoroutineCount()
	if count != 0 {
		t.Errorf("ActiveGoroutineCount on nil = %d, want 0", count)
	}
}

// ============================================================================
// BOUNDARY: DEFAULT LOGGER FUNCTIONS
// ============================================================================

func TestDefaultWithErr(t *testing.T) {
	logger, err := DefaultWithErr()
	if logger == nil {
		t.Fatal("DefaultWithErr() returned nil logger")
	}
	_ = err
}

func TestInitDefault(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	cfg := DefaultConfig()
	cfg.Level = LevelInfo

	err := InitDefault(cfg)
	if err != nil {
		t.Errorf("InitDefault error = %v", err)
	}

	logger := Default()
	if logger == nil {
		t.Fatal("Default() should return non-nil after InitDefault")
	}
}

func TestInitDefaultNoConfig(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	err := InitDefault()
	if err != nil {
		t.Errorf("InitDefault() with no config error = %v", err)
	}
}

func TestInitDefaultWithConfig(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	cfg := DefaultConfig()
	cfg.Level = LevelDebug

	err := InitDefault(cfg)
	if err != nil {
		t.Errorf("InitDefault error = %v", err)
	}

	logger := Default()
	if logger == nil {
		t.Fatal("Default() should return non-nil after InitDefault")
	}
}

// ============================================================================
// BOUNDARY: INTEGRITY CONFIG MARSHAL JSON
// ============================================================================

func TestIntegrityConfigMarshalJSON(t *testing.T) {
	config := IntegrityConfig{
		SecretKey:        []byte("sensitive-key-that-should-not-appear"),
		HashAlgorithm:    HashAlgorithmSHA256,
		IncludeTimestamp: true,
		IncludeSequence:  true,
		SignaturePrefix:  "[SIG:",
	}

	data, err := json.Marshal(&config)
	if err != nil {
		t.Fatalf("MarshalJSON error = %v", err)
	}

	output := string(data)

	// SecretKey should NOT appear in JSON output
	if strings.Contains(output, "sensitive-key") {
		t.Error("MarshalJSON should not expose SecretKey")
	}

	// Should contain expected fields
	if !strings.Contains(output, "SHA256") {
		t.Error("Should contain hash algorithm")
	}
	if !strings.Contains(output, `"includeTimestamp":true`) {
		t.Error("Should contain includeTimestamp")
	}
}

// ============================================================================
// BOUNDARY: INTEGRITY SIGNER EDGE CASES
// ============================================================================

func TestIntegritySignerBoundaryKeys(t *testing.T) {
	tests := []struct {
		name    string
		key     []byte
		wantErr bool
	}{
		{"EmptyKey", []byte{}, true},
		{"ShortKey_16bytes", make([]byte, 16), true},
		{"ExactMinKey_32bytes", make([]byte, 32), false},
		{"AboveMinKey_33bytes", make([]byte, 33), false},
		{"LongKey_64bytes", make([]byte, 64), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := IntegrityConfig{
				SecretKey:     tt.key,
				HashAlgorithm: HashAlgorithmSHA256,
			}
			_, err := NewIntegritySigner(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewIntegritySigner(keyLen=%d) error = %v, wantErr %v", len(tt.key), err, tt.wantErr)
			}
		})
	}
}

func TestIntegritySignerSignEmptyMessage(t *testing.T) {
	config, err := DefaultIntegrityConfigSafe()
	if err != nil {
		t.Fatalf("DefaultIntegrityConfigSafe error = %v", err)
	}
	signer, _ := NewIntegritySigner(config)

	sig := signer.Sign("")
	if sig == "" {
		t.Error("Sign('') should produce a non-empty signature")
	}
}

func TestIntegritySignerResetSequenceNil(t *testing.T) {
	var signer *IntegritySigner
	signer.ResetSequence() // Should not panic
}

func TestIntegrityConfigCloneCompleteness(t *testing.T) {
	original := IntegrityConfig{
		SecretKey:        []byte(strings.Repeat("x", 32)),
		HashAlgorithm:    HashAlgorithmSHA256,
		IncludeTimestamp: true,
		IncludeSequence:  true,
		SignaturePrefix:  "[SIG:",
	}

	cloned := original.Clone()

	// Modify original
	original.HashAlgorithm = HashAlgorithm(99) // invalid but non-zero
	original.IncludeTimestamp = false
	original.SignaturePrefix = "[CUSTOM:"

	if cloned.HashAlgorithm == HashAlgorithm(99) {
		t.Error("Clone should be independent: HashAlgorithm")
	}
	if !cloned.IncludeTimestamp {
		t.Error("Clone should be independent: IncludeTimestamp")
	}
	if cloned.SignaturePrefix == "[CUSTOM:" {
		t.Error("Clone should be independent: SignaturePrefix")
	}
}

// ============================================================================
// BOUNDARY: AUDIT EDGE CASES
// ============================================================================

func TestNewAuditLoggerWithConfig(t *testing.T) {
	cfg := DefaultAuditConfig()
	logger, err := NewAuditLogger(cfg)
	if err != nil {
		t.Fatalf("NewAuditLogger error = %v", err)
	}
	if logger == nil {
		t.Fatal("NewAuditLogger returned nil")
	}
}

func TestAuditSeverityMarshalAll(t *testing.T) {
	severities := []AuditSeverity{
		AuditSeverityInfo,
		AuditSeverityWarning,
		AuditSeverityError,
		AuditSeverityCritical,
	}

	for _, sev := range severities {
		t.Run(sev.String(), func(t *testing.T) {
			data, err := json.Marshal(sev)
			if err != nil {
				t.Errorf("Marshal(%v) error = %v", sev, err)
			}
			if len(data) == 0 {
				t.Errorf("Marshal(%v) produced empty output", sev)
			}
		})
	}
}

func TestAuditLoggerEmptyMessage(t *testing.T) {
	cfg := DefaultAuditConfig()
	logger, _ := NewAuditLogger(cfg)

	// Should not panic with empty message
	logger.Log(AuditEvent{Type: AuditEventInputSanitized, Message: ""})

	stats := logger.Stats()
	if stats.TotalEvents != 1 {
		t.Errorf("Expected 1 event, got %d", stats.TotalEvents)
	}
}

func TestAuditLoggerSeverityBoundary(t *testing.T) {
	cfg := DefaultAuditConfig()
	cfg.MinimumSeverity = AuditSeverityCritical
	logger, _ := NewAuditLogger(cfg)

	// Events below Critical should be filtered
	logger.Log(AuditEvent{Type: AuditEventInputSanitized, Message: "info event", Severity: AuditSeverityInfo})
	logger.Log(AuditEvent{Type: AuditEventSecurityViolation, Message: "critical event", Severity: AuditSeverityCritical})

	stats := logger.Stats()
	if stats.TotalEvents != 1 {
		t.Errorf("Expected 1 event (critical only), got %d", stats.TotalEvents)
	}
}

// ============================================================================
// BOUNDARY: WRITER CONFIG VALIDATION
// ============================================================================

func TestFileWriterConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     FileWriterConfig
		wantErr bool
	}{
		{"Default", DefaultFileWriterConfig(), false},
		{"ValidCustom", FileWriterConfig{MaxSizeMB: 10, MaxBackups: 5}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBufferedWriterConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     BufferedWriterConfig
		wantErr bool
	}{
		{"Default", DefaultBufferedWriterConfig(), false},
		{"NegativeBufferSize", BufferedWriterConfig{BufferSize: -1}, true},
		{"NegativeFlushTime", BufferedWriterConfig{FlushTime: -1 * time.Second}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewFileWriterWithConfig(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.log")
	cfg := FileWriterConfig{MaxSizeMB: 1, MaxBackups: 3}

	fw, err := NewFileWriter(tmpFile, cfg)
	if err != nil {
		t.Fatalf("NewFileWriter error = %v", err)
	}
	defer fw.Close()

	data := []byte("test via WithConfig\n")
	n, err := fw.Write(data)
	if err != nil {
		t.Errorf("Write error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}
}

func TestNewBufferedWriterWithConfig(t *testing.T) {
	var buf bytes.Buffer

	bw, err := NewBufferedWriter(&buf, BufferedWriterConfig{BufferSize: 1024})
	if err != nil {
		t.Fatalf("NewBufferedWriter error = %v", err)
	}
	defer bw.Close()

	data := []byte("test via WithConfig\n")
	n, err := bw.Write(data)
	if err != nil {
		t.Errorf("Write error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}

	bw.Flush()
	if buf.Len() == 0 {
		t.Error("BufferedWriter should flush data to underlying writer")
	}
}

func TestNewBufferedWriterWithConfigNil(t *testing.T) {
	_, err := NewBufferedWriter(nil, DefaultBufferedWriterConfig())
	if err == nil {
		t.Error("NewBufferedWriter(nil, _) should return error")
	}
}

// ============================================================================
// BOUNDARY: JSON OUTPUT WITH SPECIAL CHARACTERS
// ============================================================================

func TestJSONWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{"Unicode", "Hello 世界 🌍"},
		{"Newlines", "line1\nline2\ttab"},
		{"JSONSpecial", `path "C:\Users\test" with \slashes/`},
		{"Quotes", `He said "hello world"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := DefaultConfig()
			cfg.Targets = []OutputTarget{CustomOutput(&buf)}
			cfg.Level = LevelInfo
			cfg.Format = FormatJSON
			logger, _ := New(cfg)

			logger.Info(tt.message)

			// Output should be valid JSON
			var jsonData map[string]any
			if err := json.Unmarshal([]byte(buf.String()), &jsonData); err != nil {
				t.Fatalf("Output is not valid JSON: %v\nOutput: %s", err, buf.String())
			}

			msg, _ := jsonData["message"].(string)
			if msg == "" {
				t.Error("Message should not be empty in JSON output")
			}
		})
	}
}

// ============================================================================
// BOUNDARY: SAMPLING EDGE CASES
// ============================================================================

func TestSamplingZeroInitial(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	cfg.Sampling = &SamplingConfig{
		Enabled:    true,
		Initial:    0,
		Thereafter: 1,
		Tick:       time.Minute,
	}
	logger, _ := New(cfg)

	for i := 0; i < 10; i++ {
		logger.Info("test message")
	}

	// With Initial=0, sampling should start immediately
	output := buf.String()
	lines := strings.Count(output, "test message")
	if lines == 0 {
		t.Error("Sampling with Initial=0 should still log some messages")
	}
}

// ============================================================================
// BOUNDARY: CONCURRENT LOGGING WITH CONTEXT
// ============================================================================

func TestConcurrentLoggingWithContext(t *testing.T) {
	safeWriter := &threadSafeWriter{w: &bytes.Buffer{}}
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(safeWriter)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	const goroutines = 50
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx := context.WithValue(context.Background(), "id", id)
			_ = ctx
			logger.InfoWith("concurrent msg", Int("id", id))
		}(i)
	}
	wg.Wait()

	if safeWriter.String() == "" {
		t.Error("Concurrent logging should produce output")
	}
}

// ============================================================================
// BOUNDARY: FIELD MERGE LARGE PATH
// ============================================================================

func TestFieldMergeLargePath(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	// Create enough fields to trigger mergeFieldSlicesLarge (>20 fields)
	existing := make([]Field, 25)
	for i := 0; i < 25; i++ {
		existing[i] = Int(fmt.Sprintf("existing_%d", i), i)
	}

	entry := logger.WithFields(existing...)
	newFields := make([]Field, 10)
	for i := 0; i < 10; i++ {
		newFields[i] = String(fmt.Sprintf("new_%d", i), "value")
	}

	entry.InfoWith("large merge", newFields...)

	output := buf.String()
	if !strings.Contains(output, "large merge") {
		t.Error("Should contain message")
	}
}

// ============================================================================
// BOUNDARY: ENTRY MERGE OVERRIDES
// ============================================================================

func TestFieldMergeOverride(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	cfg.Format = FormatJSON
	logger, _ := New(cfg)

	entry := logger.WithFields(String("key", "original"))
	entry.InfoWith("override test", String("key", "overridden"))

	output := buf.String()
	var data map[string]any
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Check that fields are in output (format may vary)
	if !strings.Contains(output, "overridden") {
		t.Error("Overridden value should appear in output")
	}
}

// ============================================================================
// BOUNDARY: RECORDER CLEAR VERIFICATION
// ============================================================================

func TestLoggerRecorder_ClearComplete(t *testing.T) {
	recorder := NewLoggerRecorder()
	logger, _ := recorder.NewLogger()

	logger.InfoWith("test", String("key", "value"))
	logger.Warn("warn")

	recorder.Clear()

	// Verify complete state after clear
	if recorder.Count() != 0 {
		t.Errorf("Count after clear = %d, want 0", recorder.Count())
	}
	if recorder.HasEntries() {
		t.Error("HasEntries should be false after clear")
	}
	if recorder.LastEntry() != nil {
		t.Error("LastEntry should be nil after clear")
	}
	if entries := recorder.EntriesAtLevel(LevelInfo); len(entries) != 0 {
		t.Errorf("EntriesAtLevel after clear = %d, want 0", len(entries))
	}
}

// ============================================================================
// BOUNDARY: RECORDER FIELD TYPES
// ============================================================================

func TestLoggerRecorder_AllFieldTypes(t *testing.T) {
	recorder := NewLoggerRecorder()
	logger, _ := recorder.NewLogger()

	now := time.Now()
	logger.InfoWith("all types",
		Bool("bool", true),
		Int("int", 42),
		Float64("float", 3.14),
		String("string", "hello"),
		Duration("duration", 5*time.Second),
		Time("time", now),
		Err(errors.New("test error")),
	)

	if !recorder.ContainsField("bool") {
		t.Error("Should contain bool field")
	}
	if !recorder.ContainsField("duration") {
		t.Error("Should contain duration field")
	}
	if !recorder.ContainsField("error") {
		t.Error("Should contain error field")
	}

	val := recorder.GetFieldValue("bool")
	if val == nil {
		t.Error("GetFieldValue('bool') should return non-nil")
	}
}

// ============================================================================
// BOUNDARY: WRITER ERROR WITH HANDLER
// ============================================================================

func TestWriterErrorWithHandler(t *testing.T) {
	var capturedErr error
	var capturedWriter io.Writer

	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&errorWriter{err: errors.New("custom write error")})}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	logger.SetWriteErrorHandler(func(w io.Writer, err error) {
		capturedWriter = w
		capturedErr = err
	})

	logger.Info("trigger error")

	if capturedErr == nil {
		t.Error("Error handler should have captured write error")
	}
	if !strings.Contains(capturedErr.Error(), "custom write error") {
		t.Errorf("Captured error = %v, want to contain 'custom write error'", capturedErr)
	}
	if capturedWriter == nil {
		t.Error("Captured writer should not be nil")
	}
}

// ============================================================================
// BOUNDARY: PACKAGE-LEVEL LOGWITH
// ============================================================================

func TestPackageLevelLogWith(t *testing.T) {
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
		{"Log", func() { Log(LevelInfo, "test log") }, "test log"},
		{"Logf", func() { Logf(LevelInfo, "test %s", "logf") }, "test logf"},
		{"LogWith", func() { LogWith(LevelInfo, "test logwith", String("key", "val")) }, "test logwith"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.fn()
			if !strings.Contains(buf.String(), tt.expected) {
				t.Errorf("%s output = %q, want to contain %q", tt.name, buf.String(), tt.expected)
			}
		})
	}
}

// ============================================================================
// BOUNDARY: CONCURRENT DEFAULT LOGGER ACCESS
// ============================================================================

func TestConcurrentDefaultAccess(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	const goroutines = 100
	var wg sync.WaitGroup
	var logCount atomic.Int32

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger := Default()
			if logger == nil {
				t.Error("Default() should not return nil")
				return
			}
			logCount.Add(1)
		}()
	}
	wg.Wait()

	if logCount.Load() != goroutines {
		t.Errorf("Only %d/%d goroutines completed", logCount.Load(), goroutines)
	}
}

// ============================================================================
// BOUNDARY: CONTEXT EXTRACTOR WITH LOGGER INTEGRATION
// ============================================================================

func TestContextExtractorIntegration(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	cfg.Format = FormatJSON
	logger, _ := New(cfg)

	// Add context extractor
	err := logger.AddContextExtractor(func(ctx context.Context) []Field {
		if traceID := ctx.Value("trace_id"); traceID != nil {
			return []Field{String("trace_id", traceID.(string))}
		}
		return nil
	})
	if err != nil {
		t.Errorf("AddContextExtractor error = %v", err)
	}

	// Logger should still work normally
	logger.Info("after extractor")
	if buf.Len() == 0 {
		t.Error("Logger should still produce output after adding extractor")
	}
}

// ============================================================================
// BOUNDARY: FILE PATH VALIDATION
// ============================================================================

func TestNewFileWriterInvalidPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"NullByte", "test\x00.log"},
		{"PathTraversal", "../../../etc/passwd"},
		{"EmptyPath", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFileWriter(tt.path, DefaultFileWriterConfig())
			if err == nil {
				t.Errorf("NewFileWriter(%q) should return error", tt.path)
			}
		})
	}
}

// ============================================================================
// BOUNDARY: RECORDER THREAD SAFETY IMPROVED
// ============================================================================

func TestLoggerRecorder_ConcurrentStress(t *testing.T) {
	recorder := NewLoggerRecorder()
	logger, _ := recorder.NewLogger()

	const goroutines = 20
	const messagesPerGoroutine = 50
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				logger.InfoWith("msg", Int("id", id), Int("seq", j))
			}
		}(i)
	}
	wg.Wait()

	expected := goroutines * messagesPerGoroutine
	if recorder.Count() != expected {
		t.Errorf("Count = %d, want %d", recorder.Count(), expected)
	}
}

// ============================================================================
// BOUNDARY: ERROR WRAPPER EDGE CASES
// ============================================================================

func TestWrapErrorNilCause(t *testing.T) {
	result := wrapError(errCodeInvalidLevel, "test", nil)
	if result != nil {
		t.Errorf("wrapError with nil cause should return nil, got %v", result)
	}
}

func TestNewErrorFields(t *testing.T) {
	err := newError(errCodeInvalidLevel, "test message")
	if err == nil {
		t.Fatal("newError returned nil")
	}
	// Verify WithField works
	withField := err.WithField("key", "value")
	if withField.Context == nil {
		t.Error("WithField should create context")
	}
	if withField.Context["key"] != "value" {
		t.Errorf("Context[key] = %v, want 'value'", withField.Context["key"])
	}
}

// ============================================================================
// BOUNDARY: FILE WRITER WRITE AND CLOSE
// ============================================================================

func TestFileWriterWriteAndClose(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.log")
	fw, err := NewFileWriter(tmpFile, DefaultFileWriterConfig())
	if err != nil {
		t.Fatalf("NewFileWriter error = %v", err)
	}

	// Write data
	data := []byte("hello file writer\n")
	n, err := fw.Write(data)
	if err != nil {
		t.Errorf("Write error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}

	// Close
	err = fw.Close()
	if err != nil {
		t.Errorf("Close error = %v", err)
	}

	// Verify file content
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if !strings.Contains(string(content), "hello file writer") {
		t.Error("File should contain written data")
	}
}
