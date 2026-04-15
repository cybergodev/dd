package dd

import (
	"io"
	"time"

	"github.com/cybergodev/dd/internal"
)

// FileConfig configures file output with rotation settings.
type FileConfig struct {
	Path       string        // Log file path
	MaxSizeMB  int           // Max file size in MB before rotation (default: 100)
	MaxBackups int           // Max number of old log files to retain (default: 10)
	MaxAge     time.Duration // Max duration to retain old log files (default: 30 days)
	Compress   bool          // Enable gzip compression for rotated files (default: false)
}

// Config provides a struct-based configuration API for creating loggers.
// Direct field modification with IDE autocomplete support.
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

	// Output targets
	Output  io.Writer   // Single output writer
	Outputs []io.Writer // Multiple output writers
	File    *FileConfig // File output configuration

	// JSON configuration
	JSON *JSONOptions

	// Security configuration
	Security *SecurityConfig

	// Field validation configuration
	FieldValidation *FieldValidationConfig

	// Lifecycle handlers
	FatalHandler      FatalHandler
	WriteErrorHandler WriteErrorHandler

	// Extensibility
	ContextExtractors []ContextExtractor
	Hooks             *HookRegistry
	Sampling          *SamplingConfig
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
	return defaultConfig()
}

func defaultConfig() Config {
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
//	cfg.File = &dd.FileConfig{Path: "dev.log"}
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
		JSON: &internal.JSONOptions{
			PrettyPrint: false,
			Indent:      defaultJSONIndent,
			FieldNames:  internal.DefaultJSONFieldNames(),
		},
	}
}

// Clone creates a copy of the configuration.
//
// Clone behavior:
//   - Deep copy: File, JSON, Sampling, Security, Hooks configs
//   - Shallow copy: Output, Outputs, FatalHandler, WriteErrorHandler, FieldValidation
//     (io.Writer instances and function pointers are shared)
//   - ContextExtractors slice is copied but extractor instances are shared
//
// MAINTENANCE: When adding new pointer/slice/map fields to Config, you MUST
// add corresponding deep-copy logic in this method. Forgetting to do so will
// cause subtle shared-mutation bugs. Search for "Clone" in this file to find
// the switch-like copy blocks.
//
// The shallow copy behavior for io.Writer is intentional since writers are
// typically shared resources that should not be duplicated.
//
// Example:
//
//	base := dd.DefaultConfig()
//	base.Format = dd.FormatJSON
//
//	appCfg := base.Clone()
//	appCfg.File = &dd.FileConfig{Path: "app.log"}
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
		Output:            c.Output,
		FieldValidation:   c.FieldValidation,
		FatalHandler:      c.FatalHandler,
		WriteErrorHandler: c.WriteErrorHandler,
		Sampling:          c.Sampling,
	}

	// Copy Outputs slice
	if c.Outputs != nil {
		clone.Outputs = make([]io.Writer, len(c.Outputs))
		copy(clone.Outputs, c.Outputs)
	}

	// Copy File config
	if c.File != nil {
		clone.File = &FileConfig{
			Path:       c.File.Path,
			MaxSizeMB:  c.File.MaxSizeMB,
			MaxBackups: c.File.MaxBackups,
			MaxAge:     c.File.MaxAge,
			Compress:   c.File.Compress,
		}
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
