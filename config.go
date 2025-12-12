package dd

import (
	"fmt"
	"io"
	"os"
	"time"
)

// JSONOptions holds JSON-specific configuration options
type JSONOptions struct {
	PrettyPrint bool
	Indent      string
	FieldNames  *JSONFieldNames
}

// JSONFieldNames allows customization of JSON field names
type JSONFieldNames struct {
	Timestamp string
	Level     string
	Caller    string
	Message   string
	Fields    string
}

func DefaultJSONOptions() *JSONOptions {
	return &JSONOptions{
		PrettyPrint: false,
		Indent:      "  ",
		FieldNames:  DefaultJSONFieldNames(),
	}
}

func DefaultJSONFieldNames() *JSONFieldNames {
	return &JSONFieldNames{
		Timestamp: "timestamp",
		Level:     "level",
		Caller:    "caller",
		Message:   "message",
		Fields:    "fields",
	}
}

type LoggerConfig struct {
	Level          LogLevel
	Format         LogFormat
	TimeFormat     string
	IncludeCaller  bool
	IncludeTime    bool
	IncludeLevel   bool
	FullPath       bool
	DynamicCaller  bool
	Writers        []io.Writer
	SecurityConfig *SecurityConfig
	FatalHandler   FatalHandler
	JSON           *JSONOptions
}

func DefaultConfig() *LoggerConfig {
	return &LoggerConfig{
		Level:          LevelInfo, // Production-friendly default
		Format:         FormatText,
		TimeFormat:     time.RFC3339,
		IncludeCaller:  false, // Performance-friendly default
		IncludeTime:    true,
		IncludeLevel:   true,
		FullPath:       false, // Cleaner output
		DynamicCaller:  false, // Performance-friendly default
		Writers:        nil,
		SecurityConfig: DefaultSecurityConfig(), // Always include security config
		FatalHandler:   nil,
	}
}

func DevelopmentConfig() *LoggerConfig {
	return &LoggerConfig{
		Level:          LevelDebug,
		Format:         FormatText,
		TimeFormat:     "15:04:05.000",
		IncludeCaller:  true,
		IncludeTime:    true,
		IncludeLevel:   true,
		FullPath:       false,
		DynamicCaller:  true,
		Writers:        nil,
		SecurityConfig: nil,
		FatalHandler:   nil,
	}
}

func JSONConfig() *LoggerConfig {
	return &LoggerConfig{
		Level:          LevelDebug,
		Format:         FormatJSON,
		TimeFormat:     time.RFC3339,
		IncludeCaller:  true,
		IncludeTime:    true,
		IncludeLevel:   true,
		FullPath:       false,
		DynamicCaller:  false,
		Writers:        nil,
		SecurityConfig: nil,
		FatalHandler:   nil,
		JSON:           DefaultJSONOptions(),
	}
}

func (c *LoggerConfig) Clone() *LoggerConfig {
	if c == nil {
		return DefaultConfig()
	}

	clone := &LoggerConfig{
		Level:         c.Level,
		Format:        c.Format,
		TimeFormat:    c.TimeFormat,
		IncludeCaller: c.IncludeCaller,
		IncludeTime:   c.IncludeTime,
		IncludeLevel:  c.IncludeLevel,
		FullPath:      c.FullPath,
		DynamicCaller: c.DynamicCaller,
		FatalHandler:  c.FatalHandler,
	}

	if len(c.Writers) > 0 {
		clone.Writers = make([]io.Writer, len(c.Writers))
		copy(clone.Writers, c.Writers)
	}

	if c.SecurityConfig != nil {
		clone.SecurityConfig = &SecurityConfig{
			MaxMessageSize: c.SecurityConfig.MaxMessageSize,
			MaxWriters:     c.SecurityConfig.MaxWriters,
		}
		if c.SecurityConfig.SensitiveFilter != nil {
			clone.SecurityConfig.SensitiveFilter = c.SecurityConfig.SensitiveFilter.Clone()
		}
	}

	if c.JSON != nil {
		clone.JSON = &JSONOptions{
			PrettyPrint: c.JSON.PrettyPrint,
			Indent:      c.JSON.Indent,
		}
		if c.JSON.FieldNames != nil {
			clone.JSON.FieldNames = &JSONFieldNames{
				Timestamp: c.JSON.FieldNames.Timestamp,
				Level:     c.JSON.FieldNames.Level,
				Caller:    c.JSON.FieldNames.Caller,
				Message:   c.JSON.FieldNames.Message,
				Fields:    c.JSON.FieldNames.Fields,
			}
		}
	}

	return clone
}

func (c *LoggerConfig) Validate() error {
	if c == nil {
		return ErrNilConfig
	}

	if c.Level < LevelDebug || c.Level > LevelFatal {
		return fmt.Errorf("%w: %d", ErrInvalidLevel, c.Level)
	}

	if c.Format != FormatText && c.Format != FormatJSON {
		return fmt.Errorf("%w: %d", ErrInvalidFormat, c.Format)
	}

	if c.IncludeTime && c.TimeFormat == "" {
		c.TimeFormat = time.RFC3339
	}

	if len(c.Writers) == 0 {
		c.Writers = []io.Writer{os.Stdout}
	}

	if c.SecurityConfig == nil {
		c.SecurityConfig = DefaultSecurityConfig()
	} else {
		if c.SecurityConfig.MaxMessageSize <= 0 {
			c.SecurityConfig.MaxMessageSize = defaultMaxMessageSize
		}
		if c.SecurityConfig.MaxWriters <= 0 {
			c.SecurityConfig.MaxWriters = defaultMaxWriters
		}
	}

	return nil
}

func (c *LoggerConfig) WithFile(filename string, config FileWriterConfig) (*LoggerConfig, error) {
	fileWriter, err := NewFileWriter(filename, config)
	if err != nil {
		return c, fmt.Errorf("failed to create file writer: %w", err)
	}

	if c.Writers == nil {
		c.Writers = []io.Writer{os.Stdout}
	}
	c.Writers = append(c.Writers, fileWriter)
	return c, nil
}

func (c *LoggerConfig) WithFileOnly(filename string, config FileWriterConfig) (*LoggerConfig, error) {
	fileWriter, err := NewFileWriter(filename, config)
	if err != nil {
		return c, fmt.Errorf("failed to create file writer: %w", err)
	}

	c.Writers = []io.Writer{fileWriter}
	return c, nil
}

func (c *LoggerConfig) WithWriter(writer io.Writer) *LoggerConfig {
	if writer == nil {
		return c
	}

	if c.Writers == nil {
		c.Writers = []io.Writer{os.Stdout}
	}
	c.Writers = append(c.Writers, writer)
	return c
}

func (c *LoggerConfig) WithLevel(level LogLevel) *LoggerConfig {
	c.Level = level
	return c
}

func (c *LoggerConfig) WithFormat(format LogFormat) *LoggerConfig {
	c.Format = format
	return c
}

func (c *LoggerConfig) WithCaller(enabled bool) *LoggerConfig {
	c.IncludeCaller = enabled
	return c
}

func (c *LoggerConfig) WithDynamicCaller(enabled bool) *LoggerConfig {
	c.DynamicCaller = enabled
	return c
}

func (c *LoggerConfig) DisableFiltering() *LoggerConfig {
	if c.SecurityConfig == nil {
		c.SecurityConfig = &SecurityConfig{}
	}
	c.SecurityConfig.SensitiveFilter = nil
	return c
}

func (c *LoggerConfig) EnableBasicFiltering() *LoggerConfig {
	if c.SecurityConfig == nil {
		c.SecurityConfig = &SecurityConfig{}
	}
	c.SecurityConfig.SensitiveFilter = NewBasicSensitiveDataFilter()
	return c
}

func (c *LoggerConfig) EnableFullFiltering() *LoggerConfig {
	if c.SecurityConfig == nil {
		c.SecurityConfig = &SecurityConfig{}
	}
	c.SecurityConfig.SensitiveFilter = NewSensitiveDataFilter()
	return c
}

func (c *LoggerConfig) WithFilter(filter *SensitiveDataFilter) *LoggerConfig {
	if c.SecurityConfig == nil {
		c.SecurityConfig = &SecurityConfig{}
	}
	c.SecurityConfig.SensitiveFilter = filter
	return c
}
