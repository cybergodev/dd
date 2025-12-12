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
	New: func() any {
		sb := &strings.Builder{}
		sb.Grow(256) // 预分配合理的初始容量
		return sb
	},
}

func formatFields(fields []Field) string {
	fieldCount := len(fields)
	if fieldCount == 0 {
		return ""
	}

	sb := fieldPool.Get().(*strings.Builder)
	sb.Reset()
	defer fieldPool.Put(sb)

	// 预估容量以减少重新分配
	estimatedSize := fieldCount * 24 // 每个字段大约 24 字符
	if sb.Cap() < estimatedSize {
		sb.Grow(estimatedSize)
	}

	for i := range fieldCount {
		if i > 0 {
			sb.WriteByte(' ')
		}
		field := fields[i]
		sb.WriteString(field.Key)
		sb.WriteByte('=')

		// 优化类型判断，使用类型断言而非 switch
		switch v := field.Value.(type) {
		case string:
			if needsQuoting(v) {
				sb.WriteByte('"')
				// 简单转义，避免 fmt.Fprintf 的开销
				for _, r := range v {
					if r == '"' || r == '\\' {
						sb.WriteByte('\\')
					}
					sb.WriteRune(r)
				}
				sb.WriteByte('"')
			} else {
				sb.WriteString(v)
			}
		case nil:
			sb.WriteString("<nil>")
		case int:
			sb.WriteString(fmt.Sprintf("%d", v))
		case int64:
			sb.WriteString(fmt.Sprintf("%d", v))
		case float64:
			sb.WriteString(fmt.Sprintf("%g", v))
		case bool:
			if v {
				sb.WriteString("true")
			} else {
				sb.WriteString("false")
			}
		default:
			sb.WriteString(fmt.Sprintf("%v", v))
		}
	}

	return sb.String()
}

// 快速检查字符串是否需要引号
func needsQuoting(s string) bool {
	if len(s) == 0 {
		return true
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= ' ' || c == '"' || c == '\\' {
			return true
		}
	}
	return false
}

func (l *Logger) LogWith(level LogLevel, msg string, fields ...Field) {
	if !l.shouldLog(level) {
		return
	}

	if l.format == FormatJSON {
		l.logWithFieldsAndDepth(level, msg, fields, 7)
		return
	}

	if len(fields) > 0 {
		msg = msg + " " + formatFields(fields)
	}

	message := l.formatMessageWithDepth(level, msg, nil, 6)
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
