package dd

import (
	"fmt"
	"io"

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

	// Collect writers
	var writers []io.Writer

	// Add single output writer
	if c.Output != nil {
		writers = append(writers, c.Output)
	}

	// Add multiple output writers
	for _, w := range c.Outputs {
		if w != nil {
			writers = append(writers, w)
		}
	}

	// Handle file output
	if c.File != nil && c.File.Path != "" {
		fileWriter, err := c.createFileWriter()
		if err != nil {
			return nil, err
		}
		writers = append(writers, fileWriter)
	}

	// Default to stdout if no writers configured
	if len(writers) == 0 {
		writers = []io.Writer{defaultOutput}
	}

	loggerConfig.writers = writers

	return newFromInternalConfig(loggerConfig)
}

// createFileWriter creates a FileWriter from FileConfig.
func (c Config) createFileWriter() (*FileWriter, error) {
	if c.File == nil || c.File.Path == "" {
		return nil, nil
	}

	config := FileWriterConfig{
		MaxSizeMB:  c.File.MaxSizeMB,
		MaxBackups: c.File.MaxBackups,
		MaxAge:     c.File.MaxAge,
		Compress:   c.File.Compress,
	}

	return NewFileWriter(c.File.Path, config)
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
	writerCount := 0
	if c.Output != nil {
		writerCount++
	}
	writerCount += len(c.Outputs)
	if c.File != nil && c.File.Path != "" {
		writerCount++
	}

	// Validate writer count
	if writerCount > maxWriterCount {
		return fmt.Errorf("%w: %d writers configured, maximum is %d", ErrMaxWritersExceeded, writerCount, maxWriterCount)
	}

	// Check for nil writers in Outputs slice
	for i, w := range c.Outputs {
		if w == nil {
			return fmt.Errorf("writer at Outputs[%d] is nil", i)
		}
	}

	return nil
}
