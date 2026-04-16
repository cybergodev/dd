package dd

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/cybergodev/dd/internal"
)

// Field represents a structured log field with a key-value pair.
// Type alias to internal.Field for API compatibility.
type Field = internal.Field

// Any creates a field with any value.
func Any(key string, value any) Field {
	return Field{Key: key, Value: value}
}

// String creates a field with a string value.
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

// Int creates a field with an int value.
func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

// Int8 creates a field with an int8 value.
func Int8(key string, value int8) Field {
	return Field{Key: key, Value: value}
}

// Int16 creates a field with an int16 value.
func Int16(key string, value int16) Field {
	return Field{Key: key, Value: value}
}

// Int32 creates a field with an int32 value.
func Int32(key string, value int32) Field {
	return Field{Key: key, Value: value}
}

// Int64 creates a field with an int64 value.
func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

// Uint creates a field with a uint value.
func Uint(key string, value uint) Field {
	return Field{Key: key, Value: value}
}

// Uint8 creates a field with a uint8 value.
func Uint8(key string, value uint8) Field {
	return Field{Key: key, Value: value}
}

// Uint16 creates a field with a uint16 value.
func Uint16(key string, value uint16) Field {
	return Field{Key: key, Value: value}
}

// Uint32 creates a field with a uint32 value.
func Uint32(key string, value uint32) Field {
	return Field{Key: key, Value: value}
}

// Uint64 creates a field with a uint64 value.
func Uint64(key string, value uint64) Field {
	return Field{Key: key, Value: value}
}

// Float32 creates a field with a float32 value.
func Float32(key string, value float32) Field {
	return Field{Key: key, Value: value}
}

// Float64 creates a field with a float64 value.
func Float64(key string, value float64) Field {
	return Field{Key: key, Value: value}
}

// Bool creates a field with a bool value.
func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

// Duration creates a field with a time.Duration value.
func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value}
}

// Time creates a field with a time.Time value.
func Time(key string, value time.Time) Field {
	return Field{Key: key, Value: value}
}

// Err creates a field from an error.
// If the error is nil, the value will be nil.
// Otherwise, the value will be the error's message string.
func Err(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: nil}
	}
	return Field{Key: "error", Value: err.Error()}
}

// ErrWithKey creates a field from an error with a custom key.
func ErrWithKey(key string, err error) Field {
	if err == nil {
		return Field{Key: key, Value: nil}
	}
	return Field{Key: key, Value: err.Error()}
}

// ErrWithStack creates a field from an error including its stack trace.
// Note: Stack trace capture has a small performance overhead.
func ErrWithStack(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: nil}
	}

	const maxDepth = 32
	var pcs [maxDepth]uintptr
	n := runtime.Callers(3, pcs[:])

	frames := runtime.CallersFrames(pcs[:n])
	var sb strings.Builder
	sb.WriteString(err.Error())
	sb.WriteString("\nStack:")

	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.File, "runtime/") &&
			!strings.Contains(frame.File, "github.com/cybergodev/dd/") {
			sb.WriteString(fmt.Sprintf("\n\t%s:%d: %s",
				frame.File, frame.Line, frame.Function))
		}
		if !more {
			break
		}
	}

	return Field{Key: "error", Value: sb.String()}
}

// Package-level structured logging functions using the default logger.

// DebugWith logs a structured debug message with the default logger.
func DebugWith(msg string, fields ...Field) { Default().LogWith(LevelDebug, msg, fields...) }

// InfoWith logs a structured info message with the default logger.
func InfoWith(msg string, fields ...Field) { Default().LogWith(LevelInfo, msg, fields...) }

// WarnWith logs a structured warning message with the default logger.
func WarnWith(msg string, fields ...Field) { Default().LogWith(LevelWarn, msg, fields...) }

// ErrorWith logs a structured error message with the default logger.
func ErrorWith(msg string, fields ...Field) { Default().LogWith(LevelError, msg, fields...) }

// FatalWith logs a structured fatal message with the default logger and exits.
// WARNING: defer statements will NOT execute. For graceful shutdown, use ErrorWith() with custom logic.
func FatalWith(msg string, fields ...Field) { Default().LogWith(LevelFatal, msg, fields...) }
