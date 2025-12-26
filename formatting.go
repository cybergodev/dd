package dd

import (
	"fmt"
	"strings"
	"time"

	"github.com/cybergodev/dd/internal/caller"
	"github.com/cybergodev/dd/internal/jsonformat"
	"github.com/cybergodev/dd/internal/logformat"
	"github.com/cybergodev/dd/internal/types"
)

// MessageFormatter handles the formatting of log messages with optimized, unified logic.
type MessageFormatter struct {
	format        LogFormat
	timeFormat    string
	includeCaller bool
	includeTime   bool
	includeLevel  bool
	fullPath      bool
	dynamicCaller bool
	jsonConfig    *JSONOptions
}

// newMessageFormatter creates a new message formatter with the given configuration
func newMessageFormatter(config *LoggerConfig) *MessageFormatter {
	return &MessageFormatter{
		format:        config.Format,
		timeFormat:    config.TimeFormat,
		includeCaller: config.IncludeCaller,
		includeTime:   config.IncludeTime,
		includeLevel:  config.IncludeLevel,
		fullPath:      config.FullPath,
		dynamicCaller: config.DynamicCaller,
		jsonConfig:    config.JSON,
	}
}

// formatMessage formats a log message according to the configured format (unified entry point)
func (f *MessageFormatter) formatMessage(level LogLevel, callerDepth int, args ...any) string {
	message := fmt.Sprint(args...)
	return f.formatWithMessage(level, callerDepth, message, nil)
}

// formatMessagef formats a formatted log message (unified entry point)
func (f *MessageFormatter) formatMessagef(level LogLevel, callerDepth int, format string, args ...any) string {
	message := fmt.Sprintf(format, args...)
	return f.formatWithMessage(level, callerDepth, message, nil)
}

// formatMessageWith formats a structured log message with fields (unified entry point)
func (f *MessageFormatter) formatMessageWith(level LogLevel, callerDepth int, msg string, fields []Field) string {
	return f.formatWithMessage(level, callerDepth, msg, fields)
}

// formatWithMessage is the unified formatting implementation
func (f *MessageFormatter) formatWithMessage(level LogLevel, callerDepth int, message string, fields []Field) string {
	// Adjust caller depth if dynamic detection is enabled
	if f.dynamicCaller {
		callerDepth = f.adjustCallerDepth(callerDepth)
	}

	switch f.format {
	case FormatJSON:
		return f.formatJSON(level, callerDepth, message, fields)
	default:
		return f.formatText(level, callerDepth, message, fields)
	}
}

// formatText handles text formatting with unified logic
func (f *MessageFormatter) formatText(level LogLevel, callerDepth int, message string, fields []Field) string {
	baseMsg := logformat.FormatMessage(
		types.LogLevel(level),
		f.includeTime,
		f.timeFormat,
		f.includeLevel,
		f.includeCaller,
		callerDepth,
		f.fullPath,
		message,
	)

	// Add fields if present
	if len(fields) > 0 {
		if fieldsStr := formatFields(fields); fieldsStr != "" {
			return baseMsg + " " + fieldsStr
		}
	}

	return baseMsg
}

// formatJSON handles JSON formatting with unified logic
func (f *MessageFormatter) formatJSON(level LogLevel, callerDepth int, message string, fields []Field) string {
	fieldNames := f.getJSONFieldNames()

	// Pre-calculate capacity for better performance
	capacity := 1 // message always included
	if f.includeTime {
		capacity++
	}
	if f.includeLevel {
		capacity++
	}
	if f.includeCaller {
		capacity++
	}
	if len(fields) > 0 {
		capacity++
	}

	entry := make(map[string]any, capacity)

	// Add timestamp if enabled
	if f.includeTime {
		entry[fieldNames.Timestamp] = time.Now().Format(f.timeFormat)
	}

	// Add level if enabled
	if f.includeLevel {
		entry[fieldNames.Level] = level.String()
	}

	// Add caller if enabled
	if f.includeCaller {
		if callerInfo := caller.GetCaller(callerDepth, f.fullPath); callerInfo != "" {
			entry[fieldNames.Caller] = callerInfo
		}
	}

	// Add message
	entry[fieldNames.Message] = message

	// Add structured fields if present
	if len(fields) > 0 {
		fieldsMap := make(map[string]any, len(fields))
		for _, field := range fields {
			fieldsMap[field.Key] = field.Value
		}
		entry[fieldNames.Fields] = fieldsMap
	}

	return jsonformat.FormatJSON(entry, f.getJSONOptions())
}

// getJSONFieldNames returns the JSON field names configuration (thread-safe)
func (f *MessageFormatter) getJSONFieldNames() *JSONFieldNames {
	if f.jsonConfig != nil && f.jsonConfig.FieldNames != nil {
		return f.jsonConfig.FieldNames
	}
	return DefaultJSONFieldNames()
}

// getJSONOptions returns the JSON formatting options (thread-safe)
func (f *MessageFormatter) getJSONOptions() *types.JSONOptions {
	if f.jsonConfig == nil {
		return &types.JSONOptions{
			PrettyPrint: false,
			Indent:      DefaultJSONIndent,
		}
	}

	return &types.JSONOptions{
		PrettyPrint: f.jsonConfig.PrettyPrint,
		Indent:      f.jsonConfig.Indent,
	}
}

// adjustCallerDepth adjusts the caller depth based on dynamic caller detection.
// This method looks for the first non-dd package in the call stack.
func (f *MessageFormatter) adjustCallerDepth(baseDepth int) int {
	// Simple dynamic caller detection - look for first non-dd package
	for depth := baseDepth; depth < baseDepth+5; depth++ {
		if callerInfo := caller.GetCaller(depth, true); callerInfo != "" {
			if !strings.Contains(callerInfo, "github.com/cybergodev/dd") {
				return depth
			}
		}
	}

	return baseDepth
}
