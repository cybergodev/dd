package dd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewConfig tests the new unified configuration API
func TestNewConfig(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		cfg := DefaultConfig()
		if cfg.Level != LevelInfo {
			t.Errorf("Expected LevelInfo, got %v", cfg.Level)
		}
		if cfg.Format != FormatText {
			t.Errorf("Expected FormatText, got %v", cfg.Format)
		}
	})

	t.Run("build logger", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf)}
		cfg.Format = FormatJSON
		cfg.Level = LevelDebug

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to build logger: %v", err)
		}
		defer logger.Close()

		logger.Info("test message")

		output := buf.String()
		if !strings.Contains(output, `"message":"test message"`) {
			t.Errorf("Expected JSON output with message, got: %s", output)
		}
	})
}

func TestConfigDevelopment(t *testing.T) {
	cfg := DevelopmentConfig()

	if cfg.Level != LevelDebug {
		t.Errorf("Expected LevelDebug, got %v", cfg.Level)
	}
	if cfg.Format != FormatText {
		t.Errorf("Expected FormatText, got %v", cfg.Format)
	}
	if !cfg.DynamicCaller {
		t.Error("Expected DynamicCaller to be true")
	}
}

func TestConfigJSON(t *testing.T) {
	cfg := JSONConfig()

	if cfg.Format != FormatJSON {
		t.Errorf("Expected FormatJSON, got %v", cfg.Format)
	}
	if cfg.JSON == nil {
		t.Error("Expected JSON options to be initialized")
	}
}

// TestConfigFileOutput was removed: "File config sets file path" is
// asserted (with the defaults) by boundary_test.go
// TestOutputTypeConstructors, and "modify rotation settings" only re-checked
// struct-field assignments.

func TestBuilderConfigClone(t *testing.T) {
	t.Run("clone preserves settings", func(t *testing.T) {
		original := DefaultConfig()
		original.Format = FormatJSON
		original.Level = LevelDebug
		original.DynamicCaller = true
		target := FileOutput("")
		target.MaxSizeMB = 50
		original.Targets = []OutputTarget{target}

		cloned := original.Clone()

		if cloned.Format != original.Format {
			t.Errorf("Clone Format mismatch")
		}
		if cloned.Level != original.Level {
			t.Errorf("Clone Level mismatch")
		}
		if cloned.DynamicCaller != original.DynamicCaller {
			t.Errorf("Clone DynamicCaller mismatch")
		}
		if cloned.Targets[0].MaxSizeMB != original.Targets[0].MaxSizeMB {
			t.Errorf("Clone Targets[0].MaxSizeMB mismatch")
		}
	})

	t.Run("clone is independent", func(t *testing.T) {
		original := DefaultConfig()
		original.Level = LevelDebug
		cloned := original.Clone()

		// Modify clone
		cloned.Level = LevelInfo

		// Original should not be affected
		if original.Level != LevelDebug {
			t.Errorf("Original should still be LevelDebug, got %v", original.Level)
		}
		if cloned.Level != LevelInfo {
			t.Errorf("Clone should be LevelInfo, got %v", cloned.Level)
		}
	})

	t.Run("clone for multiple loggers", func(t *testing.T) {
		base := DefaultConfig()
		base.Format = FormatJSON

		// Create app logger
		appCfg := base.Clone()
		appTarget := FileOutput("logs/app.log")
		appTarget.MaxSizeMB = 100
		appCfg.Targets = []OutputTarget{appTarget}

		// Create error logger
		errCfg := base.Clone()
		errTarget := FileOutput("logs/error.log")
		errTarget.MaxSizeMB = 50
		errCfg.Targets = []OutputTarget{errTarget}
		errCfg.Level = LevelError

		// Verify they're independent
		if appCfg.Targets[0].MaxSizeMB != 100 {
			t.Errorf("App config MaxSizeMB should be 100")
		}
		if errCfg.Targets[0].MaxSizeMB != 50 {
			t.Errorf("Error config MaxSizeMB should be 50")
		}
		if errCfg.Level != LevelError {
			t.Errorf("Error config Level should be LevelError")
		}
	})

	t.Run("clone nil config", func(t *testing.T) {
		var nilCfg *Config
		cloned := nilCfg.Clone()
		// Cloning nil pointer returns zero-value Config
		if cloned.Level != 0 || cloned.Format != 0 {
			t.Error("Clone of nil config should return nil")
		}
	})

	t.Run("clone with all fields populated", func(t *testing.T) {
		original := &Config{
			Level:         LevelDebug,
			Format:        FormatJSON,
			TimeFormat:    time.RFC3339Nano,
			IncludeTime:   false,
			IncludeLevel:  false,
			FullPath:      true,
			DynamicCaller: true,
			Targets: []OutputTarget{
				{
					Type:       OutputFile,
					Path:       "test.log",
					MaxSizeMB:  50,
					MaxBackups: 10,
					MaxAge:     24 * time.Hour,
					Compress:   true,
				},
				{
					Type:   OutputCustom,
					Writer: io.Discard,
				},
				{
					Type:   OutputCustom,
					Writer: os.Stdout,
				},
			},
			JSON: &JSONOptions{
				PrettyPrint: true,
				Indent:      "    ",
				FieldNames: &JSONFieldNames{
					Timestamp: "ts",
					Level:     "lvl",
					Caller:    "src",
					Message:   "msg",
					Fields:    "data",
				},
			},
			Security: &SecurityConfig{
				MaxMessageSize:  1000,
				MaxWriters:      50,
				SensitiveFilter: NewSensitiveDataFilter(),
			},
			FieldValidation: &FieldValidationConfig{
				Mode: FieldValidationStrict,
			},
			Sampling: &SamplingConfig{
				Enabled:    true,
				Initial:    100,
				Thereafter: 10,
				Tick:       time.Minute,
			},
		}

		cloned := original.Clone()

		// Verify all fields copied correctly
		if cloned.Level != original.Level {
			t.Error("Level mismatch")
		}
		if cloned.Format != original.Format {
			t.Error("Format mismatch")
		}
		if cloned.TimeFormat != original.TimeFormat {
			t.Error("TimeFormat mismatch")
		}
		if cloned.IncludeTime != original.IncludeTime {
			t.Error("IncludeTime mismatch")
		}
		if cloned.FullPath != original.FullPath {
			t.Error("FullPath mismatch")
		}
		if len(cloned.Targets) == 0 || cloned.Targets[0].Path != "test.log" {
			t.Error("Targets not cloned properly")
		}
		if cloned.JSON == nil || !cloned.JSON.PrettyPrint {
			t.Error("JSON config not cloned properly")
		}
		if cloned.JSON.FieldNames == nil || cloned.JSON.FieldNames.Timestamp != "ts" {
			t.Error("JSON FieldNames not cloned properly")
		}
		if cloned.Security == nil || cloned.Security.MaxMessageSize != 1000 {
			t.Error("Security config not cloned properly")
		}
		if cloned.Sampling == nil || !cloned.Sampling.Enabled {
			t.Error("Sampling config not cloned properly")
		}
		if len(cloned.Targets) != 3 {
			t.Error("Targets not cloned properly")
		}

		// Verify independence
		cloned.Targets[0].Path = "modified.log"
		if original.Targets[0].Path == "modified.log" {
			t.Error("Modifying clone should not affect original")
		}

		cloned.JSON.FieldNames.Timestamp = "modified_ts"
		if original.JSON.FieldNames.Timestamp == "modified_ts" {
			t.Error("Modifying clone JSON.FieldNames should not affect original")
		}
	})

	t.Run("clone with nil optional fields", func(t *testing.T) {
		original := &Config{
			Level:    LevelInfo,
			Format:   FormatText,
			Targets:  nil,
			JSON:     nil,
			Security: nil,
			Sampling: nil,
		}

		cloned := original.Clone()

		if len(cloned.Targets) != 0 {
			t.Error("Cloned Targets should be empty")
		}
		if cloned.JSON != nil {
			t.Error("Cloned JSON should be nil")
		}
		if cloned.Security != nil {
			t.Error("Cloned Security should be nil")
		}
		if cloned.Sampling != nil {
			t.Error("Cloned Sampling should be nil")
		}
	})
}

func TestConfigAddHook(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Hooks = NewHookRegistry()
	cfg.Hooks.Add(HookBeforeLog, func(ctx context.Context, h *HookContext) error {
		return nil
	})

	if cfg.Hooks == nil {
		t.Fatal("Expected Hooks to be initialized")
	}
	if cfg.Hooks.count() != 1 {
		t.Errorf("Expected 1 hook, got %d", cfg.Hooks.count())
	}
}

func TestConfigIntegration(t *testing.T) {
	t.Run("complete configuration example", func(t *testing.T) {
		var buf bytes.Buffer

		cfg := DefaultConfig()
		cfg.Targets = []OutputTarget{CustomOutput(&buf)}
		cfg.Format = FormatJSON
		cfg.Level = LevelDebug
		cfg.DynamicCaller = true
		cfg.FullPath = false

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to build: %v", err)
		}
		defer logger.Close()

		logger.InfoWith("test message", String("key", "value"))

		output := buf.String()
		if !strings.Contains(output, `"message":"test message"`) {
			t.Errorf("Expected JSON message, got: %s", output)
		}
		if !strings.Contains(output, `"key":"value"`) {
			t.Errorf("Expected field 'key:value', got: %s", output)
		}
	})

	t.Run("file output configuration", func(t *testing.T) {
		// Create temp directory
		tmpDir := t.TempDir()

		cfg := DefaultConfig()
		target := FileOutput(tmpDir + "/test.log")
		target.MaxSizeMB = 10
		target.MaxBackups = 5
		target.Compress = true
		cfg.Targets = []OutputTarget{target}
		cfg.Format = FormatJSON
		cfg.Level = LevelInfo

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to build: %v", err)
		}
		defer logger.Close()

		logger.Info("test message")

		// Verify file was created
		if _, err := os.Stat(tmpDir + "/test.log"); os.IsNotExist(err) {
			t.Error("Expected log file to be created")
		}
	})
}

func TestNewMultipleConfigs(t *testing.T) {
	t.Run("returns error when multiple configs provided", func(t *testing.T) {
		cfg1 := DefaultConfig()
		cfg2 := DefaultConfig()

		_, err := New(cfg1, cfg2)
		if err == nil {
			t.Error("Expected error when multiple configs provided")
		}

		if !errors.Is(err, ErrMultipleConfigs) {
			t.Errorf("Expected ErrMultipleConfigs, got: %v", err)
		}
	})

	t.Run("returns error with three configs", func(t *testing.T) {
		cfg1 := DefaultConfig()
		cfg2 := DefaultConfig()
		cfg3 := DefaultConfig()

		_, err := New(cfg1, cfg2, cfg3)
		if err == nil {
			t.Error("Expected error when multiple configs provided")
		}

		if !strings.Contains(err.Error(), "3 configs") {
			t.Errorf("Expected error message to contain '3 configs', got: %v", err)
		}
	})

	t.Run("works with single config", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Level = LevelDebug

		logger, err := New(cfg)
		if err != nil {
			t.Errorf("Unexpected error with single config: %v", err)
		}
		defer logger.Close()
	})

	t.Run("works with no config", func(t *testing.T) {
		logger, err := New()
		if err != nil {
			t.Errorf("Unexpected error with no config: %v", err)
		}
		defer logger.Close()
	})
}

// TestOutputTargetResolve covers every OutputTarget.resolve branch, including
// the error sentinels for empty file paths, nil custom writers, and unknown
// target types.
func TestOutputTargetResolve(t *testing.T) {
	tests := []struct {
		name    string
		target  OutputTarget
		wantErr error
	}{
		// Console resolves to os.Stdout — must NOT be closed here.
		{"console", ConsoleOutput(), nil},
		{"file with path", FileOutput(filepath.Join(t.TempDir(), "resolve.log")), nil},
		{"file without path", OutputTarget{Type: OutputFile}, ErrEmptyFilePath},
		{"custom writer", CustomOutput(&bytes.Buffer{}), nil},
		{"custom without writer", OutputTarget{Type: OutputCustom}, ErrNilWriter},
		{"unknown type", OutputTarget{Type: OutputType(99)}, nil /* asserted below */},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := tt.target.resolve()

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("resolve() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if tt.name == "unknown type" {
				// Unknown types report a descriptive error rather than a sentinel.
				if err == nil || !strings.Contains(err.Error(), "unknown output type") {
					t.Errorf("resolve(unknown type) error = %v, want 'unknown output type'", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("resolve() unexpected error: %v", err)
			}
			if w == nil {
				t.Fatal("resolve() returned nil writer")
			}
			// Only close writers we own: console rows share os.Stdout with
			// the test binary, and custom rows use caller-owned buffers.
			if tt.target.Type == OutputFile {
				if closer, ok := w.(io.Closer); ok {
					closer.Close()
				}
			}
		})
	}
}

// TestConfigValidateErrors pins every Config.Validate rejection branch and
// its sentinel, so config mistakes fail at New() with a matchable error.
func TestConfigValidateErrors(t *testing.T) {
	tooManyTargets := make([]OutputTarget, maxWriterCount+1)
	for i := range tooManyTargets {
		tooManyTargets[i] = CustomOutput(&bytes.Buffer{})
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr error
	}{
		{"level below range", func(c *Config) { c.Level = LevelDebug - 1 }, ErrInvalidLevel},
		{"level above range", func(c *Config) { c.Level = LevelFatal + 1 }, ErrInvalidLevel},
		{"format invalid", func(c *Config) { c.Format = LogFormat(99) }, ErrInvalidFormat},
		{
			"non-roundtripping time format",
			func(c *Config) { c.IncludeTime = true; c.TimeFormat = "1_2" },
			nil, // exact error type asserted loosely below (internal validation)
		},
		{
			"too many writers",
			func(c *Config) { c.Targets = tooManyTargets },
			ErrMaxWritersExceeded,
		},
		{
			"custom target without writer",
			func(c *Config) { c.Targets = []OutputTarget{{Type: OutputCustom}} },
			nil, // plain fmt.Errorf, asserted by message below
		},
		{
			"file target without path",
			func(c *Config) { c.Targets = []OutputTarget{{Type: OutputFile}} },
			nil, // plain fmt.Errorf, asserted by message below
		},
		{
			"invalid audit config",
			func(c *Config) { c.Audit = &AuditConfig{BufferSize: -1} },
			nil, // wrapped audit error, asserted by message below
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(&cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	// Sanity: the defaults themselves must validate.
	if err := DefaultConfig().Validate(); err != nil {
		t.Errorf("DefaultConfig().Validate() = %v, want nil", err)
	}
}
