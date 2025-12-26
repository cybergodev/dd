package dd

import (
	"fmt"
	"io"
	"os"
	"time"
)

const defFile = DefaultLogFile

type Options struct {
	Level             LogLevel
	Format            LogFormat
	Console           bool
	File              string
	FileConfig        FileWriterConfig
	IncludeCaller     bool
	FullPath          bool
	DynamicCaller     bool
	TimeFormat        string
	FilterLevel       string
	CustomFilter      *SensitiveDataFilter
	JSONOptions       *JSONOptions
	AdditionalWriters []io.Writer
}

func NewWithOptions(opts Options) (*Logger, error) {
	// Validate and normalize options
	if opts.Level < LevelDebug || opts.Level > LevelFatal {
		opts.Level = LevelDebug
	}
	if opts.Format != FormatText && opts.Format != FormatJSON {
		opts.Format = FormatText
	}
	if opts.TimeFormat == "" {
		opts.TimeFormat = DefaultTimeFormat
	}

	// Pre-allocate writers slice capacity
	writerCap := 0
	if opts.Console {
		writerCap++
	}
	if opts.File != "" {
		writerCap++
	}
	if len(opts.AdditionalWriters) > 0 {
		writerCap += len(opts.AdditionalWriters)
	}
	if writerCap == 0 {
		writerCap = 1 // At least one default writer
	}

	config := &LoggerConfig{
		Level:         opts.Level,
		Format:        opts.Format,
		TimeFormat:    opts.TimeFormat,
		IncludeCaller: opts.IncludeCaller,
		FullPath:      opts.FullPath,
		DynamicCaller: opts.DynamicCaller,
		IncludeTime:   true,
		IncludeLevel:  true,
		Writers:       make([]io.Writer, 0, writerCap),
	}

	// Add console output
	if opts.Console {
		config.Writers = append(config.Writers, os.Stdout)
	}

	// Add file output
	if opts.File != "" {
		fileWriter, err := NewFileWriter(opts.File, opts.FileConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create file writer: %w", err)
		}
		config.Writers = append(config.Writers, fileWriter)
	}

	// Add additional writers
	if len(opts.AdditionalWriters) > 0 {
		config.Writers = append(config.Writers, opts.AdditionalWriters...)
	}

	// Ensure at least one writer
	if len(config.Writers) == 0 {
		config.Writers = []io.Writer{os.Stdout}
	}

	// Set security configuration
	config.SecurityConfig = DefaultSecurityConfig()
	if opts.CustomFilter != nil {
		config.SecurityConfig.SensitiveFilter = opts.CustomFilter
	} else {
		switch opts.FilterLevel {
		case "none":
			config.SecurityConfig.SensitiveFilter = nil
		case "basic":
			config.SecurityConfig.SensitiveFilter = NewBasicSensitiveDataFilter()
		case "full":
			config.SecurityConfig.SensitiveFilter = NewSensitiveDataFilter()
		case "":
			// Default: no filter
		default:
			return nil, fmt.Errorf("%w: %s (must be 'none', 'basic', or 'full')", ErrInvalidFilterLevel, opts.FilterLevel)
		}
	}

	// Set JSON configuration
	if opts.Format == FormatJSON {
		if opts.JSONOptions != nil {
			config.JSON = opts.JSONOptions
		} else {
			config.JSON = DefaultJSONOptions()
		}
	}

	return New(config)
}

func getFilename(filename []string) string {
	if len(filename) > 0 && filename[0] != "" {
		return filename[0]
	}
	return defFile
}

func fallbackLogger() *Logger {
	// Create simplest fallback logger that always succeeds
	config := &LoggerConfig{
		Level:          LevelInfo,
		Format:         FormatText,
		TimeFormat:     time.RFC3339,
		IncludeCaller:  false,
		IncludeTime:    true,
		IncludeLevel:   true,
		FullPath:       false,
		DynamicCaller:  false,
		Writers:        []io.Writer{os.Stdout},
		SecurityConfig: DefaultSecurityConfig(),
		FatalHandler:   nil,
	}

	logger, err := New(config)
	if err != nil {
		// If basic configuration fails, create a minimal logger
		fallbackConfig := DefaultConfig()
		fallbackConfig.Writers = []io.Writer{os.Stderr}
		fallback, fallbackErr := New(fallbackConfig)
		if fallbackErr != nil {
			// Last resort: return nil and let caller handle
			return nil
		}
		return fallback
	}
	return logger
}

func ToFile(filename ...string) *Logger {
	logger, err := NewWithOptions(Options{
		Console: false,
		File:    getFilename(filename),
	})
	if err != nil {
		return fallbackLogger()
	}
	return logger
}

func ToConsole() *Logger {
	logger, err := NewWithOptions(Options{Console: true})
	if err != nil {
		return fallbackLogger()
	}
	return logger
}

func ToJSONFile(filename ...string) *Logger {
	logger, err := NewWithOptions(Options{
		Format:  FormatJSON,
		Console: false,
		File:    getFilename(filename),
	})
	if err != nil {
		return fallbackLogger()
	}
	return logger
}

func ToAll(filename ...string) *Logger {
	logger, err := NewWithOptions(Options{
		Console: true,
		File:    getFilename(filename),
	})
	if err != nil {
		return fallbackLogger()
	}
	return logger
}
