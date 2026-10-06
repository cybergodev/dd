package dd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// D-005 Phase 1: API unification tests
//
// Covers DefaultSamplingConfig, ProductionConfig, the Config.LevelResolver
// wiring, and the package-level lifecycle mirror (Close/Shutdown).
// ============================================================================

func TestDefaultSamplingConfig(t *testing.T) {
	s := DefaultSamplingConfig()
	if !s.Enabled {
		t.Error("DefaultSamplingConfig().Enabled = false, want true (attach = active, matching DefaultAuditConfig)")
	}
	if s.Initial != 100 {
		t.Errorf("DefaultSamplingConfig().Initial = %d, want 100", s.Initial)
	}
	if s.Thereafter != 100 {
		t.Errorf("DefaultSamplingConfig().Thereafter = %d, want 100", s.Thereafter)
	}
	if s.Tick != time.Second {
		t.Errorf("DefaultSamplingConfig().Tick = %v, want 1s (per-second burst window; a never-resetting counter would permanently drop 99%% of a low-traffic service's logs)", s.Tick)
	}

	// Wiring: assigning the default to Config.Sampling activates sampling.
	var buf bytes.Buffer
	cfg := NewTestConfigWithBuffer(&buf)
	cfg.Sampling = &s
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer logger.Close()

	got := logger.GetSampling()
	if got == nil {
		t.Fatal("GetSampling() = nil, want sampling enabled via DefaultSamplingConfig")
	}
	if got.Initial != 100 || got.Thereafter != 100 || got.Tick != time.Second {
		t.Errorf("GetSampling() = %+v, want Initial=100, Thereafter=100, Tick=1s", got)
	}

	// Disabled template: Enabled=false must not activate sampling.
	s2 := DefaultSamplingConfig()
	s2.Enabled = false
	cfg2 := NewTestConfigWithBuffer(&bytes.Buffer{})
	cfg2.Sampling = &s2
	logger2, err := New(cfg2)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer logger2.Close()
	if got := logger2.GetSampling(); got != nil {
		t.Errorf("GetSampling() = %+v, want nil for Enabled=false template", got)
	}
}

func TestProductionConfig(t *testing.T) {
	cfg := ProductionConfig()
	if cfg.Level != LevelInfo {
		t.Errorf("ProductionConfig().Level = %v, want LevelInfo", cfg.Level)
	}
	if cfg.Format != FormatJSON {
		t.Errorf("ProductionConfig().Format = %v, want FormatJSON", cfg.Format)
	}
	if cfg.TimeFormat != time.RFC3339 {
		t.Errorf("ProductionConfig().TimeFormat = %q, want %q", cfg.TimeFormat, time.RFC3339)
	}
	if !cfg.IncludeTime || !cfg.IncludeLevel || !cfg.DynamicCaller {
		t.Errorf("ProductionConfig() decorations: IncludeTime=%v IncludeLevel=%v DynamicCaller=%v, want all true",
			cfg.IncludeTime, cfg.IncludeLevel, cfg.DynamicCaller)
	}
	if cfg.Security == nil {
		t.Error("ProductionConfig().Security = nil, want default security config")
	}
	if cfg.JSON == nil {
		t.Error("ProductionConfig().JSON = nil, want default JSON options")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("ProductionConfig().Validate() = %v, want nil", err)
	}

	var buf bytes.Buffer
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New(ProductionConfig()) error: %v", err)
	}
	defer logger.Close()

	logger.Info("production-preset-works")
	if !strings.Contains(buf.String(), "production-preset-works") {
		t.Errorf("output %q does not contain Info message", buf.String())
	}
	logger.Debug("production-debug-hidden")
	if strings.Contains(buf.String(), "production-debug-hidden") {
		t.Error("Debug message passed the Info-level gate of ProductionConfig")
	}
}

func TestConfigLevelResolver(t *testing.T) {
	// Default: no resolver installed.
	base, err := New(NewTestConfigWithBuffer(&bytes.Buffer{}))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if base.GetLevelResolver() != nil {
		t.Error("default config must not install a LevelResolver")
	}
	base.Close()

	// A single-goroutine test: the resolver reads `level` synchronously
	// from the logging call on the same goroutine.
	var buf bytes.Buffer
	level := LevelError
	cfg := NewTestConfigWithBuffer(&buf)
	cfg.Level = LevelDebug
	cfg.LevelResolver = func(ctx context.Context) LogLevel { return level }

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer logger.Close()

	if got := logger.GetLevelResolver(); got == nil {
		t.Fatal("GetLevelResolver() = nil, want the Config-configured resolver")
	}
	if logger.IsLevelEnabled(LevelInfo) {
		t.Error("IsLevelEnabled(LevelInfo) = true, want false under an Error-level resolver")
	}

	logger.Info("resolver-dropped") // gated out by the resolver
	logger.Error("resolver-kept")
	out := buf.String()
	if strings.Contains(out, "resolver-dropped") {
		t.Error("Info message passed a resolver set to LevelError")
	}
	if !strings.Contains(out, "resolver-kept") {
		t.Error("Error message was dropped by a resolver set to LevelError")
	}

	// The resolver is consulted dynamically.
	level = LevelDebug
	logger.Info("resolver-flip")
	if !strings.Contains(buf.String(), "resolver-flip") {
		t.Error("Info message was dropped after the resolver flipped to LevelDebug")
	}

	// Clone preserves the resolver (shared function value, like FatalHandler).
	clone := cfg.Clone()
	if clone.LevelResolver == nil {
		t.Error("Clone() dropped LevelResolver")
	}
}

func TestPackageLevelLifecycleMirror(t *testing.T) {
	// Install a dedicated logger so the test never closes the process-wide
	// default that other tests may still reference.
	oldDefault := Default()
	t.Cleanup(func() { SetDefault(oldDefault) })

	var buf bytes.Buffer
	logger, err := New(NewTestConfigWithBuffer(&buf))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	SetDefault(logger)

	if Default().IsClosed() {
		t.Fatal("freshly installed default logger should be open")
	}

	if err := Close(); err != nil {
		t.Errorf("dd.Close() = %v, want nil", err)
	}
	if !Default().IsClosed() {
		t.Error("dd.Close() did not close the default logger")
	}

	// Shutdown on an already-closed logger is a no-op returning nil.
	if err := Shutdown(context.Background()); err != nil {
		t.Errorf("dd.Shutdown() on closed logger = %v, want nil", err)
	}

	// An already-expired context fails fast without closing the logger
	// (Logger.Shutdown contract) — verify the mirror preserves it.
	logger2, err := New(NewTestConfigWithBuffer(&bytes.Buffer{}))
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	SetDefault(logger2)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Shutdown(canceled); err != context.Canceled {
		t.Errorf("dd.Shutdown(canceled ctx) = %v, want context.Canceled", err)
	}
	if Default().IsClosed() {
		t.Error("dd.Shutdown with an expired context must leave the logger open")
	}
}

// ============================================================================
// D-005 Phase 2: old→new API mapping regression tests
//
// The v1.0 fluent/Options API is long gone; these tests pin the BEHAVIOR of
// its replacements so the mappings documented in MIGRATION.md cannot drift.
// ============================================================================

// TestNilSecurityFallsBackToBasic pins the migration-table pitfall:
// cfg.Security = nil does NOT disable filtering — the constructor installs
// the basic filter (old API: DisableFiltering()).
func TestNilSecurityFallsBackToBasic(t *testing.T) {
	var buf bytes.Buffer
	cfg := NewTestConfigWithBuffer(&buf)
	cfg.Security = nil
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer logger.Close()

	logger.Info("password=secret123")
	if !strings.Contains(buf.String(), "[REDACTED]") {
		t.Errorf("nil Security must fall back to basic filtering; output: %q", buf.String())
	}
}

// TestSecurityPresetStrengthEquivalence pins the strength equivalences
// asserted in MIGRATION.md:
//
//	DefaultSecurityConfig()          == SecurityConfigForLevel(Basic)
//	DefaultSecureConfig() (deprecated) == SecurityConfigForLevel(Standard)
//
// Discriminators: password is redacted at basic strength; email only at full
// strength (the email pattern is full-only, gated on '@' in the input).
func TestSecurityPresetStrengthEquivalence(t *testing.T) {
	const passwordInput = "password=secret123"
	const emailInput = "contact user@example.com now"

	basicFilters := map[string]*SensitiveDataFilter{
		"DefaultSecurityConfig":         DefaultSecurityConfig().SensitiveFilter,
		"SecurityConfigForLevel(Basic)": SecurityConfigForLevel(SecurityLevelBasic).SensitiveFilter,
	}
	fullFilters := map[string]*SensitiveDataFilter{
		"DefaultSecureConfig":              DefaultSecureConfig().SensitiveFilter, //nolint:staticcheck // pinning the deprecated API's equivalence until v2 removal
		"SecurityConfigForLevel(Standard)": SecurityConfigForLevel(SecurityLevelStandard).SensitiveFilter,
	}

	for name, filter := range basicFilters {
		if out := filter.Filter(passwordInput); !strings.Contains(out, "[REDACTED]") {
			t.Errorf("%s: basic strength must redact passwords; got %q", name, out)
		}
		if out := filter.Filter(emailInput); strings.Contains(out, "[REDACTED]") {
			t.Errorf("%s: basic strength must NOT redact emails; got %q", name, out)
		}
	}
	for name, filter := range fullFilters {
		if out := filter.Filter(passwordInput); !strings.Contains(out, "[REDACTED]") {
			t.Errorf("%s: full strength must redact passwords; got %q", name, out)
		}
		if out := filter.Filter(emailInput); !strings.Contains(out, "[REDACTED]") {
			t.Errorf("%s: full strength must redact emails; got %q", name, out)
		}
	}
}

// TestOutputTargetHelperDefaults pins FileOutput's documented rotation
// defaults (old API: passing FileWriterConfig to WithFile).
func TestOutputTargetHelperDefaults(t *testing.T) {
	target := FileOutput("app.log")
	if target.Type != OutputFile {
		t.Errorf("FileOutput().Type = %v, want OutputFile", target.Type)
	}
	if target.Path != "app.log" {
		t.Errorf("FileOutput().Path = %q, want %q", target.Path, "app.log")
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
	if target.Compress {
		t.Error("FileOutput().Compress = true, want false")
	}

	if c := ConsoleOutput(); c.Type != OutputConsole {
		t.Errorf("ConsoleOutput().Type = %v, want OutputConsole", c.Type)
	}
	var buf bytes.Buffer
	if c := CustomOutput(&buf); c.Type != OutputCustom || c.Writer == nil {
		t.Errorf("CustomOutput() = %+v, want Type=OutputCustom with non-nil Writer", c)
	}
}

// TestMultipleTargetsFanout pins the Targets replacement for the old
// Options.Console/Options.File/Options.AdditionalWriters flags: every target
// receives every entry (old API: ToAll).
func TestMultipleTargetsFanout(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	logPath := filepath.Join(t.TempDir(), "fanout.log")

	cfg := DefaultConfig()
	cfg.Level = LevelDebug
	cfg.Targets = []OutputTarget{
		CustomOutput(&buf1),
		CustomOutput(&buf2),
		FileOutput(logPath),
	}
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer logger.Close()

	logger.Info("fanout-message")

	if !strings.Contains(buf1.String(), "fanout-message") {
		t.Errorf("custom target 1 output %q missing message", buf1.String())
	}
	if !strings.Contains(buf2.String(), "fanout-message") {
		t.Errorf("custom target 2 output %q missing message", buf2.String())
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "fanout-message") {
		t.Errorf("file target output %q missing message", string(data))
	}
}
