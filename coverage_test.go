package dd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// FATAL HANDLER TESTS
// ============================================================================

// TestFatalWithCustomHandler/TestFatalfWithCustomHandler were removed:
// dd_test.go TestFatalLogging is a table over Fatal/Fatalf/FatalWith that
// also asserts the message reaches the output.

func TestLoggerEntryFatal(t *testing.T) {
	tests := []struct {
		name string
		call func(*LoggerEntry)
	}{
		{"Fatal", func(e *LoggerEntry) { e.Fatal("entry fatal message") }},
		{"Fatalf", func(e *LoggerEntry) { e.Fatalf("entry fatalf %s", "message") }},
		{"FatalWith", func(e *LoggerEntry) { e.FatalWith("fatal with message", String("extra", "field")) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := make(chan bool, 1)
			cfg := DefaultConfig()
			cfg.Targets = []OutputTarget{CustomOutput(io.Discard)}
			cfg.FatalHandler = func() { called <- true }
			logger, _ := New(cfg)

			entry := logger.WithFields(String("service", "test"))
			tt.call(entry)

			select {
			case <-called:
				// Success
			case <-time.After(time.Second):
				t.Errorf("FatalHandler not called for LoggerEntry.%s", tt.name)
			}
		})
	}
}

// Note: Structured type tests are covered by LoggerEntry tests above
// since LoggerEntry provides the structured logging functionality

// ============================================================================
// EXIT/EXITF TESTS (Subprocess tests)
// ============================================================================

func TestExit(t *testing.T) {
	if os.Getenv("TEST_EXIT") == "1" {
		Exit("test exit message")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestExit")
	cmd.Env = append(os.Environ(), "TEST_EXIT=1")
	output, err := cmd.CombinedOutput()

	// Exit should call os.Exit(0), which causes the process to exit with code 0
	if e, ok := err.(*exec.ExitError); ok {
		if e.ExitCode() != 0 {
			t.Errorf("Exit should exit with code 0, got %d", e.ExitCode())
		}
	} else if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !strings.Contains(string(output), "test exit message") {
		t.Errorf("Exit output should contain message, got: %s", string(output))
	}
}

func TestExitf(t *testing.T) {
	if os.Getenv("TEST_EXITF") == "1" {
		Exitf("test %s message", "exitf")
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestExitf")
	cmd.Env = append(os.Environ(), "TEST_EXITF=1")
	output, err := cmd.CombinedOutput()

	if e, ok := err.(*exec.ExitError); ok {
		if e.ExitCode() != 0 {
			t.Errorf("Exitf should exit with code 0, got %d", e.ExitCode())
		}
	} else if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !strings.Contains(string(output), "test exitf message") {
		t.Errorf("Exitf output should contain formatted message, got: %s", string(output))
	}
}

func TestHookRegistrySetErrorHandler(t *testing.T) {
	registry := NewHookRegistry()

	var handlerCalled atomic.Int32
	registry.SetErrorHandler(func(event HookEvent, hookCtx *HookContext, err error) {
		handlerCalled.Add(1)
	})

	// Add a hook that always fails
	registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
		return errors.New("hook error")
	})

	// Trigger the hook
	_ = registry.Trigger(context.Background(), HookBeforeLog, &HookContext{})

	if handlerCalled.Load() == 0 {
		t.Error("Error handler should have been called")
	}
}

// ============================================================================
// ERROR TYPE METHOD TESTS
// ============================================================================

func TestMultiWriterErrorAddError(t *testing.T) {
	err := &MultiWriterError{}

	if err.ErrorCount() != 0 {
		t.Error("Initial error count should be 0")
	}

	err.addError(0, io.Discard, errors.New("error 1"))
	if err.ErrorCount() != 1 {
		t.Errorf("Error count should be 1, got %d", err.ErrorCount())
	}

	err.addError(1, io.Discard, errors.New("error 2"))
	if err.ErrorCount() != 2 {
		t.Errorf("Error count should be 2, got %d", err.ErrorCount())
	}
}

// ============================================================================
// VERIFY AUDIT EVENT TEST
// ============================================================================

func TestVerifyAuditEvent(t *testing.T) {
	// Create a signer with predictable settings for testing
	config := IntegrityConfig{
		SecretKey:        make([]byte, 32),
		HashAlgorithm:    HashAlgorithmSHA256,
		IncludeTimestamp: false, // Disable for predictable signatures
		IncludeSequence:  false,
		SignaturePrefix:  "[SIG:",
	}

	signer, err := NewIntegritySigner(config)
	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	// Create an audit event
	event := `{"type":"TEST","message":"test message"}`

	// Sign the event
	signature := signer.Sign(event)

	// Create signed entry - format: "message[SIG:...]" (no space between)
	signedEntry := event + signature

	// Verify the event
	result := VerifyAuditEvent(signedEntry, signer)
	if !result.Valid {
		t.Errorf("VerifyAuditEvent should return valid, got error: %v", result.Error)
	}

	// Test with invalid signature
	invalidEntry := event + "[SIG:invalid]"
	result = VerifyAuditEvent(invalidEntry, signer)
	if result.Valid {
		t.Error("VerifyAuditEvent should return invalid for bad signature")
	}

	// Test with malformed entry (no signature)
	malformedEntry := "no_signature_here"
	result = VerifyAuditEvent(malformedEntry, signer)
	if result.Valid {
		t.Error("VerifyAuditEvent should return invalid for entry without signature")
	}
}

func TestVerifyAuditEventWithNilSigner(t *testing.T) {
	// Verify with nil signer should not panic
	result := VerifyAuditEvent("test", nil)
	if result.Valid {
		t.Error("VerifyAuditEvent with nil signer should return invalid")
	}
}

func TestVerifyAuditEventWithEmptyEntry(t *testing.T) {
	config, err := DefaultIntegrityConfigSafe()
	if err != nil {
		t.Fatalf("DefaultIntegrityConfigSafe error = %v", err)
	}
	signer, _ := NewIntegritySigner(config)

	result := VerifyAuditEvent("", signer)
	if result.Valid {
		t.Error("VerifyAuditEvent with empty entry should return invalid")
	}
}

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
	// An empty Targets slice is not nil: New must accept it rather than
	// error, and logging must be a no-op without panicking.
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{}
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New(empty targets) error = %v, want nil", err)
	}
	if logger == nil {
		t.Fatal("New(empty targets) = nil logger")
	}
	defer logger.Close()
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

// TestSetWriteErrorHandlerNil was removed as a standalone test; its
// nil-handler branch runs inside TestWriterErrorWithHandler below.

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
	t.Run("without security", func(t *testing.T) {
		cfg := DefaultConfig()
		logger, _ := New(cfg)

		if count := logger.ActiveFilterGoroutines(); count != 0 {
			t.Errorf("ActiveFilterGoroutines without security = %d, want 0", count)
		}
		if !logger.WaitForFilterGoroutines(100 * time.Millisecond) {
			t.Error("WaitForFilterGoroutines without security should return true immediately")
		}
	})

	t.Run("with idle security filter", func(t *testing.T) {
		filter := NewSensitiveDataFilter()
		cfg := DefaultConfig()
		cfg.Security = &SecurityConfig{SensitiveFilter: filter}
		logger, _ := New(cfg)

		// No logs emitted: no in-flight filter goroutines to wait for.
		if !logger.WaitForFilterGoroutines(time.Second) {
			t.Error("WaitForFilterGoroutines should complete with no in-flight work")
		}
		if count := logger.ActiveFilterGoroutines(); count != 0 {
			t.Errorf("ActiveFilterGoroutines with idle filter = %d, want 0", count)
		}
	})
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

func TestSensitiveFilterNilReceiver(t *testing.T) {
	var filter *SensitiveDataFilter
	if !filter.Close() {
		t.Error("Close on nil should return true")
	}
	if !filter.WaitForGoroutines(time.Second) {
		t.Error("WaitForGoroutines on nil should return true")
	}
	if count := filter.ActiveGoroutineCount(); count != 0 {
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
	// Default initialization cannot fail with DefaultConfig; a non-nil error
	// here would mean the default logger is running in fallback mode.
	if err != nil {
		t.Errorf("DefaultWithErr() unexpected init error: %v", err)
	}
}

func TestInitDefault(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	tests := []struct {
		name string
		cfg  Config
	}{
		{"info level", func() Config {
			cfg := DefaultConfig()
			cfg.Level = LevelInfo
			return cfg
		}()},
		{"debug level", func() Config {
			cfg := DefaultConfig()
			cfg.Level = LevelDebug
			return cfg
		}()},
		{"defaults", DefaultConfig()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := InitDefault(tt.cfg); err != nil {
				t.Fatalf("InitDefault error = %v", err)
			}
			if Default() == nil {
				t.Fatal("Default() should return non-nil after InitDefault")
			}
			if DefaultInitError() != nil {
				t.Errorf("DefaultInitError() = %v after successful InitDefault, want nil", DefaultInitError())
			}
		})
	}

	// The no-argument form is a separate call shape; verify it too.
	if err := InitDefault(); err != nil {
		t.Errorf("InitDefault() with no config error = %v", err)
	}
}

// TestSetDefaultClearsStaleInitError pins SetDefault's contract: installing a
// caller-provided logger clears any initialization error recorded by a
// previous (fallback) default logger, mirroring InitDefault's clear-on-success.
// Otherwise DefaultInitError/DefaultWithErr keep reporting a stale error for a
// logger that is no longer installed.
func TestSetDefaultClearsStaleInitError(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	// Simulate the state Default() leaves behind when its build fails and a
	// fallback logger is installed: an error recorded for later retrieval.
	stale := errors.New("stale init error")
	defaultInitErr.Store(stale)
	t.Cleanup(func() { defaultInitErr.Store(errNoInit) })

	if err := DefaultInitError(); err == nil {
		t.Fatal("precondition: DefaultInitError() should report the stored error")
	}

	logger, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	SetDefault(logger)

	if err := DefaultInitError(); err != nil {
		t.Errorf("DefaultInitError() = %v after SetDefault, want nil (stale error cleared)", err)
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

// TestNewBufferedWriterWithConfigNil was removed: boundary_test.go
// TestBufferedWriterBoundary asserts the same nil-writer rejection.

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
			if err := json.Unmarshal(buf.Bytes(), &jsonData); err != nil {
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

	// Clearing the handler must be safe: the nil branch of
	// handleWriteError runs on the next failing write.
	logger.SetWriteErrorHandler(nil)
	logger.Info("trigger error again") // must not panic with nil handler
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

// TestLoggerRecorder_ConcurrentStress was removed: recorder_test.go
// TestLoggerRecorder_ThreadSafety covers the same concurrent-write count.

// ============================================================================
// BOUNDARY: ERROR WRAPPER EDGE CASES
// ============================================================================

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
// CONTEXT HELPER TESTS
// ============================================================================

func TestContextSettersAndGetters(t *testing.T) {
	tests := []struct {
		name    string
		setFunc func(context.Context, string) context.Context
		getFunc func(context.Context) string
		value   string
	}{
		{"TraceID", WithTraceID, GetTraceID, "trace-123"},
		{"SpanID", WithSpanID, GetSpanID, "span-456"},
		{"RequestID", WithRequestID, GetRequestID, "req-789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			newCtx := tt.setFunc(ctx, tt.value)

			if newCtx == nil {
				t.Fatalf("%s should return non-nil context", tt.name)
			}

			got := tt.getFunc(newCtx)
			if got != tt.value {
				t.Errorf("got = %q, want %q", got, tt.value)
			}
		})
	}
}

func TestContextGetters_Empty(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		got  string
	}{
		{"GetTraceID", GetTraceID(ctx)},
		{"GetSpanID", GetSpanID(ctx)},
		{"GetRequestID", GetRequestID(ctx)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != "" {
				t.Errorf("%s() on empty context = %q, want empty", tt.name, tt.got)
			}
		})
	}
}

func TestContextKeys_WithLogger(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	cfg.Format = FormatJSON
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	ctx := context.Background()
	ctx = WithTraceID(ctx, "trace-abc")
	ctx = WithSpanID(ctx, "span-def")
	ctx = WithRequestID(ctx, "req-ghi")

	// Manually extract context values and pass them as fields
	logger.InfoWith("test message with context",
		String("trace_id", GetTraceID(ctx)),
		String("span_id", GetSpanID(ctx)),
		String("request_id", GetRequestID(ctx)),
		String("user", "test"),
	)

	output := buf.String()
	if !strings.Contains(output, "trace-abc") {
		t.Errorf("Output should contain trace_id, got: %s", output)
	}
}

// ============================================================================
// DEFAULT FILE WRITER CONFIG TEST
// ============================================================================

func TestDefaultFileWriterConfig(t *testing.T) {
	config := DefaultFileWriterConfig()

	if config.MaxSizeMB != DefaultMaxSizeMB {
		t.Errorf("MaxSizeMB = %d, want %d", config.MaxSizeMB, DefaultMaxSizeMB)
	}
	if config.MaxBackups != DefaultMaxBackups {
		t.Errorf("MaxBackups = %d, want %d", config.MaxBackups, DefaultMaxBackups)
	}
	if config.MaxAge != DefaultMaxAge {
		t.Errorf("MaxAge = %v, want %v", config.MaxAge, DefaultMaxAge)
	}
	if config.Compress != false {
		t.Error("Compress should be false by default")
	}
}

// ============================================================================
// CONTEXT EXTRACTOR EDGE CASES
// ============================================================================

// TestExtractorRegistry_NilContext/TestExtractorRegistry_EmptyRegistry were
// removed: registry_test.go TestExtractorRegistry_Extract covers both the
// empty-registry and nil-context shapes.

// ============================================================================
// LOGGER ENTRY METHOD TESTS
// ============================================================================

// TestLoggerEntryMethods consolidates every LoggerEntry call shape: the
// per-level convenience methods, the generic level-parameterized methods,
// and the FATAL variants (which run a custom FatalHandler instead of
// exiting). Every row asserts both the message and that the entry's pre-set
// field survives into the output. Fatal rows close the logger, so each row
// builds a fresh one.
func TestLoggerEntryMethods(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(*LoggerEntry)
		expected string
	}{
		// Per-level convenience methods
		{"Debug", func(e *LoggerEntry) { e.Debug("debug msg") }, "debug msg"},
		{"Info", func(e *LoggerEntry) { e.Info("info msg") }, "info msg"},
		{"Warn", func(e *LoggerEntry) { e.Warn("warn msg") }, "warn msg"},
		{"Error", func(e *LoggerEntry) { e.Error("error msg") }, "error msg"},
		{"Fatal", func(e *LoggerEntry) { e.Fatal("fatal msg") }, "fatal msg"},
		// Formatted variants
		{"Debugf", func(e *LoggerEntry) { e.Debugf("debug: %s", "formatted") }, "debug: formatted"},
		{"Infof", func(e *LoggerEntry) { e.Infof("info: %d", 42) }, "info: 42"},
		{"Warnf", func(e *LoggerEntry) { e.Warnf("warn: %v", true) }, "warn: true"},
		{"Errorf", func(e *LoggerEntry) { e.Errorf("error: %s", "test") }, "error: test"},
		{"Fatalf", func(e *LoggerEntry) { e.Fatalf("fatal: %s", "test") }, "fatal: test"},
		// Structured variants
		{"DebugWith", func(e *LoggerEntry) { e.DebugWith("debug msg", String("extra", "debug")) }, "debug msg"},
		{"InfoWith", func(e *LoggerEntry) { e.InfoWith("info msg", String("extra", "info")) }, "info msg"},
		{"WarnWith", func(e *LoggerEntry) { e.WarnWith("warn msg", String("extra", "warn")) }, "warn msg"},
		{"ErrorWith", func(e *LoggerEntry) { e.ErrorWith("error msg", String("extra", "error")) }, "error msg"},
		{"FatalWith", func(e *LoggerEntry) { e.FatalWith("fatal msg", String("extra", "fatal")) }, "fatal msg"},
		// Generic level-parameterized methods
		{"Log", func(e *LoggerEntry) { e.Log(LevelInfo, "info message") }, "info message"},
		{"Logf", func(e *LoggerEntry) { e.Logf(LevelWarn, "warning: %s", "test") }, "warning: test"},
		{"LogWith", func(e *LoggerEntry) { e.LogWith(LevelError, "error message", String("extra", "data")) }, "error message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := DefaultConfig()
			cfg.Targets = []OutputTarget{CustomOutput(&buf)}
			cfg.Level = LevelDebug
			cfg.FatalHandler = func() {} // entry.Fatal* must not os.Exit
			logger, _ := New(cfg)
			defer logger.Close()

			entry := logger.WithFields(String("service", "api"))
			tt.logFunc(entry)

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("Entry.%s() should contain %q, got: %s", tt.name, tt.expected, output)
			}
			if !strings.Contains(output, "service=api") {
				t.Errorf("Entry.%s() should carry the entry's pre-set field, got: %s", tt.name, output)
			}
		})
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

// ============================================================================
// LOGGER INSTANCE PRINT/PRINTLN/PRINTF TESTS (MERGED)
// ============================================================================

func TestLoggerPrintMethods(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(*Logger)
		expected string
	}{
		{
			name: "Print",
			logFunc: func(l *Logger) {
				l.Print("test", "print")
			},
			expected: "test print",
		},
		{
			name: "Println",
			logFunc: func(l *Logger) {
				l.Println("test", "println")
			},
			expected: "test println",
		},
		{
			name: "Printf",
			logFunc: func(l *Logger) {
				l.Printf("test %s", "formatted")
			},
			expected: "test formatted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := DefaultConfig()
			cfg.Targets = []OutputTarget{CustomOutput(&buf)}
			cfg.Level = LevelDebug

			logger, err := New(cfg)
			if err != nil {
				t.Fatalf("Failed to create logger: %v", err)
			}
			defer logger.Close()

			tt.logFunc(logger)

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("logger.%s() should contain %q, got: %s", tt.name, tt.expected, output)
			}
		})
	}
}

// ============================================================================
// LOGGER INSTANCE TEXT/TEXTF/JSON/JSONF TESTS (MERGED)
// ============================================================================

func TestLoggerVisualizationMethods(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(*Logger)
		expected []string
	}{
		{
			name: "Text",
			logFunc: func(l *Logger) {
				l.Text("test data")
			},
			expected: []string{"test data"},
		},
		{
			name: "Textf",
			logFunc: func(l *Logger) {
				l.Textf("test %s", "formatted")
			},
			expected: []string{"test formatted"},
		},
		{
			name: "JSON",
			logFunc: func(l *Logger) {
				l.JSON(map[string]string{"key": "value"})
			},
			expected: []string{`"key"`, `"value"`},
		},
		{
			name: "JSONF",
			logFunc: func(l *Logger) {
				l.JSONF("test: %s", "formatted")
			},
			expected: []string{"test: formatted"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := New()
			if err != nil {
				t.Fatalf("Failed to create logger: %v", err)
			}
			defer logger.Close()

			r, w, _ := os.Pipe()
			oldStdout := os.Stdout
			os.Stdout = w

			tt.logFunc(logger)

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			for _, exp := range tt.expected {
				if !strings.Contains(output, exp) {
					t.Errorf("logger.%s() should contain %q, got: %s", tt.name, exp, output)
				}
			}
		})
	}
}

// ============================================================================
// ============================================================================
// SECURITY FILTER ENABLE/DISABLE TESTS
// ============================================================================

func TestSecurityFilterEnableDisable(t *testing.T) {
	filter := NewSensitiveDataFilter()

	t.Run("Enable", func(t *testing.T) {
		filter.Disable()
		filter.Enable()

		if !filter.IsEnabled() {
			t.Error("Filter should be enabled after Enable()")
		}
	})

	t.Run("Disable", func(t *testing.T) {
		filter.Enable()
		filter.Disable()

		if filter.IsEnabled() {
			t.Error("Filter should be disabled after Disable()")
		}
	})

	t.Run("IsEnabled", func(t *testing.T) {
		filter.Enable()
		if !filter.IsEnabled() {
			t.Error("IsEnabled() should return true when enabled")
		}

		filter.Disable()
		if filter.IsEnabled() {
			t.Error("IsEnabled() should return false when disabled")
		}
	})

	t.Run("FilterRespectsEnableDisable", func(t *testing.T) {
		filter.Enable()
		result1 := filter.Filter("password=secret123")
		if !strings.Contains(result1, "[REDACTED]") {
			t.Error("Enabled filter should redact")
		}

		filter.Disable()
		result2 := filter.Filter("password=secret123")
		if strings.Contains(result2, "[REDACTED]") {
			t.Error("Disabled filter should not redact")
		}
		if result2 != "password=secret123" {
			t.Errorf("Disabled filter should return original, got %s", result2)
		}
	})
}

// ============================================================================
// COMPLEX TYPE FORMATTING TESTS
// ============================================================================

func TestComplexTypeFormatting(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	t.Run("SliceFormatting", func(t *testing.T) {
		buf.Reset()
		logger.Info("Users:", []string{"alice", "bob"})
		output := buf.String()

		if !strings.Contains(output, `["alice","bob"]`) {
			t.Errorf("Slice should format as JSON, got: %s", output)
		}
	})

	t.Run("MapFormatting", func(t *testing.T) {
		buf.Reset()
		logger.Info("Config:", map[string]int{"a": 1, "b": 2})
		output := buf.String()

		if !strings.Contains(output, `{"`) {
			t.Errorf("Map should format as JSON, got: %s", output)
		}
	})

	t.Run("NestedSliceFormatting", func(t *testing.T) {
		buf.Reset()
		logger.Info("Matrix:", [][]int{{1, 2}, {3, 4}})
		output := buf.String()

		if !strings.Contains(output, `[[`) {
			t.Errorf("Nested slice should format as JSON, got: %s", output)
		}
	})

	t.Run("NilSliceFormatting", func(t *testing.T) {
		buf.Reset()
		logger.Info("Nil:", []string(nil))
		output := buf.String()

		if !strings.Contains(output, `[]`) {
			t.Errorf("Nil slice should format as [], got: %s", output)
		}
	})

	t.Run("EmptySliceFormatting", func(t *testing.T) {
		buf.Reset()
		logger.Info("Empty:", []int{})
		output := buf.String()

		if !strings.Contains(output, `[]`) {
			t.Errorf("Empty slice should format as [], got: %s", output)
		}
	})
}

// ============================================================================
// STRUCTURED LOGGING COMPLEX TYPE TESTS
// ============================================================================

func TestStructuredLoggingComplexTypes(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	t.Run("SliceInField", func(t *testing.T) {
		buf.Reset()
		logger.InfoWith("Users", Any("names", []string{"alice", "bob"}))
		output := buf.String()

		if !strings.Contains(output, `["alice","bob"]`) {
			t.Errorf("Field with slice should format as JSON, got: %s", output)
		}
	})

	t.Run("MapInField", func(t *testing.T) {
		buf.Reset()
		logger.InfoWith("Config", Any("config", map[string]int{"port": 8080}))
		output := buf.String()

		if !strings.Contains(output, `{"port":8080}`) && !strings.Contains(output, `"port":8080`) {
			t.Errorf("Field with map should format as JSON, got: %s", output)
		}
	})

	t.Run("StructInField", func(t *testing.T) {
		buf.Reset()
		type User struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		user := User{Name: "John", Age: 30}
		logger.InfoWith("User", Any("user", user))
		output := buf.String()

		if !strings.Contains(output, `"name"`) && !strings.Contains(output, `"age"`) {
			t.Errorf("Field with struct should format as JSON, got: %s", output)
		}
	})

	t.Run("TimeInField", func(t *testing.T) {
		buf.Reset()
		now := time.Now()
		logger.InfoWith("Timestamp", Any("time", now))
		output := buf.String()

		if len(output) == 0 {
			t.Error("Time field should produce output")
		}
	})
}

// ============================================================================
// FILE ROTATION AND COMPRESSION TESTS
// ============================================================================

func TestFileRotationTrigger(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	fw, err := NewFileWriter(logFile, FileWriterConfig{
		MaxSizeMB:  1,
		MaxBackups: 3,
		MaxAge:     24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewFileWriter() error = %v", err)
	}

	largeData := make([]byte, 1024*1024) // 1MB of zeros
	for i := 0; i < 3; i++ {             // 3MB total into a 1MB cap
		if n, err := fw.Write(largeData); err != nil || n != len(largeData) {
			t.Fatalf("Write %d = %d bytes, err %v", i, n, err)
		}
	}
	fw.Close()

	// Rotation happens synchronously inside Write once the cap is exceeded:
	// after 3MB into a 1MB cap at least one backup must exist, and backups
	// hold real data (they are renamed originals, never empty).
	matches, _ := filepath.Glob(filepath.Join(tmpDir, "test_log_*.log"))
	if len(matches) == 0 {
		entries, _ := os.ReadDir(tmpDir)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("no backup files after 3MB into a 1MB cap; directory: %v", names)
	}
	for _, backup := range matches {
		info, err := os.Stat(backup)
		if err != nil {
			t.Errorf("stat backup %s: %v", backup, err)
		} else if info.Size() == 0 {
			t.Errorf("backup %s is empty", filepath.Base(backup))
		}
	}
}

func TestFileCompressionTrigger(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "compress.log")

	fw, err := NewFileWriter(logFile, FileWriterConfig{
		MaxSizeMB:  1,
		MaxBackups: 3,
		MaxAge:     24 * time.Hour,
		Compress:   true,
	})
	if err != nil {
		t.Fatalf("NewFileWriter() error = %v", err)
	}

	largeData := make([]byte, 1024*1024) // 1MB of zeros
	for i := 0; i < 3; i++ {
		if _, err := fw.Write(largeData); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	// Close waits for the background compression goroutine, so the .gz
	// backups must be on disk when it returns.
	if err := fw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(tmpDir, "compress_log_*.log.gz"))
	if len(matches) == 0 {
		entries, _ := os.ReadDir(tmpDir)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("no compressed backups after rotation with Compress=true; directory: %v", names)
	}
	for _, gzFile := range matches {
		gzInfo, err := os.Stat(gzFile)
		if err != nil {
			t.Errorf("stat %s: %v", gzFile, err)
			continue
		}
		// A 1MB run of zeros must compress to well under its original size.
		if gzInfo.Size() >= int64(len(largeData)) {
			t.Errorf("compressed %s is %d bytes, want < %d", filepath.Base(gzFile), gzInfo.Size(), len(largeData))
		}
	}
}

// ============================================================================
// JSON OPTIONS CUSTOMIZATION TESTS
// ============================================================================

func TestJSONOptionsCustomization(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Level = LevelInfo
	config.Format = FormatJSON
	config.Targets = []OutputTarget{CustomOutput(&buf)}
	config.JSON = &JSONOptions{
		PrettyPrint: true,
		Indent:      "  ",
		FieldNames: &JSONFieldNames{
			Timestamp: "time",
			Level:     "severity",
			Message:   "msg",
			Fields:    "data",
		},
	}
	logger, _ := New(config)

	logger.InfoWith("test", String("key", "value"))
	output := buf.String()

	var jsonData map[string]any
	if err := json.Unmarshal([]byte(output), &jsonData); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	// Check for custom field names
	if jsonData["time"] == nil && jsonData["timestamp"] != nil {
		t.Error("Custom timestamp field name 'time' not applied")
	}
	if jsonData["severity"] == nil && jsonData["level"] != nil {
		t.Error("Custom level field name 'severity' not applied")
	}
	if jsonData["msg"] == nil && jsonData["message"] != nil {
		t.Error("Custom message field name 'msg' not applied")
	}

	// At least verify the message was logged
	if jsonData["message"] == nil && jsonData["msg"] == nil {
		t.Error("Should have some message field")
	}
}

// ============================================================================
// DYNAMIC CALLER DETECTION TESTS
// ============================================================================

func TestDynamicCallerDetection(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	cfg.DynamicCaller = true
	cfg.FullPath = false
	logger, _ := New(cfg)

	logger.Info("test message")
	output := buf.String()

	if !strings.Contains(output, ".go:") {
		t.Error("Dynamic caller should include file:line")
	}
}

func TestFullPathCaller(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	cfg.DynamicCaller = true
	cfg.FullPath = true
	logger, _ := New(cfg)

	logger.Info("test message")
	output := buf.String()

	if !strings.Contains(output, "/") && !strings.Contains(output, "\\") {
		t.Error("FullPath should include directory separators")
	}
}

// ============================================================================
// CONCURRENT WRITER ADD/REMOVE TESTS
// ============================================================================

// TestConcurrentWriterAddRemove is covered by TestConcurrentAddRemoveWriter in dd_test.go

// threadSafeBuffer wraps bytes.Buffer with mutex for safe concurrent access
type threadSafeBuffer struct {
	mu sync.Mutex
	*bytes.Buffer
}

func (b *threadSafeBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

// ============================================================================
// ADDITIONAL EDGE CASES
// ============================================================================

// TestEmptyStructuredFields/TestVeryLongFieldName were removed: their rows
// live in dd_test.go TestEdgeShapedFields with message-contains assertions.

func TestSpecialCharactersInMessage(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	specialMsg := "Test\n\x00\x1f\x1b\"message\""
	logger.Info(specialMsg)

	output := buf.String()

	if strings.Contains(output, "\x00") || strings.Contains(output, "\x1f") {
		t.Error("Control characters should be sanitized")
	}

	if !strings.Contains(output, "Test") {
		t.Error("Printable content should remain")
	}
}

// ============================================================================
// FATAL LEVEL INTEGRATION TESTS
// ============================================================================

func TestFatalWithLoggingIntegration(t *testing.T) {
	var buf bytes.Buffer
	exited := false

	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	cfg.FatalHandler = func() { exited = true }
	logger, _ := New(cfg)

	logger.FatalWith("fatal", String("key", "value"))

	if !exited {
		t.Error("FatalWith should call fatal handler")
	}

	if !strings.Contains(buf.String(), "fatal") {
		t.Error("FatalWith should log message")
	}
}

// ============================================================================
// PACKAGE-LEVEL PRINT FUNCTIONS TESTS
// ============================================================================

func TestPackageLevelPrintFunctions(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func()
		expected string
	}{
		{
			name: "Print",
			logFunc: func() {
				Print("test", "print")
			},
			expected: "test print",
		},
		{
			name: "Println",
			logFunc: func() {
				Println("test", "println")
			},
			expected: "test println",
		},
		{
			name: "Printf",
			logFunc: func() {
				Printf("test %s", "formatted")
			},
			expected: "test formatted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := DefaultConfig()
			cfg.Targets = []OutputTarget{CustomOutput(&buf)}
			cfg.Level = LevelDebug

			logger, err := New(cfg)
			if err != nil {
				t.Fatalf("Failed to create logger: %v", err)
			}
			defer logger.Close()

			// Set as default logger
			oldDefault := Default()
			SetDefault(logger)
			defer SetDefault(oldDefault)

			tt.logFunc()

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("dd.%s() should contain %q, got: %s", tt.name, tt.expected, output)
			}
		})
	}
}

func TestPackageLevelPrintWithSecurityFilter(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelDebug
	cfg.Security = &SecurityConfig{
		SensitiveFilter: newBasicSensitiveDataFilter(),
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Set as default logger
	oldDefault := Default()
	SetDefault(logger)
	defer SetDefault(oldDefault)

	Print("password=secret123")

	output := buf.String()
	if strings.Contains(output, "secret123") {
		t.Error("Package-level Print should apply sensitive data filtering")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("Package-level Print should redact sensitive data")
	}
}
