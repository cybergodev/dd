package internal

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// DefaultJSONIndent is the default indentation string for JSON output.
const DefaultJSONIndent = "  "

// MaxConvertDepth is the maximum recursion depth for ConvertValue.
// This prevents stack overflow when converting deeply nested structures.
const MaxConvertDepth = 100

// LogFormat defines the output format for log messages.
type LogFormat int8

const (
	LogFormatText LogFormat = iota
	LogFormatJSON
)

func (f LogFormat) String() string {
	switch f {
	case LogFormatText:
		return "text"
	case LogFormatJSON:
		return "json"
	default:
		return "unknown"
	}
}

type LogLevel int8

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// Pre-cached level strings to avoid allocations in hot path
var levelStrings = [5]string{
	"DEBUG",
	"INFO",
	"WARN",
	"ERROR",
	"FATAL",
}

func (l LogLevel) String() string {
	if l >= 0 && int(l) < len(levelStrings) {
		return levelStrings[l]
	}
	return "UNKNOWN"
}

func (l LogLevel) IsValid() bool {
	return l >= LevelDebug && l <= LevelFatal
}

type JSONFieldNames struct {
	Timestamp string
	Level     string
	Caller    string
	Message   string
	Fields    string
}

func (j *JSONFieldNames) IsComplete() bool {
	return j != nil &&
		j.Timestamp != "" &&
		j.Level != "" &&
		j.Caller != "" &&
		j.Message != "" &&
		j.Fields != ""
}

type JSONOptions struct {
	PrettyPrint bool
	Indent      string
	FieldNames  *JSONFieldNames
}

// IsComplexValue checks if a field value is a complex type that should be JSON-formatted.
// This is used to determine if a value needs JSON marshaling in structured logging.
// Uses type switch fast paths to avoid reflection for common types.
func IsComplexValue(v any) bool {
	if v == nil {
		return false
	}

	// Fast path: type switch for common simple types (avoids reflection)
	switch v := v.(type) {
	// Primitive types - not complex
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		complex64, complex128:
		return false

	// Time types have good String() methods
	case time.Time, time.Duration:
		return false

	// Pointer types - must be before interface types to avoid being shadowed
	// These specific pointer types implement fmt.Stringer, so they must come first
	case *time.Time:
		return false

	case *time.Duration:
		return false

	// Pointer to Stringer - use String() method
	case *interface{ String() string }:
		return false

	// Types with String() method - use their formatting
	// Note: fmt.Stringer is equivalent to interface{ String() string }
	// This must come AFTER specific pointer types that implement Stringer
	case fmt.Stringer:
		return false

	// Error interface - use Error() method
	case error:
		return false

	// Pointer types - dereference and check
	case *string, *bool,
		*int, *int8, *int16, *int32, *int64,
		*uint, *uint8, *uint16, *uint32, *uint64,
		*float32, *float64:
		return false

	// nil pointer check for interface types
	case *error:
		if v == nil {
			return false
		}
		return IsComplexValue(*v)

	case *fmt.Stringer:
		if v == nil {
			return false
		}
		return IsComplexValue(*v)

	// byte slice is common and can be represented as hex/base64
	case []byte:
		return true

		// For all other types, fall through to reflection
	}

	// Slow path: use reflection for unknown types
	val := reflect.ValueOf(v)
	kind := val.Kind()

	// Handle pointers
	if kind == reflect.Ptr {
		if val.IsNil() {
			return false
		}
		val = val.Elem()
		kind = val.Kind()
	}

	// Check for complex types that benefit from JSON formatting
	switch kind {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
		// Special case: types with String() method
		if _, ok := v.(interface{ String() string }); ok {
			return false
		}
		return true
	default:
		return false
	}
}

// ConvertValue converts any value to a JSON-serializable format.
// This is used for debug visualization and complex value formatting.
// The function has a maximum recursion depth to prevent stack overflow.
func ConvertValue(v any) any {
	return convertValueWithDepth(v, 0)
}

// convertValueWithDepth is the internal implementation with depth tracking.
func convertValueWithDepth(v any, depth int) any {
	// Check recursion depth to prevent stack overflow
	if depth > MaxConvertDepth {
		return "[MAX_DEPTH_EXCEEDED]"
	}

	if v == nil {
		return nil
	}

	val := reflect.ValueOf(v)

	if !val.IsValid() {
		return nil
	}

	// Handle pointers
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}

	// Handle interfaces
	if val.Kind() == reflect.Interface {
		if val.IsNil() {
			return nil
		}
		return convertValueWithDepth(val.Elem().Interface(), depth+1)
	}

	switch val.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return val.Interface()

	case reflect.Func:
		return fmt.Sprintf("<func:%s>", val.Type().String())

	case reflect.Chan:
		return fmt.Sprintf("<chan:%s>", val.Type().String())

	case reflect.Slice, reflect.Array:
		return convertSliceWithDepth(val, depth)

	case reflect.Map:
		return convertMapWithDepth(val, depth)

	case reflect.Struct:
		return convertStructWithDepth(val, depth)

	default:
		// Handle special common types
		if val.CanInterface() {
			iface := val.Interface()
			switch v := iface.(type) {
			case error:
				if v == nil {
					return nil
				}
				return v.Error()
			case time.Time:
				return v.Format(time.RFC3339)
			case time.Duration:
				return v.String()
			case fmt.Stringer:
				return v.String()
			}
		}

		// Try JSON marshaling as fallback
		if val.CanInterface() {
			if data, err := json.Marshal(val.Interface()); err == nil {
				var result any
				if json.Unmarshal(data, &result) == nil {
					return result
				}
			}
		}

		return fmt.Sprintf("<%s:%v>", val.Type().String(), val)
	}
}

func convertSliceWithDepth(val reflect.Value, depth int) any {
	// Check depth before processing
	if depth > MaxConvertDepth {
		return "[MAX_DEPTH_EXCEEDED]"
	}

	length := val.Len()
	if length == 0 {
		return []any{}
	}

	result := make([]any, length)
	for i := 0; i < length; i++ {
		result[i] = convertValueWithDepth(val.Index(i).Interface(), depth+1)
	}
	return result
}

func convertMapWithDepth(val reflect.Value, depth int) any {
	// Check depth before processing
	if depth > MaxConvertDepth {
		return "[MAX_DEPTH_EXCEEDED]"
	}

	if val.IsNil() {
		return nil
	}

	result := make(map[string]any)
	keys := val.MapKeys()

	for _, key := range keys {
		keyStr := fmt.Sprintf("%v", key.Interface())
		result[keyStr] = convertValueWithDepth(val.MapIndex(key).Interface(), depth+1)
	}

	return result
}

func convertStructWithDepth(val reflect.Value, depth int) any {
	// Check depth before processing
	if depth > MaxConvertDepth {
		return "[MAX_DEPTH_EXCEEDED]"
	}

	typ := val.Type()

	// Handle special types
	if val.CanInterface() {
		iface := val.Interface()
		switch v := iface.(type) {
		case error:
			if v == nil {
				return nil
			}
			return v.Error()
		case time.Time:
			return v.Format(time.RFC3339)
		case time.Duration:
			return v.String()
		case fmt.Stringer:
			return v.String()
		}
	}

	result := make(map[string]any)

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := typ.Field(i)

		if !field.CanInterface() {
			continue
		}

		fieldName := fieldType.Name
		if tag := fieldType.Tag.Get("json"); tag != "" && tag != "-" {
			tagName, _, found := strings.Cut(tag, ",")
			if found && tagName != "" {
				fieldName = tagName
			} else if !found && tag != "" {
				fieldName = tag
			}
			if fieldName == "" {
				fieldName = fieldType.Name
			}
		}

		if fieldName != "" {
			result[fieldName] = convertValueWithDepth(field.Interface(), depth+1)
		}
	}

	return result
}
