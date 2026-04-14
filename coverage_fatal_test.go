package dd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// FATAL HANDLER TESTS
// ============================================================================

func TestFatalWithCustomHandler(t *testing.T) {
	called := make(chan bool, 1)
	cfg := DefaultConfig()
	cfg.Output = io.Discard
	cfg.FatalHandler = func() { called <- true }
	logger, _ := New(cfg)

	logger.Fatal("test message")

	select {
	case <-called:
		// Success - handler was called
	case <-time.After(time.Second):
		t.Error("FatalHandler not called")
	}
}

func TestFatalfWithCustomHandler(t *testing.T) {
	called := make(chan string, 1)
	cfg := DefaultConfig()
	cfg.Output = io.Discard
	cfg.FatalHandler = func() { called <- "called" }
	logger, _ := New(cfg)

	logger.Fatalf("test %s", "message")

	select {
	case msg := <-called:
		if msg != "called" {
			t.Errorf("Unexpected message: %s", msg)
		}
	case <-time.After(time.Second):
		t.Error("FatalHandler not called")
	}
}

func TestLoggerEntryFatal(t *testing.T) {
	tests := []struct {
		name   string
		call   func(*LoggerEntry)
	}{
		{"Fatal", func(e *LoggerEntry) { e.Fatal("entry fatal message") }},
		{"Fatalf", func(e *LoggerEntry) { e.Fatalf("entry fatalf %s", "message") }},
		{"FatalWith", func(e *LoggerEntry) { e.FatalWith("fatal with message", String("extra", "field")) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := make(chan bool, 1)
			cfg := DefaultConfig()
			cfg.Output = io.Discard
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

func TestDefaultHookErrorHandler(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	hookCtx := &HookContext{
		Event:   HookBeforeLog,
		Message: "test message",
	}
	testErr := errors.New("test hook error")

	defaultHookErrorHandler(HookBeforeLog, hookCtx, testErr)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "hook error") {
		t.Errorf("defaultHookErrorHandler should log error, got: %s", output)
	}
	if !strings.Contains(output, "BeforeLog") {
		t.Errorf("defaultHookErrorHandler should include event name, got: %s", output)
	}
}

func TestNewHookErrorRecorder(t *testing.T) {
	recorder := newHookErrorRecorder()
	if recorder == nil {
		t.Fatal("newHookErrorRecorder returned nil")
	}
	if recorder.count() != 0 {
		t.Error("New recorder should have no errors")
	}
}

func TestHookErrorRecorderHandler(t *testing.T) {
	recorder := newHookErrorRecorder()
	handler := recorder.handler()

	hookCtx := &HookContext{
		Event:   HookBeforeLog,
		Message: "test message",
	}
	testErr := errors.New("test error")

	handler(HookBeforeLog, hookCtx, testErr)

	if recorder.count() != 1 {
		t.Errorf("Expected 1 error, got %d", recorder.count())
	}

	errors := recorder.errors()
	if len(errors) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(errors))
	}
	if errors[0].Event != HookBeforeLog {
		t.Errorf("Expected event HookBeforeLog, got %v", errors[0].Event)
	}
	if errors[0].Message != "test message" {
		t.Errorf("Expected message 'test message', got %s", errors[0].Message)
	}
}

func TestHookErrorRecorderHandlerNilContext(t *testing.T) {
	recorder := newHookErrorRecorder()
	handler := recorder.handler()

	testErr := errors.New("test error")

	// Should not panic with nil context
	handler(HookAfterLog, nil, testErr)

	if recorder.count() != 1 {
		t.Errorf("Expected 1 error, got %d", recorder.count())
	}

	errors := recorder.errors()
	if errors[0].Message != "" {
		t.Errorf("Expected empty message for nil context, got %s", errors[0].Message)
	}
}

func TestHookErrorRecorderErrors(t *testing.T) {
	recorder := newHookErrorRecorder()
	handler := recorder.handler()

	// Add multiple errors
	handler(HookBeforeLog, &HookContext{Message: "msg1"}, errors.New("err1"))
	handler(HookAfterLog, &HookContext{Message: "msg2"}, errors.New("err2"))
	handler(HookOnError, &HookContext{Message: "msg3"}, errors.New("err3"))

	errors := recorder.errors()
	if len(errors) != 3 {
		t.Errorf("Expected 3 errors, got %d", len(errors))
	}

	// Verify it's a copy (modifying shouldn't affect original)
	errors[0] = hookErrorInfo{Event: HookOnClose}
	errors2 := recorder.errors()
	if errors2[0].Event == HookOnClose {
		t.Error("Errors() should return a copy")
	}
}

func TestHookErrorRecorderCount(t *testing.T) {
	recorder := newHookErrorRecorder()
	handler := recorder.handler()

	if recorder.count() != 0 {
		t.Error("New recorder should have count 0")
	}

	handler(HookBeforeLog, nil, errors.New("err1"))
	if recorder.count() != 1 {
		t.Errorf("Expected count 1, got %d", recorder.count())
	}

	handler(HookAfterLog, nil, errors.New("err2"))
	if recorder.count() != 2 {
		t.Errorf("Expected count 2, got %d", recorder.count())
	}
}

func TestHookErrorRecorderClear(t *testing.T) {
	recorder := newHookErrorRecorder()
	handler := recorder.handler()

	handler(HookBeforeLog, nil, errors.New("err1"))
	handler(HookAfterLog, nil, errors.New("err2"))

	if recorder.count() != 2 {
		t.Fatalf("Expected 2 errors before clear, got %d", recorder.count())
	}

	recorder.clear()

	if recorder.count() != 0 {
		t.Errorf("Expected 0 errors after clear, got %d", recorder.count())
	}
}

func TestHookErrorRecorderHasErrors(t *testing.T) {
	recorder := newHookErrorRecorder()
	handler := recorder.handler()

	if recorder.hasErrors() {
		t.Error("New recorder should not have errors")
	}

	handler(HookBeforeLog, nil, errors.New("err1"))

	if !recorder.hasErrors() {
		t.Error("Recorder should have errors after adding one")
	}

	recorder.clear()

	if recorder.hasErrors() {
		t.Error("Recorder should not have errors after clear")
	}
}

func TestHookRegistrySetErrorHandler(t *testing.T) {
	registry := NewHookRegistry()
	recorder := newHookErrorRecorder()
	registry.SetErrorHandler(recorder.handler())

	// Add a hook that always fails
	registry.Add(HookBeforeLog, func(ctx context.Context, hc *HookContext) error {
		return errors.New("hook error")
	})

	// Trigger the hook
	_ = registry.Trigger(context.Background(), HookBeforeLog, &HookContext{})

	if !recorder.hasErrors() {
		t.Error("Error handler should have been called")
	}
}

// ============================================================================
// ERROR TYPE METHOD TESTS
// ============================================================================

func TestWriterError(t *testing.T) {
	tests := []struct {
		name     string
		err      *WriterError
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "<nil WriterError>",
		},
		{
			name:     "with error",
			err:      &WriterError{Index: 0, Writer: io.Discard, Err: errors.New("write error")},
			expected: "writer[0]: write error",
		},
		{
			name:     "without error",
			err:      &WriterError{Index: 1, Writer: io.Discard, Err: nil},
			expected: "writer[1]: unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("Error() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestWriterErrorUnwrap(t *testing.T) {
	tests := []struct {
		name     string
		err      *WriterError
		expected error
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: nil,
		},
		{
			name:     "with wrapped error",
			err:      &WriterError{Index: 0, Writer: io.Discard, Err: errors.New("inner error")},
			expected: errors.New("inner error"),
		},
		{
			name:     "nil wrapped error",
			err:      &WriterError{Index: 0, Writer: io.Discard, Err: nil},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Unwrap()
			if tt.expected == nil {
				if result != nil {
					t.Errorf("Unwrap() = %v, want nil", result)
				}
			} else {
				if result == nil || result.Error() != tt.expected.Error() {
					t.Errorf("Unwrap() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestMultiWriterError(t *testing.T) {
	tests := []struct {
		name     string
		err      *MultiWriterError
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "empty errors",
			err:      &MultiWriterError{Errors: []WriterError{}},
			expected: "",
		},
		{
			name: "single error",
			err: &MultiWriterError{Errors: []WriterError{
				{Index: 0, Err: errors.New("single error")},
			}},
			expected: "writer[0]: single error",
		},
		{
			name: "multiple errors",
			err: &MultiWriterError{Errors: []WriterError{
				{Index: 0, Err: errors.New("error 1")},
				{Index: 1, Err: errors.New("error 2")},
			}},
			expected: "multiple writer errors:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if !strings.Contains(result, tt.expected) {
				t.Errorf("Error() = %q, should contain %q", result, tt.expected)
			}
		})
	}
}

func TestMultiWriterErrorUnwrap(t *testing.T) {
	tests := []struct {
		name     string
		err      *MultiWriterError
		expected int
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: 0,
		},
		{
			name:     "empty errors",
			err:      &MultiWriterError{Errors: []WriterError{}},
			expected: 0,
		},
		{
			name: "multiple errors",
			err: &MultiWriterError{Errors: []WriterError{
				{Index: 0, Err: errors.New("error 1")},
				{Index: 1, Err: errors.New("error 2")},
			}},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Unwrap()
			if len(result) != tt.expected {
				t.Errorf("Unwrap() returned %d errors, want %d", len(result), tt.expected)
			}
		})
	}
}

func TestMultiWriterErrorErrorCount(t *testing.T) {
	tests := []struct {
		name     string
		err      *MultiWriterError
		expected int
	}{
		{"nil error", nil, 0},
		{"empty errors", &MultiWriterError{Errors: []WriterError{}}, 0},
		{"single error", &MultiWriterError{Errors: []WriterError{{}}}, 1},
		{"multiple errors", &MultiWriterError{Errors: []WriterError{{}, {}}}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.ErrorCount()
			if result != tt.expected {
				t.Errorf("ErrorCount() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestMultiWriterErrorFirstError(t *testing.T) {
	tests := []struct {
		name     string
		err      *MultiWriterError
		hasError bool
	}{
		{"nil error", nil, false},
		{"empty errors", &MultiWriterError{Errors: []WriterError{}}, false},
		{"with error", &MultiWriterError{Errors: []WriterError{{Index: 0, Err: errors.New("first")}}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.FirstError()
			if tt.hasError && result == nil {
				t.Error("FirstError() should return an error")
			}
			if !tt.hasError && result != nil {
				t.Errorf("FirstError() should return nil, got %v", result)
			}
		})
	}
}

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
// LOGGER ERROR WITH FIELD TEST
// ============================================================================

func TestLoggerErrorWithField(t *testing.T) {
	err := newError(errCodeInvalidLevel, "invalid level")
	errWithField := err.WithField("key", "value")

	if errWithField == nil {
		t.Fatal("WithField returned nil")
	}

	if errWithField.Context["key"] != "value" {
		t.Errorf("WithField context key = %v, want 'value'", errWithField.Context["key"])
	}
}

func TestLoggerErrorIs(t *testing.T) {
	err := newError(errCodeInvalidLevel, "invalid level")

	if !errors.Is(err, ErrInvalidLevel) {
		t.Error("errors.Is should match sentinel error")
	}

	if errors.Is(err, ErrNilConfig) {
		t.Error("errors.Is should not match different sentinel error")
	}
}
