package dd

import (
	"fmt"
	"io"
	"os"
	"time"
)

const defFile = "logs/app.log"

type Options struct {
	Level             LogLevel
	Format            LogFormat
	Console           bool
	File              string
	FileConfig        *FileWriterConfig
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
	if opts.Level < LevelDebug || opts.Level > LevelFatal {
		opts.Level = LevelDebug
	}
	if opts.Format != FormatText && opts.Format != FormatJSON {
		opts.Format = FormatText
	}
	if opts.TimeFormat == "" {
		opts.TimeFormat = time.RFC3339
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
		Writers:       make([]io.Writer, 0),
	}

	if opts.Console {
		config.Writers = append(config.Writers, os.Stdout)
	}

	if opts.File != "" {
		fileWriter, err := NewFileWriter(opts.File, opts.FileConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create file writer: %w", err)
		}
		config.Writers = append(config.Writers, fileWriter)
	}

	if len(opts.AdditionalWriters) > 0 {
		config.Writers = append(config.Writers, opts.AdditionalWriters...)
	}

	if len(config.Writers) == 0 {
		config.Writers = []io.Writer{os.Stdout}
	}

	if config.SecurityConfig == nil {
		config.SecurityConfig = DefaultSecurityConfig()
	}

	if opts.CustomFilter != nil {
		config.SecurityConfig.SensitiveFilter = opts.CustomFilter
	} else if opts.FilterLevel != "" {
		switch opts.FilterLevel {
		case "none":
			config.SecurityConfig.SensitiveFilter = nil
		case "basic":
			config.SecurityConfig.SensitiveFilter = NewBasicSensitiveDataFilter()
		case "full":
			config.SecurityConfig.SensitiveFilter = NewSensitiveDataFilter()
		default:
			return nil, fmt.Errorf("invalid filter level: %s (must be 'none', 'basic', or 'full')", opts.FilterLevel)
		}
	}

	if opts.Format == FormatJSON {
		if opts.JSONOptions != nil {
			config.JSON = opts.JSONOptions
		} else {
			config.JSON = DefaultJSONOptions()
		}
	}

	return New(config)
}

func ToFile(filename ...string) *Logger {
	file := defFile
	if len(filename) > 0 && filename[0] != "" {
		file = filename[0]
	}
	logger, err := NewWithOptions(Options{
		Console: false,
		File:    file,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dd: create file logger '%s': %v, using console fallback\n", file, err)
		fallback, _ := NewWithOptions(Options{Console: true})
		return fallback
	}
	return logger
}

func ToConsole() *Logger {
	logger, err := NewWithOptions(Options{Console: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dd: create console logger: %v, using fallback\n", err)
		fallback, _ := New(DefaultConfig())
		return fallback
	}
	return logger
}

func ToJSONFile(filename ...string) *Logger {
	file := defFile
	if len(filename) > 0 && filename[0] != "" {
		file = filename[0]
	}
	logger, err := NewWithOptions(Options{
		Format:  FormatJSON,
		Console: false,
		File:    file,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dd: create JSON file logger '%s': %v, using console fallback\n", file, err)
		fallback, _ := NewWithOptions(Options{Console: true})
		return fallback
	}
	return logger
}

func ToAll(filename ...string) *Logger {
	file := defFile
	if len(filename) > 0 && filename[0] != "" {
		file = filename[0]
	}
	logger, err := NewWithOptions(Options{
		Console: true,
		File:    file,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dd: create logger with file '%s': %v, using console fallback\n", file, err)
		fallback, _ := NewWithOptions(Options{Console: true})
		return fallback
	}
	return logger
}
