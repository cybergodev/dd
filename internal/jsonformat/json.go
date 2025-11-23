package jsonformat

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"time"
)

type LogLevel int8

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

type JSONFieldNames struct {
	Timestamp string
	Level     string
	Caller    string
	Message   string
	Fields    string
}

type JSONOptions struct {
	PrettyPrint bool
	Indent      string
	FieldNames  *JSONFieldNames
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

func mergeWithDefaults(f *JSONFieldNames) *JSONFieldNames {
	if f == nil {
		return DefaultJSONFieldNames()
	}

	result := &JSONFieldNames{
		Timestamp: f.Timestamp,
		Level:     f.Level,
		Caller:    f.Caller,
		Message:   f.Message,
		Fields:    f.Fields,
	}

	if result.Timestamp == "" {
		result.Timestamp = "timestamp"
	}
	if result.Level == "" {
		result.Level = "level"
	}
	if result.Caller == "" {
		result.Caller = "caller"
	}
	if result.Message == "" {
		result.Message = "message"
	}
	if result.Fields == "" {
		result.Fields = "fields"
	}

	return result
}

func FormatMessage(
	level LogLevel,
	includeTime bool,
	timeFormat string,
	includeLevel bool,
	includeCaller bool,
	callerDepth int,
	fullPath bool,
	message string,
	fields map[string]any,
) (string, error) {
	opts := &JSONOptions{
		PrettyPrint: false,
		Indent:      "  ",
		FieldNames:  DefaultJSONFieldNames(),
	}

	return FormatMessageWithOptions(
		level,
		includeTime,
		timeFormat,
		includeLevel,
		includeCaller,
		callerDepth,
		fullPath,
		message,
		fields,
		opts,
	)
}

func FormatMessageWithOptions(
	level LogLevel,
	includeTime bool,
	timeFormat string,
	includeLevel bool,
	includeCaller bool,
	callerDepth int,
	fullPath bool,
	message string,
	fields map[string]any,
	opts *JSONOptions,
) (string, error) {
	if opts == nil {
		opts = &JSONOptions{
			PrettyPrint: false,
			Indent:      "  ",
			FieldNames:  DefaultJSONFieldNames(),
		}
	}

	opts.FieldNames = mergeWithDefaults(opts.FieldNames)

	capacity := 4
	if len(fields) > 0 {
		capacity = 5
	}
	entry := make(map[string]any, capacity)

	if includeTime {
		entry[opts.FieldNames.Timestamp] = time.Now().Format(timeFormat)
	}

	if includeLevel {
		entry[opts.FieldNames.Level] = LogLevel(level).String()
	}

	if includeCaller {
		if caller := getCaller(callerDepth, fullPath); caller != "" {
			entry[opts.FieldNames.Caller] = caller
		}
	}

	entry[opts.FieldNames.Message] = message

	if len(fields) > 0 {
		entry[opts.FieldNames.Fields] = fields
	}

	var data []byte
	var err error

	if opts.PrettyPrint {
		data, err = json.MarshalIndent(entry, "", opts.Indent)
	} else {
		data, err = json.Marshal(entry)
	}

	if err != nil {
		return "", fmt.Errorf("failed to marshal log entry: %w", err)
	}

	return string(data), nil
}

func getCaller(callerDepth int, fullPath bool) string {
	_, file, line, ok := runtime.Caller(callerDepth)
	if !ok {
		return ""
	}

	if !fullPath {
		file = filepath.Base(file)
	}

	return fmt.Sprintf("%s:%d", file, line)
}
