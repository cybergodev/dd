package dd

import (
	"fmt"
	"io"
	"os"

	"github.com/cybergodev/dd/internal"
)

// internalConfig is used internally to create a logger.
// It holds processed configuration ready for logger initialization.
type internalConfig struct {
	level             LogLevel
	format            LogFormat
	timeFormat        string
	includeTime       bool
	includeLevel      bool
	fullPath          bool
	dynamicCaller     bool
	writers           []io.Writer
	json              *JSONOptions
	securityConfig    *SecurityConfig
	fieldValidation   *FieldValidationConfig
	fatalHandler      FatalHandler
	writeErrorHandler WriteErrorHandler
	contextExtractors []ContextExtractor
	hooks             *HookRegistry
	sampling          *SamplingConfig
}

// build creates a new Logger from the configuration.
// This is an internal method used by dd.New().
func (c Config) build() (*Logger, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	// Build internal config
	loggerConfig := &internalConfig{
		level:             c.Level,
		format:            c.Format,
		timeFormat:        c.TimeFormat,
		includeTime:       c.IncludeTime,
		includeLevel:      c.IncludeLevel,
		fullPath:          c.FullPath,
		dynamicCaller:     c.DynamicCaller,
		securityConfig:    c.Security,
		fieldValidation:   c.FieldValidation,
		fatalHandler:      c.FatalHandler,
		writeErrorHandler: c.WriteErrorHandler,
		contextExtractors: c.ContextExtractors,
		hooks:             c.Hooks,
		sampling:          c.Sampling,
	}

	// Handle JSON options
	if c.Format == FormatJSON && c.JSON != nil {
		loggerConfig.json = c.JSON
	} else if c.Format == FormatJSON {
		loggerConfig.json = &internal.JSONOptions{
			PrettyPrint: false,
			Indent:      defaultJSONIndent,
			FieldNames:  internal.DefaultJSONFieldNames(),
		}
	}

	// Collect writers from Targets
	var writers []io.Writer
	if len(c.Targets) > 0 {
		for _, t := range c.Targets {
			w, err := t.resolve()
			if err != nil {
				return nil, err
			}
			if w != nil {
				writers = append(writers, w)
			}
		}
	}

	// Default to stdout if no writers configured
	if len(writers) == 0 {
		writers = []io.Writer{defaultOutput}
	}

	loggerConfig.writers = writers

	return newFromInternalConfig(loggerConfig)
}

// resolve converts an OutputTarget to an io.Writer.
func (t OutputTarget) resolve() (io.Writer, error) {
	switch t.Type {
	case OutputConsole:
		return os.Stdout, nil
	case OutputFile:
		if t.Path == "" {
			return nil, ErrEmptyFilePath
		}
		return NewFileWriter(t.Path, FileWriterConfig{
			MaxSizeMB:  t.MaxSizeMB,
			MaxBackups: t.MaxBackups,
			MaxAge:     t.MaxAge,
			Compress:   t.Compress,
		})
	case OutputCustom:
		if t.Writer == nil {
			return nil, ErrNilWriter
		}
		return t.Writer, nil
	default:
		return nil, fmt.Errorf("unknown output type: %d", t.Type)
	}
}

// Validate validates the configuration and returns an error if any field is invalid.
// Call this before passing a Config to New() to catch configuration errors early.
func (c Config) Validate() error {
	// Validate log level
	if c.Level < LevelDebug || c.Level > LevelFatal {
		return fmt.Errorf("%w: %d (valid range: %d-%d)", ErrInvalidLevel, c.Level, LevelDebug, LevelFatal)
	}

	// Validate format
	if c.Format != FormatText && c.Format != FormatJSON {
		return fmt.Errorf("%w: %d (valid: %d=Text, %d=JSON)", ErrInvalidFormat, c.Format, FormatText, FormatJSON)
	}

	// Validate time format
	if c.IncludeTime && c.TimeFormat != "" {
		if err := internal.ValidateTimeFormat(c.TimeFormat); err != nil {
			return err
		}
	}

	// Count total writers
	writerCount := len(c.Targets)

	// Validate writer count
	if writerCount > maxWriterCount {
		return fmt.Errorf("%w: %d writers configured, maximum is %d", ErrMaxWritersExceeded, writerCount, maxWriterCount)
	}

	// Validate Targets
	for _, t := range c.Targets {
		if t.Type == OutputCustom && t.Writer == nil {
			return fmt.Errorf("OutputTarget with OutputCustom type has nil Writer")
		}
		if t.Type == OutputFile && t.Path == "" {
			return fmt.Errorf("OutputTarget with OutputFile type has empty Path")
		}
	}

	return nil
}
