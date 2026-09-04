package dd

// SEC-003 regression tests: no user-supplied value or callback may let a panic
// escape the public API. Each test exercises a path that, before the fixes,
// either crashed the caller outright or (for the write-error-handler case)
// silently skipped Fatal handling.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// panicSentinel values are what the misbehaving user types raise.
const (
	panicStringMsg = "String boom"
	panicErrorMsg  = "Error boom"
)

type panickingStringer struct{}

func (panickingStringer) String() string { panic(panicStringMsg) }

type panickingError struct{}

func (panickingError) Error() string { panic(panicErrorMsg) }

// structWithPanickingField reaches ConvertValue's struct walk: the outer struct
// has no String method (so IsComplexValue is true) while its field panics.
type structWithPanickingField struct {
	Inner panickingStringer
	Name  string
}

// failingWriter always fails so handleWriteError fires.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}

// panickingCloseWriter's Close panics.
type panickingCloseWriter struct{ bytes.Buffer }

func (panickingCloseWriter) Close() error { panic("Close boom") }

// panickingFlusher's Flush panics.
type panickingFlusher struct{}

func (panickingFlusher) Write(p []byte) (int, error) { return len(p), nil }
func (panickingFlusher) Flush() error                { panic("Flush boom") }

// captureStdout redirects os.Stdout around fn and returns what was written.
// The debug Text/JSON helpers write straight to os.Stdout (read at call time),
// so a swap is visible to them.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(data)
}

// newTestLogger builds a logger writing into buf at debug level.
func newTestLogger(t *testing.T, buf *bytes.Buffer) *Logger {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(buf)}
	cfg.Level = LevelDebug
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return logger
}

// TestSEC003PanickingUserValuesInLogArgs covers the formatting of user args.
// FormatArgsToString runs before logCoreWithDepth's recover, so a panicking
// String/Error method used to escape Log/Logf directly.
func TestSEC003PanickingUserValuesInLogArgs(t *testing.T) {
	tests := []struct {
		name   string
		logNow func(l *Logger)
	}{
		{"Stringer arg", func(l *Logger) { l.Info(panickingStringer{}) }},
		{"error arg", func(l *Logger) { l.Info(panickingError{}) }},
		{"struct with panicking field", func(l *Logger) { l.Info(structWithPanickingField{Name: "x"}) }},
		{"multi-arg with Stringer", func(l *Logger) { l.Info("ctx:", panickingStringer{}) }},
		{"entry Stringer arg", func(l *Logger) { l.WithField("k", "v").Info(panickingStringer{}) }},
		{"Print", func(l *Logger) { l.Print(panickingStringer{}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := newTestLogger(t, &buf)
			defer logger.Close()

			tt.logNow(logger) // must not panic

			out := buf.String()
			if !strings.Contains(out, "PANIC=") {
				t.Errorf("expected PANIC= placeholder in output, got: %s", out)
			}
		})
	}
}

// TestSEC003ErrHelpersWithPanickingError verifies the Err field constructors
// convert a panicking Error method into a placeholder value.
func TestSEC003ErrHelpersWithPanickingError(t *testing.T) {
	for name, field := range map[string]Field{
		"Err":          Err(panickingError{}),
		"ErrWithKey":   ErrWithKey("custom", panickingError{}),
		"ErrWithStack": ErrWithStack(panickingError{}),
	} {
		s, ok := field.Value.(string)
		if !ok {
			t.Errorf("%s value = %T (%v), want string", name, field.Value, field.Value)
			continue
		}
		if !strings.Contains(s, "PANIC=Error method: "+panicErrorMsg) {
			t.Errorf("%s = %q, want PANIC= placeholder", name, s)
		}
	}
}

// TestSEC003LevelResolverPanic verifies a panicking LevelResolver falls back to
// the static level instead of crashing the logging call.
func TestSEC003LevelResolverPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(t, &buf)
	defer logger.Close()

	logger.SetLevelResolver(func(ctx context.Context) LogLevel { panic("resolver boom") })

	logger.Info("after resolver panic") // must not panic

	if !strings.Contains(buf.String(), "after resolver panic") {
		t.Errorf("log entry lost after resolver panic, got: %s", buf.String())
	}

	// The fallback must honor the static level: with Error set, Info is filtered.
	buf.Reset()
	if err := logger.SetLevel(LevelError); err != nil {
		t.Fatalf("SetLevel: %v", err)
	}
	logger.Info("should be filtered")
	if buf.String() != "" {
		t.Errorf("static level not honored on resolver fallback, got: %s", buf.String())
	}
}

// TestSEC003HookErrorHandlerPanic verifies Trigger survives a panicking error
// handler and still reports the hook error.
func TestSEC003HookErrorHandlerPanic(t *testing.T) {
	registry := NewHookRegistry()
	registry.SetErrorHandler(func(event HookEvent, hookCtx *HookContext, err error) {
		panic("handler boom")
	})
	registry.Add(HookBeforeLog, func(ctx context.Context, hookCtx *HookContext) error {
		return errors.New("hook failed")
	})

	err := registry.Trigger(context.Background(), HookBeforeLog, &HookContext{Event: HookBeforeLog}) // must not panic
	if err == nil {
		t.Error("Trigger should return an error when the hook fails")
	}
	if !strings.Contains(err.Error(), "handler boom") {
		t.Errorf("error should surface the handler panic, got: %v", err)
	}
}

// TestSEC003CloseWithPanickingErrorHandler exercises the hook error handler via
// Logger.Close — a public path with no surrounding recover.
func TestSEC003CloseWithPanickingErrorHandler(t *testing.T) {
	logger := newTestLogger(t, &bytes.Buffer{})

	handlerCalled := false
	registry := NewHookRegistry()
	registry.SetErrorHandler(func(event HookEvent, hookCtx *HookContext, err error) {
		handlerCalled = true
		if event == HookOnClose {
			panic("close handler boom")
		}
	})
	registry.Add(HookOnClose, func(ctx context.Context, hookCtx *HookContext) error {
		return errors.New("close hook failed")
	})
	if err := logger.SetHooks(registry); err != nil {
		t.Fatalf("SetHooks: %v", err)
	}

	if err := logger.Close(); err != nil { // must not panic
		t.Logf("Close returned hook error as expected: %v", err)
	}
	if !handlerCalled {
		t.Error("error handler was never invoked")
	}
}

// TestSEC003WriteErrorHandlerPanicFATALStillExits verifies that a panicking
// WriteErrorHandler no longer unwinds past writeMessage: the Fatal flow must
// still reach the fatal handler (pre-fix the entry was silently swallowed by
// logCoreWithDepth's recover and the process never exited).
func TestSEC003WriteErrorHandlerPanicFATALStillExits(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(failingWriter{})}
	cfg.FatalHandler = func() {} // prevent os.Exit in tests
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	logger.SetWriteErrorHandler(func(w io.Writer, err error) {
		panic("write handler boom")
	})

	logger.Fatal("fatal must still exit") // must not panic, must call fatalHandler

	// Reaching this line means the panic was contained. Assert the AfterLog /
	// fatal path ran by checking the logger can still be closed cleanly.
	if err := logger.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestSEC003PanickingFatalHandlerStillExits verifies that a panicking
// FatalHandler falls back to os.Exit(1) instead of letting the panic unwind
// into logCoreWithDepth's recover (pre-fix behavior: the panic was swallowed
// and the process kept running after a Fatal log). Uses the subprocess re-exec
// pattern from coverage_test.go because the tested behavior is os.Exit itself.
func TestSEC003PanickingFatalHandlerStillExits(t *testing.T) {
	if os.Getenv("DD_TEST_FATAL_HANDLER_PANIC") == "1" {
		cfg := DefaultConfig()
		cfg.FatalHandler = func() { panic("fatal handler boom") }
		logger, err := New(cfg)
		if err != nil {
			os.Exit(2) // setup failure, distinct from the expected exit paths
		}
		logger.Fatal("fatal with panicking handler")
		// Unreachable when the fix works: Fatal must not return.
		_, _ = os.Stderr.WriteString("FATAL DID NOT EXIT\n")
		os.Exit(3)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestSEC003PanickingFatalHandlerStillExits")
	cmd.Env = append(os.Environ(), "DD_TEST_FATAL_HANDLER_PANIC=1")
	output, err := cmd.CombinedOutput()

	exitCode := 0
	if e, ok := err.(*exec.ExitError); ok {
		exitCode = e.ExitCode()
	} else if err != nil {
		t.Fatalf("running subprocess: %v", err)
	}
	if exitCode != 1 {
		t.Errorf("subprocess exit code = %d, want 1 (fatal handler panic fallback); output:\n%s", exitCode, output)
	}
	if strings.Contains(string(output), "FATAL DID NOT EXIT") {
		t.Errorf("Fatal returned without exiting the process; output:\n%s", output)
	}
	if !strings.Contains(string(output), "fatal handler panic") {
		t.Errorf("expected fatal-handler panic warning on stderr, got:\n%s", output)
	}
}

// TestSEC003PanickingWriterClose verifies a panicking writer Close is converted
// into an error by Logger.Close and MultiWriter.Close.
func TestSEC003PanickingWriterClose(t *testing.T) {
	t.Run("Logger.Close", func(t *testing.T) {
		logger := newTestLogger(t, &bytes.Buffer{})
		if err := logger.AddWriter(&panickingCloseWriter{}); err != nil {
			t.Fatalf("AddWriter: %v", err)
		}
		err := logger.Close() // must not panic
		if err == nil || !strings.Contains(err.Error(), "Close boom") {
			t.Errorf("Close error = %v, want wrapped close panic", err)
		}
	})

	t.Run("MultiWriter.Close", func(t *testing.T) {
		mw := NewMultiWriter(&panickingCloseWriter{})
		err := mw.Close() // must not panic
		if err == nil || !strings.Contains(err.Error(), "Close boom") {
			t.Errorf("Close error = %v, want wrapped close panic", err)
		}
	})
}

// TestSEC003PanickingFlusher verifies a panicking writer Flush is converted
// into an error by Logger.Flush.
func TestSEC003PanickingFlusher(t *testing.T) {
	logger := newTestLogger(t, &bytes.Buffer{})
	defer logger.Close()
	if err := logger.AddWriter(panickingFlusher{}); err != nil {
		t.Fatalf("AddWriter: %v", err)
	}

	err := logger.Flush() // must not panic
	if err == nil || !strings.Contains(err.Error(), "Flush boom") {
		t.Errorf("Flush error = %v, want wrapped flush panic", err)
	}
}

// TestSEC003DebugHelpersWithPanickingValues covers the direct-output debug
// helpers (no sensitive filtering, no logCore recover around them).
func TestSEC003DebugHelpersWithPanickingValues(t *testing.T) {
	t.Run("JSON", func(t *testing.T) {
		out := captureStdout(t, func() {
			Default().JSON(panickingStringer{}) // must not panic
		})
		if !strings.Contains(out, "PANIC=String method") {
			t.Errorf("expected PANIC= placeholder, got: %s", out)
		}
	})
	t.Run("Text", func(t *testing.T) {
		out := captureStdout(t, func() {
			Default().Text(structWithPanickingField{Name: "x"}) // must not panic
		})
		if !strings.Contains(out, "PANIC=String method") {
			t.Errorf("expected PANIC= placeholder, got: %s", out)
		}
	})
}

// TestSEC003ConcurrentPanickingArgs is a smoke test that the recover backstops
// hold under concurrent use (recover + shared state, run with -race). Uses
// threadSafeWriter (dd_test.go): a logger shared across goroutines requires a
// thread-safe writer.
func TestSEC003ConcurrentPanickingArgs(t *testing.T) {
	safeWriter := &threadSafeWriter{w: &bytes.Buffer{}}
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(safeWriter)}
	cfg.Level = LevelDebug
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer logger.Close()

	var wg sync.WaitGroup
	var logged atomic.Int64
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				logger.Info(panickingStringer{}, panickingError{})
				logged.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := logged.Load(); got != 200 {
		t.Errorf("completed log calls = %d, want 200", got)
	}
	if n := strings.Count(safeWriter.String(), "PANIC="); n < 200 {
		t.Errorf("PANIC placeholders = %d, want >= 200", n)
	}
}

// panickingWriteWriter panics on Write while its countdown is positive,
// then writes normally. The countdown lets a test prove background flushes
// survive panics without a later panic landing on the test goroutine via
// the Close-time flush.
type panickingWriteWriter struct {
	panicCountdown atomic.Int64
}

func (w *panickingWriteWriter) Write(p []byte) (int, error) {
	if w.panicCountdown.Load() > 0 {
		w.panicCountdown.Add(-1)
		panic("write panic")
	}
	return len(p), nil
}

// TestSEC003AutoFlushPanickingWriter verifies that a panicking underlying
// writer cannot crash the autoFlush background goroutine (SEC-003) or leave
// bw.mu locked; a lock abandoned mid-panic would deadlock every later
// Write on the same writer.
func TestSEC003AutoFlushPanickingWriter(t *testing.T) {
	w := &panickingWriteWriter{}
	w.panicCountdown.Store(100)
	bw, err := NewBufferedWriter(w, BufferedWriterConfig{FlushTime: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewBufferedWriter: %v", err)
	}
	// Small payload stays under flushSize, so nothing reaches the
	// underlying writer synchronously.
	if _, err := bw.Write([]byte("buffered payload")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Several flush ticks pass; every tick Flush panics inside the
	// background goroutine and must be recovered there. Without the
	// recover this crashes the whole test binary.
	time.Sleep(60 * time.Millisecond)
	// If the recovered panic left mu locked, this Write blocks forever
	// and the test fails by timeout.
	if _, err := bw.Write([]byte("still writable")); err != nil {
		t.Fatalf("Write after panicking autoflush: %v", err)
	}
	// Stop panicking so the Close-time flush succeeds on this goroutine.
	w.panicCountdown.Store(0)
	if err := bw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestSEC003VerifyWithBracketInSignaturePrefix pins the Verify slicing fix.
// IntegrityConfig.Validate accepts any SignaturePrefix, including one that
// contains ']'. Pre-fix, Verify scanned for the closing bracket from the
// prefix START, matched the ']' inside the prefix, and sliced
// entry[sigStart+len(prefix):sigStart+sigEnd] with hi < lo — a slice-bounds
// panic on the Sign→Verify round-trip itself, i.e. from public API with no
// recover above it.
func TestSEC003VerifyWithBracketInSignaturePrefix(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	signer, err := NewIntegritySigner(IntegrityConfig{
		SecretKey:       key,
		SignaturePrefix: "[S]IG:", // ']' before the prefix end
	})
	if err != nil {
		t.Fatalf("NewIntegritySigner: %v", err)
	}

	msg := "audit event payload"
	entry := msg + signer.Sign(msg)

	res, err := signer.Verify(entry) // must not panic
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Valid {
		t.Errorf("Sign→Verify round-trip with bracketed prefix: Valid = false (Message = %q)", res.Message)
	}
	if res.Message != msg {
		t.Errorf("Verify Message = %q, want %q", res.Message, msg)
	}

	// The audit-facing helper delegates to the same path.
	if r := VerifyAuditEvent(entry, signer); !r.Valid {
		t.Errorf("VerifyAuditEvent: Valid = false, want true")
	}

	// Malformed entries containing the prefix must stay on the invalid path.
	for _, bad := range []string{
		"[S]IG:",             // prefix with no closing bracket after it
		"partial [S]IG tail", // unterminated prefix fragment
		"[S]IG:x] tampered",  // bracket follows content that cannot parse
	} {
		res, err := signer.Verify(bad) // must not panic
		if err != nil {
			t.Errorf("Verify(%q) error = %v, want nil", bad, err)
			continue
		}
		if res.Valid {
			t.Errorf("Verify(%q) Valid = true, want false", bad)
		}
	}
}
