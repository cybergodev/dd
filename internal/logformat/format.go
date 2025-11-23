package logformat

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
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

func FormatMessage(
	level LogLevel,
	includeTime bool,
	timeFormat string,
	includeLevel bool,
	includeCaller bool,
	callerDepth int,
	fullPath bool,
	args ...any,
) string {
	var parts []string

	if includeTime {
		timestamp := time.Now().Format(timeFormat)
		parts = append(parts, fmt.Sprintf("[%s]", timestamp))
	}

	if includeLevel {
		parts = append(parts, fmt.Sprintf("[%s]", LogLevel(level).String()))
	}

	if includeCaller {
		if caller := getCaller(callerDepth, fullPath); caller != "" {
			parts = append(parts, caller)
		}
	}

	var message string
	if len(args) == 0 {
		message = ""
	} else if len(args) == 1 {
		if s, ok := args[0].(string); ok {
			message = s
		} else {
			message = fmt.Sprint(args[0])
		}
	} else {
		message = strings.TrimSuffix(fmt.Sprintln(args...), "\n")
	}
	parts = append(parts, message)

	return strings.Join(parts, " ")
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
