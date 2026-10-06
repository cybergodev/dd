package dd

import (
	"io"
	"time"

	"github.com/cybergodev/dd/internal"
)

// OutputType defines the type of log output destination.
type OutputType int

const (
	// OutputConsole writes to os.Stdout.
	OutputConsole OutputType = iota
	// OutputFile writes to a file with rotation support.
	OutputFile
	// OutputCustom writes to a user-provided io.Writer.
	OutputCustom
)

// OutputTarget configures a single log output destination.
// Use ConsoleOutput(), FileOutput(), or CustomOutput() helpers to create instances.
//
// Example:
//
//	cfg := dd.DefaultConfig()
//	cfg.Targets = []dd.OutputTarget{
//	    dd.ConsoleOutput(),
//	    dd.FileOutput("logs/app.log"),
//	    dd.CustomOutput(customWriter),
//	}
//	logger, _ := dd.New(cfg)
type OutputTarget struct {
	// Type specifies the output destination type.
	Type OutputType

	// File output settings (OutputFile only).
	Path       string
	MaxSizeMB  int
	MaxBackups int
	MaxAge     time.Duration
	Compress   bool

	// Custom writer (OutputCustom only).
	Writer io.Writer
}

// ConsoleOutput creates an OutputTarget that writes to os.Stdout.
func ConsoleOutput() OutputTarget {
	return OutputTarget{Type: OutputConsole}
}

// FileOutput creates an OutputTarget that writes to a file with rotation.
// Defaults: MaxSizeMB=100, MaxBackups=10, MaxAge=30 days, Compress=false.
// Modify the returned value to customize rotation settings.
//
// Example:
//
//	target := dd.FileOutput("logs/app.log")
//	target.MaxSizeMB = 50
//	target.Compress = true
func FileOutput(path string) OutputTarget {
	return OutputTarget{
		Type:       OutputFile,
		Path:       path,
		MaxSizeMB:  DefaultMaxSizeMB,
		MaxBackups: DefaultMaxBackups,
		MaxAge:     DefaultMaxAge,
	}
}

// CustomOutput creates an OutputTarget that writes to a custom io.Writer.
func CustomOutput(w io.Writer) OutputTarget {
	return OutputTarget{Type: OutputCustom, Writer: w}
}

// Config provides a struct-based configuration API for creating loggers.
// Direct field modification with IDE autocomplete support.
//
// Zero-value contract: the zero Config (Config{}) is valid but is NOT
// equivalent to DefaultConfig — it logs at LevelDebug with IncludeTime,
// IncludeLevel, and DynamicCaller disabled. Always start from DefaultConfig()
// or one of the presets (DevelopmentConfig, JSONConfig, ProductionConfig)
// and modify fields from there.
//
// Example:
//
//	cfg := dd.DefaultConfig()
//	cfg.Format = dd.FormatJSON
//	cfg.Level = dd.LevelDebug
//	logger, _ := dd.New(cfg)
type Config struct {
	// Log level
	Level LogLevel

	// Output format
	Format LogFormat

	// Time settings
	TimeFormat   string
	IncludeTime  bool
	IncludeLevel bool

	// Caller information
	DynamicCaller bool
	FullPath      bool

	// Output destinations (unified).
	// Use ConsoleOutput(), FileOutput(), CustomOutput() helpers.
	// When empty, the logger writes to os.Stdout.
	Targets []OutputTarget

	// JSON configuration
	JSON *JSONOptions

	// Security configuration
	Security *SecurityConfig

	// Field validation configuration
	FieldValidation *FieldValidationConfig

	// Lifecycle handlers
	FatalHandler      FatalHandler
	WriteErrorHandler WriteErrorHandler

	// Dynamic level resolution. When set, the resolver determines the
	// effective log level for every log entry and every IsLevelEnabled
	// check, overriding Level. The zero value (nil) disables dynamic
	// resolution. Can be replaced after construction with
	// Logger.SetLevelResolver.
	LevelResolver LevelResolver

	// Extensibility
	ContextExtractors []ContextExtractor
	Hooks             *HookRegistry
	Sampling          *SamplingConfig

	// Audit configuration for security event logging.
	// When set, audit events are emitted for sensitive data redactions,
	// rate limit events, and security violations.
	Audit *AuditConfig
}

// DefaultConfig creates a new Config with default settings.
// Returns a value type; callers modify their own copy without affecting defaults.
//
// Example:
//
//	cfg := dd.DefaultConfig()
//	cfg.Level = dd.LevelDebug
//	cfg.Format = dd.FormatJSON
//	logger, _ := dd.New(cfg)
func DefaultConfig() Config {
	return Config{
		Level:         LevelInfo,
		Format:        FormatText,
		TimeFormat:    DefaultTimeFormat,
		IncludeTime:   true,
		IncludeLevel:  true,
		FullPath:      false,
		DynamicCaller: true,                    // Enable dynamic caller detection by default
		Security:      DefaultSecurityConfig(), // Security enabled by default
		FatalHandler:  defaultFatalHandler,
	}
}

// DevelopmentConfig creates a Config with development-friendly settings.
// Enables DEBUG level and dynamic caller detection.
// Note: Security filtering is enabled by default even in development mode
// to catch accidental logging of sensitive data early in the development cycle.
//
// Example:
//
//	cfg := dd.DevelopmentConfig()
//	cfg.Targets = []dd.OutputTarget{dd.FileOutput("dev.log")}
//	logger, _ := dd.New(cfg)
func DevelopmentConfig() Config {
	return Config{
		Level:         LevelDebug,
		Format:        FormatText,
		TimeFormat:    devTimeFormat,
		IncludeTime:   true,
		IncludeLevel:  true,
		FullPath:      false,
		DynamicCaller: true,
		Security:      DefaultSecurityConfig(), // Security enabled by default
		FatalHandler:  defaultFatalHandler,
	}
}

// JSONConfig creates a Config with JSON output settings.
// Note: Security filtering is enabled by default to prevent sensitive data
// from being logged in JSON format which is often shipped to external systems.
//
// Example:
//
//	cfg := dd.JSONConfig()
//	cfg.Level = dd.LevelInfo
//	logger, _ := dd.New(cfg)
func JSONConfig() Config {
	return Config{
		Level:         LevelDebug,
		Format:        FormatJSON,
		TimeFormat:    time.RFC3339,
		IncludeTime:   true,
		IncludeLevel:  true,
		FullPath:      false,
		DynamicCaller: true,
		Security:      DefaultSecurityConfig(), // Security enabled by default
		FatalHandler:  defaultFatalHandler,
		JSON:          DefaultJSONOptions(),
	}
}

// ProductionConfig creates a Config with production-ready settings:
// Info level, JSON output, RFC3339 timestamps, and dynamic caller detection.
// Security filtering is enabled by default so accidental sensitive-data
// logging is caught before records are shipped to aggregation systems.
// Combine with Targets (e.g. FileOutput) and optionally Audit for
// security-event monitoring.
//
// Example:
//
//	cfg := dd.ProductionConfig()
//	cfg.Targets = []dd.OutputTarget{dd.FileOutput("logs/app.log")}
//	logger, _ := dd.New(cfg)
func ProductionConfig() Config {
	return Config{
		Level:         LevelInfo,
		Format:        FormatJSON,
		TimeFormat:    time.RFC3339,
		IncludeTime:   true,
		IncludeLevel:  true,
		FullPath:      false,
		DynamicCaller: true,
		Security:      DefaultSecurityConfig(), // Security enabled by default
		FatalHandler:  defaultFatalHandler,
		JSON:          DefaultJSONOptions(),
	}
}

// Clone creates a copy of the configuration.
//
// Clone behavior:
//   - Deep copy: JSON, Sampling, Security, Hooks, Audit configs
//   - Shared by pointer (later mutations through the original are visible to
//     the clone): FieldValidation (the whole *FieldValidationConfig struct)
//   - Shared values: FatalHandler, WriteErrorHandler, LevelResolver (function values)
//   - ContextExtractors slice is copied but extractor instances are shared
//
// MAINTENANCE: When adding new pointer/slice/map fields to Config, you MUST
// add corresponding deep-copy logic in this method. Forgetting to do so will
// cause subtle shared-mutation bugs. Search for "Clone" in this file to find
// the switch-like copy blocks.
//
// Example:
//
//	base := dd.DefaultConfig()
//	base.Format = dd.FormatJSON
//
//	appCfg := base.Clone()
//	appCfg.Targets = []dd.OutputTarget{dd.FileOutput("app.log")}
//	logger, _ := dd.New(appCfg)
func (c *Config) Clone() Config {
	if c == nil {
		return Config{}
	}
	clone := Config{
		Level:             c.Level,
		Format:            c.Format,
		TimeFormat:        c.TimeFormat,
		IncludeTime:       c.IncludeTime,
		IncludeLevel:      c.IncludeLevel,
		FullPath:          c.FullPath,
		DynamicCaller:     c.DynamicCaller,
		FieldValidation:   c.FieldValidation,
		FatalHandler:      c.FatalHandler,
		WriteErrorHandler: c.WriteErrorHandler,
		LevelResolver:     c.LevelResolver,
		Sampling:          c.Sampling,
	}

	// Copy Targets slice
	if c.Targets != nil {
		clone.Targets = make([]OutputTarget, len(c.Targets))
		copy(clone.Targets, c.Targets)
	}

	// Copy JSON options
	if c.JSON != nil {
		clone.JSON = &internal.JSONOptions{
			PrettyPrint: c.JSON.PrettyPrint,
			Indent:      c.JSON.Indent,
		}
		if c.JSON.FieldNames != nil {
			clone.JSON.FieldNames = &internal.JSONFieldNames{
				Timestamp: c.JSON.FieldNames.Timestamp,
				Level:     c.JSON.FieldNames.Level,
				Caller:    c.JSON.FieldNames.Caller,
				Message:   c.JSON.FieldNames.Message,
				Fields:    c.JSON.FieldNames.Fields,
			}
		}
	}

	// Copy ContextExtractors
	if c.ContextExtractors != nil {
		clone.ContextExtractors = make([]ContextExtractor, len(c.ContextExtractors))
		copy(clone.ContextExtractors, c.ContextExtractors)
	}

	// Copy Hooks
	if c.Hooks != nil {
		clone.Hooks = c.Hooks.clone()
	}

	// Copy Security config
	if c.Security != nil {
		clone.Security = c.Security.Clone()
	}

	// Copy Sampling config
	if c.Sampling != nil {
		clone.Sampling = &SamplingConfig{
			Enabled:    c.Sampling.Enabled,
			Initial:    c.Sampling.Initial,
			Thereafter: c.Sampling.Thereafter,
			Tick:       c.Sampling.Tick,
		}
	}

	// Copy Audit config (reuse AuditConfig.Clone to avoid field-copy drift)
	if c.Audit != nil {
		audit := c.Audit.Clone()
		clone.Audit = &audit
	}

	return clone
}

// ============================================================================
// JSON Options
// ============================================================================

// JSONOptions configures JSON output format.
type JSONOptions = internal.JSONOptions

// JSONFieldNames configures custom field names for JSON output.
type JSONFieldNames = internal.JSONFieldNames

// DefaultJSONOptions returns default JSON options.
func DefaultJSONOptions() *JSONOptions {
	return &JSONOptions{
		PrettyPrint: false,
		Indent:      defaultJSONIndent,
		FieldNames:  internal.DefaultJSONFieldNames(),
	}
}

// ============================================================================
// Sampling Configuration
// ============================================================================

// SamplingConfig configures log sampling for high-throughput scenarios.
// Sampling reduces log volume by only recording a subset of messages.
type SamplingConfig struct {
	// Enabled controls whether sampling is active.
	Enabled bool
	// Initial is the number of messages that are always logged before sampling begins.
	// This ensures visibility of initial burst traffic.
	Initial int
	// Thereafter is the sampling rate after Initial messages.
	// A value of 10 means log 1 out of every 10 messages.
	Thereafter int
	// Tick is the time interval after which counters are reset.
	// This allows sampling to restart periodically for burst handling.
	Tick time.Duration
}

// DefaultSamplingConfig returns a SamplingConfig with sensible defaults:
// Initial=100, Thereafter=100, Tick=1s. Counters reset every second, so the
// Initial allowance refreshes per second: traffic below 100 messages/second
// is never sampled, and only burst traffic beyond it is reduced to 1 in 100 —
// matching the per-second budget model of DefaultRateLimitConfig. Set Tick=0
// for process-lifetime counters (the first 100 messages ever, then 1 in 100
// forever). The returned config has Enabled=true, so assigning it to
// Config.Sampling activates sampling — set Enabled=false on the returned
// value to use it as an inert template.
//
// Example:
//
//	cfg := dd.DefaultConfig()
//	sampling := dd.DefaultSamplingConfig()
//	sampling.Initial = 10 // per second: log first 10, then 1 in 10
//	cfg.Sampling = &sampling
//	logger, _ := dd.New(cfg)
func DefaultSamplingConfig() SamplingConfig {
	return SamplingConfig{
		Enabled:    true,
		Initial:    100,
		Thereafter: 100,
		Tick:       time.Second,
	}
}
