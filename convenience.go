package dd

import (
	"context"
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
	// 验证和标准化选项
	if opts.Level < LevelDebug || opts.Level > LevelFatal {
		opts.Level = LevelDebug
	}
	if opts.Format != FormatText && opts.Format != FormatJSON {
		opts.Format = FormatText
	}
	if opts.TimeFormat == "" {
		opts.TimeFormat = time.RFC3339
	}

	// 预分配写入器切片容量
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
		writerCap = 1 // 至少需要一个默认写入器
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

	// 添加控制台输出
	if opts.Console {
		config.Writers = append(config.Writers, os.Stdout)
	}

	// 添加文件输出
	if opts.File != "" {
		fileWriter, err := NewFileWriter(opts.File, opts.FileConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create file writer: %w", err)
		}
		config.Writers = append(config.Writers, fileWriter)
	}

	// 添加额外的写入器
	if len(opts.AdditionalWriters) > 0 {
		config.Writers = append(config.Writers, opts.AdditionalWriters...)
	}

	// 确保至少有一个写入器
	if len(config.Writers) == 0 {
		config.Writers = []io.Writer{os.Stdout}
	}

	// 设置安全配置
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
			// 默认不设置过滤器
		default:
			return nil, fmt.Errorf("%w: %s (must be 'none', 'basic', or 'full')", ErrInvalidFilterLevel, opts.FilterLevel)
		}
	}

	// 设置 JSON 配置
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
	// 创建最简单的回退日志器，确保总是成功
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
		// 如果连基本配置都失败，创建最小化的日志器
		ctx, cancel := context.WithCancel(context.Background())
		fallback := &Logger{
			format:        FormatText,
			timeFormat:    time.RFC3339,
			callerDepth:   defaultCallerDepth,
			includeCaller: false,
			includeTime:   true,
			includeLevel:  true,
			fullPath:      false,
			writers:       []io.Writer{os.Stderr},
			ctx:           ctx,
			cancel:        cancel,
		}
		fallback.level.Store(int32(LevelInfo))
		fallback.securityConfig.Store(DefaultSecurityConfig())
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
	logger, _ := NewWithOptions(Options{Console: true})
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
