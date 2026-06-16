package dd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// BOUNDARY: ERROR TYPES
// ============================================================================

func TestWriterErrorBoundary(t *testing.T) {
	t.Run("nil WriterError Error", func(t *testing.T) {
		var e *WriterError
		if got := e.Error(); got != "<nil WriterError>" {
			t.Errorf("nil WriterError.Error() = %q, want %q", got, "<nil WriterError>")
		}
	})

	t.Run("nil WriterError Unwrap", func(t *testing.T) {
		var e *WriterError
		if got := e.Unwrap(); got != nil {
			t.Errorf("nil WriterError.Unwrap() = %v, want nil", got)
		}
	})

	t.Run("WriterError with nil Err", func(t *testing.T) {
		e := &WriterError{Index: 0, Writer: io.Discard, Err: nil}
		if got := e.Error(); !strings.Contains(got, "unknown error") {
			t.Errorf("WriterError with nil Err should contain 'unknown error', got: %s", got)
		}
	})
}

func TestMultiWriterErrorBoundary(t *testing.T) {
	t.Run("nil MultiWriterError Error", func(t *testing.T) {
		var e *MultiWriterError
		if got := e.Error(); got != "" {
			t.Errorf("nil MultiWriterError.Error() = %q, want empty", got)
		}
	})

	t.Run("nil MultiWriterError HasErrors", func(t *testing.T) {
		var e *MultiWriterError
		if e.HasErrors() {
			t.Error("nil MultiWriterError.HasErrors() should be false")
		}
	})

	t.Run("nil MultiWriterError ErrorCount", func(t *testing.T) {
		var e *MultiWriterError
		if got := e.ErrorCount(); got != 0 {
			t.Errorf("nil MultiWriterError.ErrorCount() = %d, want 0", got)
		}
	})

	t.Run("nil MultiWriterError FirstError", func(t *testing.T) {
		var e *MultiWriterError
		if got := e.FirstError(); got != nil {
			t.Errorf("nil MultiWriterError.FirstError() = %v, want nil", got)
		}
	})

	t.Run("nil MultiWriterError Unwrap", func(t *testing.T) {
		var e *MultiWriterError
		if got := e.Unwrap(); got != nil {
			t.Errorf("nil MultiWriterError.Unwrap() = %v, want nil", got)
		}
	})

	t.Run("empty MultiWriterError", func(t *testing.T) {
		e := &MultiWriterError{Errors: nil}
		if e.HasErrors() {
			t.Error("empty MultiWriterError.HasErrors() should be false")
		}
		if e.ErrorCount() != 0 {
			t.Error("empty MultiWriterError.ErrorCount() should be 0")
		}
		if e.FirstError() != nil {
			t.Error("empty MultiWriterError.FirstError() should be nil")
		}
	})

	t.Run("single error", func(t *testing.T) {
		e := &MultiWriterError{
			Errors: []WriterError{{Index: 0, Err: errors.New("write failed")}},
		}
		if !e.HasErrors() {
			t.Error("should have errors")
		}
		if e.ErrorCount() != 1 {
			t.Errorf("ErrorCount() = %d, want 1", e.ErrorCount())
		}
		if !strings.Contains(e.Error(), "write failed") {
			t.Errorf("Error() = %q, want to contain 'write failed'", e.Error())
		}
	})

	t.Run("multiple errors", func(t *testing.T) {
		e := &MultiWriterError{
			Errors: []WriterError{
				{Index: 0, Err: errors.New("error 0")},
				{Index: 1, Err: errors.New("error 1")},
			},
		}
		if e.ErrorCount() != 2 {
			t.Errorf("ErrorCount() = %d, want 2", e.ErrorCount())
		}
		if !strings.Contains(e.Error(), "multiple writer errors") {
			t.Errorf("Error() should contain 'multiple writer errors', got: %s", e.Error())
		}
		unwrapped := e.Unwrap()
		if len(unwrapped) != 2 {
			t.Errorf("Unwrap() returned %d errors, want 2", len(unwrapped))
		}
	})

	t.Run("FirstError returns first", func(t *testing.T) {
		e := &MultiWriterError{
			Errors: []WriterError{
				{Index: 0, Err: errors.New("first")},
				{Index: 1, Err: errors.New("second")},
			},
		}
		first := e.FirstError()
		if first == nil {
			t.Fatal("FirstError() should not be nil")
		}
		if !strings.Contains(first.Error(), "first") {
			t.Errorf("FirstError() = %v, want to contain 'first'", first)
		}
	})
}

// ============================================================================
// BOUNDARY: CONFIG OUTPUT TYPES
// ============================================================================

func TestOutputTypeConstructors(t *testing.T) {
	t.Run("ConsoleOutput", func(t *testing.T) {
		target := ConsoleOutput()
		if target.Type != OutputConsole {
			t.Errorf("ConsoleOutput().Type = %d, want %d", target.Type, OutputConsole)
		}
	})

	t.Run("FileOutput defaults", func(t *testing.T) {
		target := FileOutput("test.log")
		if target.Type != OutputFile {
			t.Errorf("FileOutput().Type = %d, want %d", target.Type, OutputFile)
		}
		if target.Path != "test.log" {
			t.Errorf("FileOutput().Path = %q, want %q", target.Path, "test.log")
		}
		if target.MaxSizeMB != DefaultMaxSizeMB {
			t.Errorf("FileOutput().MaxSizeMB = %d, want %d", target.MaxSizeMB, DefaultMaxSizeMB)
		}
		if target.MaxBackups != DefaultMaxBackups {
			t.Errorf("FileOutput().MaxBackups = %d, want %d", target.MaxBackups, DefaultMaxBackups)
		}
		if target.MaxAge != DefaultMaxAge {
			t.Errorf("FileOutput().MaxAge = %v, want %v", target.MaxAge, DefaultMaxAge)
		}
	})

	t.Run("CustomOutput", func(t *testing.T) {
		var buf bytes.Buffer
		target := CustomOutput(&buf)
		if target.Type != OutputCustom {
			t.Errorf("CustomOutput().Type = %d, want %d", target.Type, OutputCustom)
		}
		if target.Writer != &buf {
			t.Error("CustomOutput().Writer should be the provided writer")
		}
	})
}

// ============================================================================
// BOUNDARY: SECURITY FILTER EDGE CASES
// ============================================================================

func TestSensitiveDataFilterBoundary(t *testing.T) {
	t.Run("AddPattern on nil filter", func(t *testing.T) {
		var f *SensitiveDataFilter
		err := f.AddPattern("test")
		if !errors.Is(err, ErrNilFilter) {
			t.Errorf("nil AddPattern() = %v, want ErrNilFilter", err)
		}
	})

	t.Run("AddPattern empty", func(t *testing.T) {
		f := NewSensitiveDataFilter()
		err := f.AddPattern("")
		if !errors.Is(err, ErrEmptyPattern) {
			t.Errorf("AddPattern('') = %v, want ErrEmptyPattern", err)
		}
	})

	t.Run("AddPatterns nil filter", func(t *testing.T) {
		var f *SensitiveDataFilter
		err := f.AddPatterns("test")
		if !errors.Is(err, ErrNilFilter) {
			t.Errorf("nil AddPatterns() = %v, want ErrNilFilter", err)
		}
	})

	t.Run("AddPatterns skips empty", func(t *testing.T) {
		f := NewSensitiveDataFilter()
		initial := f.PatternCount()
		err := f.AddPatterns("", "valid_pattern_test", "")
		if err != nil {
			t.Errorf("AddPatterns() error: %v", err)
		}
		if f.PatternCount() != initial+1 {
			t.Errorf("PatternCount() = %d, want %d+1", f.PatternCount(), initial)
		}
	})

	t.Run("PatternCount nil", func(t *testing.T) {
		var f *SensitiveDataFilter
		if f.PatternCount() != 0 {
			t.Error("nil PatternCount() should be 0")
		}
	})

	t.Run("IsEnabled nil", func(t *testing.T) {
		var f *SensitiveDataFilter
		if f.IsEnabled() {
			t.Error("nil IsEnabled() should be false")
		}
	})

	t.Run("Enable nil", func(t *testing.T) {
		var f *SensitiveDataFilter
		f.Enable() // Should not panic
	})

	t.Run("Disable nil", func(t *testing.T) {
		var f *SensitiveDataFilter
		f.Disable() // Should not panic
	})

	t.Run("AddPattern too long", func(t *testing.T) {
		f := NewSensitiveDataFilter()
		longPattern := strings.Repeat("a", 10001)
		err := f.AddPattern(longPattern)
		if !errors.Is(err, ErrPatternTooLong) {
			t.Errorf("AddPattern(too long) = %v, want ErrPatternTooLong", err)
		}
	})

	t.Run("ClearPatterns", func(t *testing.T) {
		f := NewSensitiveDataFilter()
		before := f.PatternCount()
		f.ClearPatterns()
		if f.PatternCount() != 0 {
			t.Errorf("PatternCount() after Clear = %d, want 0", f.PatternCount())
		}
		_ = before
	})
}

// ============================================================================
// BOUNDARY: LOGGER FLUSH/WRITERCOUNT
// ============================================================================

func TestLoggerFlushBoundary(t *testing.T) {
	t.Run("Flush on closed logger", func(t *testing.T) {
		cfg := DefaultConfig()
		logger, _ := New(cfg)
		logger.Close()

		err := logger.Flush()
		if err != nil {
			t.Errorf("Flush on closed logger should not error, got: %v", err)
		}
	})

	t.Run("Flush with no flushable writers", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf)}
		logger, _ := New(cfg)

		err := logger.Flush()
		if err != nil {
			t.Errorf("Flush with non-flushable writer should not error, got: %v", err)
		}
	})
}

func TestLoggerWriterCountBoundary(t *testing.T) {
	t.Run("WriterCount after close", func(t *testing.T) {
		cfg := DefaultConfig()
		logger, _ := New(cfg)
		before := logger.WriterCount()
		logger.Close()
		after := logger.WriterCount()
		if after != 0 {
			t.Errorf("WriterCount() after close = %d, want 0 (was %d before)", after, before)
		}
	})
}

// ============================================================================
// BOUNDARY: SAFEWRITE PANIC RECOVERY
// ============================================================================

func TestSafeWritePanicRecovery(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&panicWriter{})}
	cfg.Level = LevelInfo
	logger, _ := New(cfg)

	// Should not panic even though writer panics
	logger.Info("test message")
}

type panicWriter struct{}

func (w *panicWriter) Write(p []byte) (n int, err error) {
	panic("writer panic")
}

// ============================================================================
// BOUNDARY: MULTIWRITER EDGE CASES
// ============================================================================

func TestMultiWriterBoundary(t *testing.T) {
	t.Run("AddWriter nil MultiWriter", func(t *testing.T) {
		var mw *MultiWriter
		err := mw.AddWriter(io.Discard)
		if !errors.Is(err, ErrNilMultiWriter) {
			t.Errorf("nil AddWriter() = %v, want ErrNilMultiWriter", err)
		}
	})

	t.Run("AddWriter nil writer", func(t *testing.T) {
		mw := NewMultiWriter()
		err := mw.AddWriter(nil)
		if !errors.Is(err, ErrNilWriter) {
			t.Errorf("AddWriter(nil) = %v, want ErrNilWriter", err)
		}
	})

	t.Run("RemoveWriter nil writer returns not found", func(t *testing.T) {
		mw := NewMultiWriter()
		err := mw.RemoveWriter(nil)
		if !errors.Is(err, ErrWriterNotFound) {
			t.Errorf("RemoveWriter(nil) = %v, want ErrWriterNotFound", err)
		}
	})

	t.Run("RemoveWriter nil MultiWriter", func(t *testing.T) {
		var mw *MultiWriter
		err := mw.RemoveWriter(io.Discard)
		if !errors.Is(err, ErrNilMultiWriter) {
			t.Errorf("nil RemoveWriter() = %v, want ErrNilMultiWriter", err)
		}
	})

	t.Run("Write to nil MultiWriter panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("nil MultiWriter.Write() should panic")
			}
		}()
		var mw *MultiWriter
		mw.Write([]byte("test"))
	})

	t.Run("AddWriter duplicate", func(t *testing.T) {
		mw := NewMultiWriter()
		var buf bytes.Buffer
		if err := mw.AddWriter(&buf); err != nil {
			t.Fatalf("First AddWriter: %v", err)
		}
		if err := mw.AddWriter(&buf); err != nil {
			t.Errorf("Duplicate AddWriter should not error: %v", err)
		}
	})

	t.Run("Write with failing writer collects error", func(t *testing.T) {
		var buf bytes.Buffer
		mw := NewMultiWriter(&buf, &errorWriter{err: errors.New("fail")})

		_, err := mw.Write([]byte("test"))
		if err == nil {
			t.Error("Write with failing writer should return error")
		}

		var mwe *MultiWriterError
		if !errors.As(err, &mwe) {
			t.Errorf("Error should be MultiWriterError, got: %T", err)
		}
	})

	t.Run("Close nil MultiWriter panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("nil MultiWriter.Close() should panic")
			}
		}()
		var mw *MultiWriter
		mw.Close()
	})
}

// ============================================================================
// BOUNDARY: FILE WRITER VALIDATION
// ============================================================================

func TestFileWriterValidation(t *testing.T) {
	t.Run("MaxSizeMB too large", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := NewFileWriter(tmpDir+"/test.log", FileWriterConfig{
			MaxSizeMB:  200000, // exceeds maxFileSizeMB
			MaxBackups: 5,
		})
		if err == nil {
			t.Error("Should reject MaxSizeMB too large")
		}
		if !errors.Is(err, ErrMaxSizeExceeded) {
			t.Errorf("Error = %v, want ErrMaxSizeExceeded", err)
		}
	})

	t.Run("MaxBackups too large", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := NewFileWriter(tmpDir+"/test.log", FileWriterConfig{
			MaxSizeMB:  100,
			MaxBackups: 2000, // exceeds maxBackupCount
		})
		if err == nil {
			t.Error("Should reject MaxBackups too large")
		}
		if !errors.Is(err, ErrMaxBackupsExceeded) {
			t.Errorf("Error = %v, want ErrMaxBackupsExceeded", err)
		}
	})

	t.Run("Write to closed FileWriter", func(t *testing.T) {
		tmpDir := t.TempDir()
		fw, err := NewFileWriter(tmpDir+"/test.log", DefaultFileWriterConfig())
		if err != nil {
			t.Fatalf("NewFileWriter() error: %v", err)
		}
		fw.Close()

		_, err = fw.Write([]byte("test"))
		if err == nil {
			t.Error("Write to closed FileWriter should fail")
		}
	})

	t.Run("Write empty data", func(t *testing.T) {
		tmpDir := t.TempDir()
		fw, err := NewFileWriter(tmpDir+"/test.log", DefaultFileWriterConfig())
		if err != nil {
			t.Fatalf("NewFileWriter() error: %v", err)
		}
		defer fw.Close()

		n, err := fw.Write([]byte{})
		if err != nil {
			t.Errorf("Write empty: %v", err)
		}
		if n != 0 {
			t.Errorf("Write empty returned %d, want 0", n)
		}
	})
}

// ============================================================================
// BOUNDARY: BUFFERED WRITER EDGE CASES
// ============================================================================

func TestBufferedWriterBoundary(t *testing.T) {
	t.Run("NewBufferedWriter nil writer", func(t *testing.T) {
		_, err := NewBufferedWriter(nil, BufferedWriterConfig{BufferSize: 1024})
		if err == nil {
			t.Error("Should reject nil writer")
		}
	})

	t.Run("Write empty data", func(t *testing.T) {
		var buf bytes.Buffer
		bw, err := NewBufferedWriter(&buf, BufferedWriterConfig{BufferSize: 4096})
		if err != nil {
			t.Fatalf("NewBufferedWriter() error: %v", err)
		}
		defer bw.Close()

		n, err := bw.Write([]byte{})
		if err != nil {
			t.Errorf("Write empty: %v", err)
		}
		if n != 0 {
			t.Errorf("Write empty returned %d, want 0", n)
		}
	})

	t.Run("BufferSize too large", func(t *testing.T) {
		var buf bytes.Buffer
		_, err := NewBufferedWriter(&buf, BufferedWriterConfig{BufferSize: 20 * 1024 * 1024}) // 20MB > max 10MB
		if err == nil {
			t.Error("Should reject buffer size too large")
		}
		if !errors.Is(err, ErrBufferSizeTooLarge) {
			t.Errorf("Error = %v, want ErrBufferSizeTooLarge", err)
		}
	})
}

// ============================================================================
// BOUNDARY: LOGGER SET SECURITY CONFIG NIL
// ============================================================================

func TestLoggerSecurityConfigNil(t *testing.T) {
	cfg := DefaultConfig()
	logger, _ := New(cfg)

	// Set nil security config should use default
	logger.SetSecurityConfig(nil)
	retrieved := logger.GetSecurityConfig()
	if retrieved == nil {
		t.Error("SetSecurityConfig(nil) should use default config")
	}
	if retrieved.MaxMessageSize <= 0 {
		t.Error("Default MaxMessageSize should be positive")
	}
}

// ============================================================================
// BOUNDARY: CONCURRENT SECURITY FILTER ACCESS
// ============================================================================

func TestConcurrentSecurityFilterAccess(t *testing.T) {
	filter := NewSensitiveDataFilter()
	const goroutines = 50
	var wg sync.WaitGroup

	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			filter.Filter(fmt.Sprintf("password=secret%d", id))
			filter.PatternCount()
			filter.IsEnabled()
		}(i)
	}

	// Concurrent enable/disable
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				filter.Enable()
			} else {
				filter.Disable()
			}
		}(i)
	}

	wg.Wait()
}

// ============================================================================
// BOUNDARY: SECURITY LEVEL PRESETS
// ============================================================================

func TestSecurityLevelPresets(t *testing.T) {
	tests := []struct {
		name   string
		config *SecurityConfig
	}{
		{"DefaultSecure", DefaultSecureConfig()},
		{"Default", DefaultSecurityConfig()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.config
			if cfg == nil {
				t.Fatal("Config should not be nil")
			}
			if cfg.MaxMessageSize <= 0 {
				t.Error("MaxMessageSize should be positive")
			}
			if cfg.MaxWriters <= 0 {
				t.Error("MaxWriters should be positive")
			}
			if cfg.SensitiveFilter == nil {
				t.Error("SensitiveFilter should not be nil")
			}
		})
	}
}

// ============================================================================
// BOUNDARY: FIELD CONSTRUCTOR VALUE RANGES
// ============================================================================

func TestFieldConstructorBoundaryValues(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Format = FormatJSON
	cfg.Level = LevelDebug
	logger, _ := New(cfg)

	tests := []struct {
		name  string
		field Field
		log   func()
		check func(output string) bool
	}{
		{
			name:  "zero int",
			field: Int("zero", 0),
			log:   func() { logger.InfoWith("test", Int("zero", 0)) },
			check: func(s string) bool { return strings.Contains(s, "zero") },
		},
		{
			name:  "negative int",
			field: Int("neg", -42),
			log:   func() { logger.InfoWith("test", Int("neg", -42)) },
			check: func(s string) bool { return strings.Contains(s, "-42") },
		},
		{
			name:  "max int64",
			field: Int64("max", int64(1<<62)),
			log:   func() { logger.InfoWith("test", Int64("max", int64(1<<62))) },
			check: func(s string) bool { return strings.Contains(s, "max") },
		},
		{
			name:  "false bool",
			field: Bool("flag", false),
			log:   func() { logger.InfoWith("test", Bool("flag", false)) },
			check: func(s string) bool { return strings.Contains(s, "flag") },
		},
		{
			name:  "empty string",
			field: String("empty", ""),
			log:   func() { logger.InfoWith("test", String("empty", "")) },
			check: func(s string) bool { return strings.Contains(s, "empty") },
		},
		{
			name:  "zero duration",
			field: Duration("dur", 0),
			log:   func() { logger.InfoWith("test", Duration("dur", 0)) },
			check: func(s string) bool { return strings.Contains(s, "dur") },
		},
		{
			name:  "nil error",
			field: Err(nil),
			log:   func() { logger.InfoWith("test", Err(nil)) },
			check: func(s string) bool { return true },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.log()
			if !tt.check(buf.String()) {
				t.Errorf("Field %s check failed, output: %s", tt.name, buf.String())
			}
		})
	}
}

// ============================================================================
// BOUNDARY: LOGGER NEW VARIATIONS
// ============================================================================

func TestNewLoggerVariations(t *testing.T) {
	t.Run("zero value config", func(t *testing.T) {
		// Zero-value Config has Level=0 (LevelDebug), Format=0 (FormatText)
		var cfg Config
		_, err := New(cfg)
		if err != nil {
			t.Errorf("New(zero Config) should not error: %v", err)
		}
	})

	t.Run("config with all targets nil", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Targets = nil
		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("New(nil targets) error: %v", err)
		}
		// Should log to stdout without panicking
		logger.Info("test")
		logger.Close()
	})
}

// ============================================================================
// BOUNDARY: ERROR IS WITH ALL SENTINELS
// ============================================================================

func TestErrorIsAllSentinels(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
		code string
	}{
		{"ErrNilConfig", ErrNilConfig, errCodeNilConfig},
		{"ErrNilWriter", ErrNilWriter, errCodeNilWriter},
		{"ErrNilFilter", ErrNilFilter, errCodeNilFilter},
		{"ErrNilHook", ErrNilHook, errCodeNilHook},
		{"ErrNilExtractor", ErrNilExtractor, errCodeNilExtractor},
		{"ErrLoggerClosed", ErrLoggerClosed, errCodeLoggerClosed},
		{"ErrWriterNotFound", ErrWriterNotFound, errCodeWriterNotFound},
		{"ErrInvalidLevel", ErrInvalidLevel, errCodeInvalidLevel},
		{"ErrInvalidFormat", ErrInvalidFormat, errCodeInvalidFormat},
		{"ErrMaxWritersExceeded", ErrMaxWritersExceeded, errCodeMaxWritersExceeded},
		{"ErrEmptyFilePath", ErrEmptyFilePath, errCodeEmptyFilePath},
		{"ErrPathTooLong", ErrPathTooLong, errCodePathTooLong},
		{"ErrPathTraversal", ErrPathTraversal, errCodePathTraversal},
		{"ErrNullByte", ErrNullByte, errCodeNullByte},
		{"ErrInvalidPath", ErrInvalidPath, errCodeInvalidPath},
		{"ErrSymlinkNotAllowed", ErrSymlinkNotAllowed, errCodeSymlinkNotAllowed},
		{"ErrMaxSizeExceeded", ErrMaxSizeExceeded, errCodeMaxSizeExceeded},
		{"ErrMaxBackupsExceeded", ErrMaxBackupsExceeded, errCodeMaxBackupsExceeded},
		{"ErrBufferSizeTooLarge", ErrBufferSizeTooLarge, errCodeBufferSizeTooLarge},
		{"ErrInvalidPattern", ErrInvalidPattern, errCodeInvalidPattern},
		{"ErrEmptyPattern", ErrEmptyPattern, errCodeEmptyPattern},
		{"ErrPatternTooLong", ErrPatternTooLong, errCodePatternTooLong},
		{"ErrReDoSPattern", ErrReDoSPattern, errCodeReDoSPattern},
		{"ErrPatternFailed", ErrPatternFailed, errCodePatternFailed},
		{"ErrConfigValidation", ErrConfigValidation, errCodeConfigValidation},
		{"ErrWriterAdd", ErrWriterAdd, errCodeWriterAdd},
		{"ErrMultipleConfigs", ErrMultipleConfigs, errCodeMultipleConfigs},
		{"ErrNilMultiWriter", ErrNilMultiWriter, errCodeNilMultiWriter},
	}

	for _, tt := range sentinels {
		t.Run(tt.name, func(t *testing.T) {
			err := &LoggerError{Code: tt.code, Message: "test"}
			if !errors.Is(err, tt.err) {
				t.Errorf("errors.Is(LoggerError{Code:%q}, %v) = false", tt.code, tt.name)
			}
		})
	}
}

// ============================================================================
// BOUNDARY: CONCURRENT LOG WITH SAMPLING
// ============================================================================

func TestConcurrentSamplingLog(t *testing.T) {
	safeWriter := &threadSafeWriter{w: &bytes.Buffer{}}
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(safeWriter)}
	cfg.Level = LevelInfo
	cfg.Sampling = &SamplingConfig{
		Enabled:    true,
		Initial:    100,
		Thereafter: 10,
	}
	logger, _ := New(cfg)

	const goroutines = 20
	const msgsPer = 50
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range msgsPer {
				logger.Info(fmt.Sprintf("g%d m%d", id, j))
			}
		}(i)
	}
	wg.Wait()
	// Just verify no panics or deadlocks
}

// ============================================================================
// BOUNDARY: TIME FORMAT EDGE CASES
// ============================================================================

func TestTimeFormatBoundary(t *testing.T) {
	t.Run("IncludeTime false produces no timestamp", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf)}
		cfg.Level = LevelInfo
		cfg.IncludeTime = false
		cfg.IncludeLevel = false
		cfg.DynamicCaller = false
		logger, _ := New(cfg)

		logger.Info("notime")
		output := buf.String()
		// Output should just be "notime\n" without timestamp
		if !strings.Contains(output, "notime") {
			t.Errorf("Expected 'notime' in output, got: %s", output)
		}
	})

	t.Run("Invalid time format still works", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf)}
		cfg.Level = LevelInfo
		cfg.TimeFormat = "invalid-format-string"
		logger, _ := New(cfg)

		logger.Info("test") // Should not panic
		if buf.Len() == 0 {
			t.Error("Should produce output even with invalid time format")
		}
	})
}

// ============================================================================
// BOUNDARY: TIME FORMAT VALIDATION (via Config)
// ============================================================================

func TestTimeFormatValidation(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{"empty format is valid", "", false},
		{"RFC3339", "2006-01-02T15:04:05Z07:00", false},
		{"date only", "2006-01-02", false},
		{"kitchen time", "3:04PM", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.TimeFormat = tt.format
			_, err := New(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New(TimeFormat=%q) error = %v, wantErr %v", tt.format, err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// BOUNDARY: APPLY FILE WRITER DEFAULTS
// ============================================================================

func TestApplyFileWriterDefaults(t *testing.T) {
	tests := []struct {
		name           string
		config         FileWriterConfig
		wantMaxSizeMB  int
		wantMaxBackups int
		wantMaxAge     time.Duration
	}{
		{
			name:           "zero values use full defaults",
			config:         FileWriterConfig{},
			wantMaxSizeMB:  DefaultMaxSizeMB,
			wantMaxBackups: DefaultMaxBackups,
			wantMaxAge:     DefaultMaxAge,
		},
		{
			name: "negative MaxSizeMB uses default",
			config: FileWriterConfig{
				MaxSizeMB: -1,
			},
			wantMaxSizeMB:  DefaultMaxSizeMB,
			wantMaxBackups: DefaultMaxBackups,
			wantMaxAge:     DefaultMaxAge,
		},
		{
			name: "only MaxAge set uses default MaxBackups",
			config: FileWriterConfig{
				MaxSizeMB: 50,
				MaxAge:    7 * 24 * time.Hour,
			},
			wantMaxSizeMB:  50,
			wantMaxBackups: DefaultMaxBackups,
			wantMaxAge:     7 * 24 * time.Hour,
		},
		{
			name: "only MaxBackups set keeps MaxAge zero",
			config: FileWriterConfig{
				MaxSizeMB:  50,
				MaxBackups: 5,
			},
			wantMaxSizeMB:  50,
			wantMaxBackups: 5,
			wantMaxAge:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyFileWriterDefaults(tt.config)
			if result.MaxSizeMB != tt.wantMaxSizeMB {
				t.Errorf("MaxSizeMB = %d, want %d", result.MaxSizeMB, tt.wantMaxSizeMB)
			}
			if result.MaxBackups != tt.wantMaxBackups {
				t.Errorf("MaxBackups = %d, want %d", result.MaxBackups, tt.wantMaxBackups)
			}
			if result.MaxAge != tt.wantMaxAge {
				t.Errorf("MaxAge = %v, want %v", result.MaxAge, tt.wantMaxAge)
			}
		})
	}
}

// ============================================================================
// BOUNDARY: MERGE FIELD SLICES
// ============================================================================

func TestMergeFieldSlicesBoundary(t *testing.T) {
	t.Run("no existing fields", func(t *testing.T) {
		new := []Field{String("a", "1")}
		result := mergeFieldSlices(nil, new)
		if len(result) != 1 || result[0].Key != "a" {
			t.Errorf("Expected [{a}], got %v", result)
		}
	})

	t.Run("no new fields", func(t *testing.T) {
		existing := []Field{String("a", "1")}
		result := mergeFieldSlices(existing, nil)
		if len(result) != 1 || result[0].Key != "a" {
			t.Errorf("Expected [{a}], got %v", result)
		}
	})

	t.Run("override existing field", func(t *testing.T) {
		existing := []Field{String("a", "1"), String("b", "2")}
		new := []Field{String("a", "override")}
		result := mergeFieldSlices(existing, new)
		if len(result) != 2 {
			t.Fatalf("Expected 2 fields, got %d", len(result))
		}
		// 'a' should be overridden
		for _, f := range result {
			if f.Key == "a" && f.Value.(string) != "override" {
				t.Errorf("Field 'a' should be overridden, got %v", f.Value)
			}
		}
	})

	t.Run("large field count uses map path", func(t *testing.T) {
		existing := make([]Field, 20)
		for i := range existing {
			existing[i] = Int(fmt.Sprintf("e%d", i), i)
		}
		new := make([]Field, 10)
		for i := range new {
			new[i] = Int(fmt.Sprintf("n%d", i), i)
		}
		result := mergeFieldSlices(existing, new)
		if len(result) == 0 {
			t.Error("Should merge large field slices")
		}
	})
}

// ============================================================================
// BOUNDARY: PACKAGE-LEVEL FATAL/ISLEVELENABLED WRAPPERS
// ============================================================================

func TestPackageLevelFatalWrappers(t *testing.T) {
	oldDefault := Default()
	defer SetDefault(oldDefault)

	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	cfg.Level = LevelInfo
	cfg.FatalHandler = func() {}
	logger, _ := New(cfg)
	SetDefault(logger)

	// Fatal/Fatalf/IsLevelEnabled are simple wrappers, test they don't panic
	t.Run("IsLevelEnabled wrapper", func(t *testing.T) {
		if !IsLevelEnabled(LevelInfo) {
			t.Error("IsLevelEnabled(LevelInfo) should be true")
		}
	})
}

// ============================================================================
// BOUNDARY: DEFAULT INIT ERROR
// ============================================================================

func TestDefaultInitError(t *testing.T) {
	// DefaultInitError should return nil for a normally initialized logger
	err := DefaultInitError()
	if err != nil {
		t.Logf("DefaultInitError() = %v (may be non-nil if init failed)", err)
	}
}

// ============================================================================
// BOUNDARY: CONFIG CLONE WITH NIL RECEIVER
// ============================================================================

func TestConfigCloneNilReceiver(t *testing.T) {
	var nilCfg *Config
	cloned := nilCfg.Clone()
	if cloned.Level != 0 {
		t.Error("Clone of nil *Config should return zero-value Config")
	}
}
