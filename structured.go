package dd

import (
	"fmt"
	"strings"
	"sync"
)

type Field struct {
	Key   string
	Value any
}

func Any(key string, value any) Field {
	return Field{Key: key, Value: value}
}

func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

func Float64(key string, value float64) Field {
	return Field{Key: key, Value: value}
}

func Err(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: nil}
	}
	return Field{Key: "error", Value: err.Error()}
}

var fieldPool = sync.Pool{
	New: func() interface{} {
		return new(strings.Builder)
	},
}

func formatFields(fields []Field) string {
	if len(fields) == 0 {
		return ""
	}

	sb := fieldPool.Get().(*strings.Builder)
	sb.Reset()
	defer fieldPool.Put(sb)

	for i, field := range fields {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(field.Key)
		sb.WriteString("=")

		switch v := field.Value.(type) {
		case string:
			if strings.ContainsAny(v, " \t\n") {
				sb.WriteString(fmt.Sprintf("%q", v))
			} else {
				sb.WriteString(v)
			}
		case nil:
			sb.WriteString("<nil>")
		default:
			sb.WriteString(fmt.Sprintf("%v", v))
		}
	}

	return sb.String()
}

func fieldsToMap(fields []Field) map[string]any {
	if len(fields) == 0 {
		return nil
	}

	m := make(map[string]any, len(fields))
	for _, field := range fields {
		m[field.Key] = field.Value
	}
	return m
}

func (l *Logger) LogWith(level LogLevel, msg string, fields ...Field) {
	if !l.shouldLog(level) {
		return
	}

	if l.format == FormatJSON {
		l.logWithFieldsAndDepth(level, msg, fields, 7)
		return
	}

	fullMessage := msg
	if len(fields) > 0 {
		fullMessage = msg + " " + formatFields(fields)
	}

	message := l.formatMessageWithDepth(level, fullMessage, nil, 6)
	message = l.applySecurity(message)
	l.writeMessage(message)

	if level == LevelFatal {
		l.handleFatal()
	}
}

func (l *Logger) DebugWith(msg string, fields ...Field) { l.LogWith(LevelDebug, msg, fields...) }
func (l *Logger) InfoWith(msg string, fields ...Field)  { l.LogWith(LevelInfo, msg, fields...) }
func (l *Logger) WarnWith(msg string, fields ...Field)  { l.LogWith(LevelWarn, msg, fields...) }
func (l *Logger) ErrorWith(msg string, fields ...Field) { l.LogWith(LevelError, msg, fields...) }
func (l *Logger) FatalWith(msg string, fields ...Field) { l.LogWith(LevelFatal, msg, fields...) }

func DebugWith(msg string, fields ...Field) { Default().DebugWith(msg, fields...) }
func InfoWith(msg string, fields ...Field)  { Default().InfoWith(msg, fields...) }
func WarnWith(msg string, fields ...Field)  { Default().WarnWith(msg, fields...) }
func ErrorWith(msg string, fields ...Field) { Default().ErrorWith(msg, fields...) }
func FatalWith(msg string, fields ...Field) { Default().FatalWith(msg, fields...) }
