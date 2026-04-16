package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Field represents a structured log field.
// This is the internal representation used by the formatter.
type Field struct {
	Key   string
	Value any
}

// Constants for field formatting
const (
	// FieldBuilderCapacity is the initial capacity for field builder
	// Increased from 384 to 768 to reduce grow() calls
	FieldBuilderCapacity = 768
	// EstimatedFieldSize is the estimated size per field in bytes
	EstimatedFieldSize = 40
)

// fieldPool pools bytes.Buffer objects for field formatting
// to reduce memory allocations during high-frequency logging.
// SECURITY: Uses bytes.Buffer instead of strings.Builder to allow
// proper zeroing of sensitive data before returning to pool.
var fieldPool = sync.Pool{
	New: func() any {
		buf := &bytes.Buffer{}
		buf.Grow(FieldBuilderCapacity)
		return buf
	},
}

// FormatFields formats structured fields into a string representation.
// Uses a sync.Pool for bytes.Buffer to reduce allocations.
// SECURITY: Zeroes buffer contents before returning to pool to prevent
// sensitive data from remaining in pooled memory.
func FormatFields(fields []Field) string {
	fieldCount := len(fields)
	if fieldCount == 0 {
		return ""
	}

	buf := fieldPool.Get().(*bytes.Buffer)
	buf.Reset()

	// SECURITY: Zero buffer contents before returning to pool
	defer func() {
		// Don't return large buffers to pool - let GC collect them
		if buf.Cap() > 2048 {
			return
		}
		// Zero the buffer contents for security
		zeroBuffer(buf)
		fieldPool.Put(buf)
	}()

	estimatedSize := fieldCount * EstimatedFieldSize
	if buf.Cap() < estimatedSize {
		buf.Grow(estimatedSize - buf.Cap())
	}

	for i, field := range fields {
		if field.Key == "" {
			continue
		}

		if i > 0 {
			buf.WriteByte(' ')
		}

		buf.WriteString(field.Key)
		buf.WriteByte('=')

		formatFieldValueBytes(buf, field.Value)
	}

	return buf.String()
}

// formatFieldValueBytes formats a single field value to the buffer.
// This is separated to allow for better inlining and reduce code complexity.
// Uses bytes.Buffer instead of strings.Builder for proper security clearing.
func formatFieldValueBytes(buf *bytes.Buffer, v any) {
	switch val := v.(type) {
	case string:
		if NeedsQuoting(val) {
			buf.WriteByte('"')
			for j := 0; j < len(val); j++ {
				c := val[j]
				if c == '"' || c == '\\' {
					buf.WriteByte('\\')
				}
				buf.WriteByte(c)
			}
			buf.WriteByte('"')
		} else {
			buf.WriteString(val)
		}
	case int:
		buf.WriteString(strconv.FormatInt(int64(val), 10))
	case int64:
		buf.WriteString(strconv.FormatInt(val, 10))
	case int32:
		buf.WriteString(strconv.FormatInt(int64(val), 10))
	case int16:
		buf.WriteString(strconv.FormatInt(int64(val), 10))
	case int8:
		buf.WriteString(strconv.FormatInt(int64(val), 10))
	case uint:
		buf.WriteString(strconv.FormatUint(uint64(val), 10))
	case uint64:
		buf.WriteString(strconv.FormatUint(val, 10))
	case uint32:
		buf.WriteString(strconv.FormatUint(uint64(val), 10))
	case uint16:
		buf.WriteString(strconv.FormatUint(uint64(val), 10))
	case uint8:
		buf.WriteString(strconv.FormatUint(uint64(val), 10))
	case float64:
		buf.WriteString(strconv.FormatFloat(val, 'g', -1, 64))
	case float32:
		buf.WriteString(strconv.FormatFloat(float64(val), 'g', -1, 32))
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case time.Duration:
		buf.WriteString(val.String())
	case time.Time:
		buf.WriteString(val.Format(time.RFC3339))
	case nil:
		buf.WriteString("<nil>")
	default:
		if IsComplexValue(v) {
			if jsonData, err := json.Marshal(v); err == nil {
				buf.Write(jsonData)
			} else {
				fmt.Fprint(buf, v)
			}
		} else {
			fmt.Fprint(buf, v)
		}
	}
}

// NeedsQuoting checks if a string needs to be quoted in log output.
// Strings containing spaces, special characters, or control characters need quoting.
func NeedsQuoting(s string) bool {
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
