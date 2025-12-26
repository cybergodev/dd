package jsonformat

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cybergodev/dd/internal/caller"
	"github.com/cybergodev/dd/internal/types"
)

func DefaultJSONFieldNames() *types.JSONFieldNames {
	return &types.JSONFieldNames{
		Timestamp: "timestamp",
		Level:     "level",
		Caller:    "caller",
		Message:   "message",
		Fields:    "fields",
	}
}

func mergeWithDefaults(f *types.JSONFieldNames) *types.JSONFieldNames {
	if f == nil {
		return DefaultJSONFieldNames()
	}

	result := &types.JSONFieldNames{
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
	level types.LogLevel,
	includeTime bool,
	timeFormat string,
	includeLevel bool,
	includeCaller bool,
	callerDepth int,
	fullPath bool,
	message string,
	fields map[string]any,
) (string, error) {
	opts := &types.JSONOptions{
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
	level types.LogLevel,
	includeTime bool,
	timeFormat string,
	includeLevel bool,
	includeCaller bool,
	callerDepth int,
	fullPath bool,
	message string,
	fields map[string]any,
	opts *types.JSONOptions,
) (string, error) {
	if opts == nil {
		opts = &types.JSONOptions{
			PrettyPrint: false,
			Indent:      "  ",
			FieldNames:  DefaultJSONFieldNames(),
		}
	}

	opts.FieldNames = mergeWithDefaults(opts.FieldNames)

	capacity := 1 // message always included
	if includeTime {
		capacity++
	}
	if includeLevel {
		capacity++
	}
	if includeCaller {
		capacity++
	}
	if len(fields) > 0 {
		capacity++
	}

	entry := make(map[string]any, capacity)

	if includeTime {
		entry[opts.FieldNames.Timestamp] = time.Now().Format(timeFormat)
	}

	if includeLevel {
		entry[opts.FieldNames.Level] = level.String()
	}

	if includeCaller {
		if callerInfo := caller.GetCaller(callerDepth, fullPath); callerInfo != "" {
			entry[opts.FieldNames.Caller] = callerInfo
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

// FormatJSON formats a map as JSON with the given options
func FormatJSON(entry map[string]any, opts *types.JSONOptions) string {
	if opts == nil {
		opts = &types.JSONOptions{
			PrettyPrint: false,
			Indent:      "  ",
		}
	}

	var data []byte
	var err error

	if opts.PrettyPrint {
		data, err = json.MarshalIndent(entry, "", opts.Indent)
	} else {
		data, err = json.Marshal(entry)
	}

	if err != nil {
		return fmt.Sprintf(`{"error":"json marshal failed: %v"}`, err)
	}

	return string(data)
}
