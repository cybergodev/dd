package dd

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/dd/internal"
)

// This file targets error paths and boundary conditions the main suites
// leave unexercised: rotation failure recovery, legacy integrity
// verification, lazy-message gating, constructor failure cleanup, and the
// nil/empty edges of small dispatch helpers.

// closeSpyWriter records Close calls so tests can assert that
// newFromInternalConfig does not leak writers added before a failure.
type closeSpyWriter struct {
	closed bool
}

func (w *closeSpyWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *closeSpyWriter) Close() error                { w.closed = true; return nil }

func TestVerifyLegacySignatures(t *testing.T) {
	signer, err := NewIntegritySigner(IntegrityConfig{
		SecretKey:       []byte("errorpath-boundary-key-0123456789a"),
		HashAlgorithm:   HashAlgorithmSHA256,
		SignaturePrefix: "[SIG:",
	})
	if err != nil {
		t.Fatalf("NewIntegritySigner() error = %v", err)
	}

	legacySig := func(msg string) string {
		return base64.RawURLEncoding.EncodeToString(signer.expectedSignature(msg, "", ""))
	}

	cases := []struct {
		name      string
		message   string
		sig       string
		wantValid bool
		wantMsg   string
	}{
		{
			name:      "undecodable signature is unverified, not an error",
			message:   "hello",
			sig:       "!!!not-base64!!!",
			wantValid: false,
			wantMsg:   "hello",
		},
		{
			name:      "valid legacy signature",
			message:   "hello",
			sig:       legacySig("hello"),
			wantValid: true,
			wantMsg:   "hello",
		},
		{
			name:      "separator space stripped before re-check",
			message:   "hello ",
			sig:       legacySig("hello"),
			wantValid: true,
			wantMsg:   "hello",
		},
		{
			name:      "signature over different message stays invalid",
			message:   "hello",
			sig:       legacySig("tampered"),
			wantValid: false,
			wantMsg:   "hello",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := signer.verifyLegacy(tc.message, tc.sig)
			if err != nil {
				t.Fatalf("verifyLegacy() error = %v", err)
			}
			if got.Valid != tc.wantValid {
				t.Errorf("Valid = %v, want %v", got.Valid, tc.wantValid)
			}
			if got.Message != tc.wantMsg {
				t.Errorf("Message = %q, want %q", got.Message, tc.wantMsg)
			}
		})
	}

	// End-to-end: Verify dispatches metadata-less signatures to the legacy
	// path. The one-space separator before [SIG: must not break verification.
	t.Run("Verify dispatches legacy format", func(t *testing.T) {
		entry := "hello [SIG:" + legacySig("hello") + "]"
		got, err := signer.Verify(entry)
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
		if !got.Valid {
			t.Errorf("Verify(legacy entry) Valid = false, want true")
		}
		if got.Message != "hello" {
			t.Errorf("Message = %q, want %q", got.Message, "hello")
		}
	})
}

func TestLogWithLazyMessageGate(t *testing.T) {
	t.Run("nil logger is a no-op", func(t *testing.T) {
		var l *Logger
		l.logWithLazyMessage(LevelInfo, func() string { return "boom" }, nil, "", 0)
	})

	t.Run("nil message func is a no-op", func(t *testing.T) {
		l, err := New()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		defer l.Close()
		l.logWithLazyMessage(LevelInfo, nil, nil, "", 0)
	})

	t.Run("gate rejection skips the lazy message", func(t *testing.T) {
		l, err := New(Config{Level: LevelWarn})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		defer l.Close()

		called := false
		l.logWithLazyMessage(LevelInfo, func() string {
			called = true
			return "lazy"
		}, nil, "", 0)
		if called {
			t.Error("message func invoked although the level gate rejects Info")
		}
	})

	t.Run("gate pass invokes the lazy message and writes it", func(t *testing.T) {
		var buf bytes.Buffer
		l, err := New(Config{
			Level:   LevelDebug,
			Targets: []OutputTarget{CustomOutput(&buf)},
		})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		defer l.Close()

		called := false
		l.logWithLazyMessage(LevelWarn, func() string {
			called = true
			return "lazy-df21"
		}, nil, "", 0)
		if !called {
			t.Error("message func not invoked although the level gate passes Warn")
		}
		if !strings.Contains(buf.String(), "lazy-df21") {
			t.Errorf("output %q does not contain the lazy message", buf.String())
		}
	})
}

func TestNewFromInternalConfigErrorPaths(t *testing.T) {
	t.Run("invalid audit config fails with wrapped error", func(t *testing.T) {
		cfg := defaultConfig().toInternalConfig()
		cfg.auditConfig = &AuditConfig{Enabled: true, BufferSize: -1}

		_, err := newFromInternalConfig(cfg)
		if err == nil {
			t.Fatal("newFromInternalConfig(invalid audit) = nil error, want error")
		}
		if !strings.Contains(err.Error(), "failed to initialize audit logger") {
			t.Errorf("error = %v, want it to mention the audit logger failure", err)
		}
	})

	t.Run("nil writer fails and closes writers added before it", func(t *testing.T) {
		spy := &closeSpyWriter{}
		cfg := defaultConfig().toInternalConfig()
		cfg.writers = []io.Writer{spy, nil}

		_, err := newFromInternalConfig(cfg)
		if !errors.Is(err, ErrNilWriter) {
			t.Fatalf("error = %v, want ErrNilWriter (wrapped)", err)
		}
		if !strings.Contains(err.Error(), "failed to add writer") {
			t.Errorf("error = %v, want it to mention the add-writer failure", err)
		}
		if !spy.closed {
			t.Error("writer added before the failure was not closed (leak)")
		}
	})
}

func TestFileWriterRotateRenameFailure(t *testing.T) {
	// A directory at the next backup path makes os.Rename fail on every
	// platform: POSIX refuses EISDIR, Windows refuses to replace a directory.
	blockBackup := func(t *testing.T, path string) string {
		backup := internal.GetBackupPath(path, 1, false)
		if err := os.Mkdir(backup, 0o750); err != nil {
			t.Fatalf("block backup path: %v", err)
		}
		return backup
	}

	newWriter := func(t *testing.T, path string) *FileWriter {
		fw, err := NewFileWriter(path, FileWriterConfig{MaxSizeMB: 1})
		if err != nil {
			t.Fatalf("NewFileWriter() error = %v", err)
		}
		if _, err := fw.Write([]byte("seed")); err != nil {
			t.Fatalf("seed write: %v", err)
		}
		return fw
	}

	t.Run("rename blocked, original reopened", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "app.log")
		fw := newWriter(t, path)
		defer fw.Close()
		blockBackup(t, path)

		err := fw.rotate()
		if err == nil || !strings.Contains(err.Error(), "rename to backup") {
			t.Fatalf("rotate() error = %v, want rename-to-backup failure", err)
		}
		// The writer must stay usable: it reopened the original file.
		if _, err := fw.Write([]byte("after")); err != nil {
			t.Errorf("post-failure Write() error = %v, want reopened file to accept writes", err)
		}
	})

	t.Run("rename blocked and reopen denied", func(t *testing.T) {
		if runtime.GOOS != "windows" && os.Geteuid() == 0 {
			t.Skip("root bypasses file permissions; read-only file cannot block reopen")
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "app.log")
		fw := newWriter(t, path)
		defer fw.Close()
		blockBackup(t, path)
		if err := os.Chmod(path, 0o444); err != nil {
			t.Fatalf("chmod read-only: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o666) })

		err := fw.rotate()
		if err == nil || !strings.Contains(err.Error(), "cannot reopen file") {
			t.Fatalf("rotate() error = %v, want reopen-denied failure", err)
		}
		// fw.file is nil after the failed reopen; Close must tolerate that.
		if err := fw.Close(); err != nil {
			t.Errorf("Close() after failed reopen error = %v", err)
		}
	})
}

func TestFileWriterCompressBackupMissingFile(t *testing.T) {
	dir := t.TempDir()
	fw, err := NewFileWriter(filepath.Join(dir, "app.log"), FileWriterConfig{MaxSizeMB: 1})
	if err != nil {
		t.Fatalf("NewFileWriter() error = %v", err)
	}
	defer fw.Close()

	// compressBackup is normally launched via wg.Add(1); mirror that when
	// calling it directly so its deferred Done does not underflow.
	fw.wg.Add(1)
	fw.compressBackup(filepath.Join(dir, "missing.log")) // must not panic
}

func TestIntegrityConfigMarshalJSONNilReceiver(t *testing.T) {
	var cfg *IntegrityConfig
	b, err := cfg.MarshalJSON()
	if err != nil {
		t.Fatalf("(*IntegrityConfig)(nil).MarshalJSON() error = %v", err)
	}
	if string(b) != "null" {
		t.Errorf("MarshalJSON() = %s, want null", b)
	}
}

func TestHandleWriteErrorHookDispatch(t *testing.T) {
	t.Run("no hooks registered", func(t *testing.T) {
		l, err := New()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		defer l.Close()
		l.handleWriteError(&bytes.Buffer{}, errors.New("write failed"))
	})

	t.Run("OnError hook receives error and writer", func(t *testing.T) {
		sentinel := errors.New("disk on fire")
		var gotErr error
		var gotWriter io.Writer

		registry := NewHookRegistry()
		registry.Add(HookOnError, func(_ context.Context, hc *HookContext) error {
			gotErr, gotWriter = hc.Error, hc.Writer
			return nil
		})
		l, err := New(Config{Hooks: registry})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		defer l.Close()

		wantWriter := &bytes.Buffer{}
		l.handleWriteError(wantWriter, sentinel)
		if !errors.Is(gotErr, sentinel) {
			t.Errorf("hook error = %v, want %v", gotErr, sentinel)
		}
		if gotWriter != wantWriter {
			t.Errorf("hook writer = %v, want the failing writer", gotWriter)
		}
	})
}

func TestContextExtractorRegistryCount(t *testing.T) {
	var nilReg *contextExtractorRegistry
	if got := nilReg.count(); got != 0 {
		t.Errorf("nil registry count() = %d, want 0", got)
	}

	extractor := func(context.Context) []Field { return nil }
	reg := newContextExtractorRegistry()
	if got := reg.count(); got != 0 {
		t.Errorf("empty registry count() = %d, want 0", got)
	}
	reg.Add(extractor)
	reg.Add(nil) // ignored, not stored
	reg.Add(extractor)
	if got := reg.count(); got != 2 {
		t.Errorf("count() = %d, want 2 (nil extractor must not count)", got)
	}
}

func TestShouldLogLevelBounds(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer l.Close()

	if l.shouldLog(LevelFatal + 1) {
		t.Error("shouldLog(above LevelFatal) = true, want false")
	}
}

func TestIsValidPrefixEmpty(t *testing.T) {
	if isValidPrefix("") {
		t.Error("isValidPrefix(\"\") = true, want false")
	}
}

func TestGetSecurityConfigZeroValueLogger(t *testing.T) {
	// A zero-value Logger carries no stored config; the getter must still
	// answer with defaults instead of nil.
	var l Logger
	if got := l.GetSecurityConfig(); got == nil {
		t.Error("GetSecurityConfig() on zero-value Logger = nil, want defaults")
	}
}

func TestAuditLoggerWriteEventNilOutput(t *testing.T) {
	al := &AuditLogger{config: &AuditConfig{}}
	al.writeEvent(AuditEvent{Type: AuditEventSecurityViolation}) // nil Output: no-op, no panic
}

func TestReplaceWithPatternCancelledContext(t *testing.T) {
	f := newSensitiveDataFilterWithPatterns(
		[]*regexp.Regexp{regexp.MustCompile(`secret`)}, nil, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	const input = "secret value"
	if got := f.replaceWithPatternWithContext(ctx, input, regexp.MustCompile(`secret`)); got != input {
		t.Errorf("replaceWithPatternWithContext(cancelled) = %q, want input unchanged", got)
	}
}
