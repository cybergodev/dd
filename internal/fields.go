package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
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
	// EstimatedFieldSize is the estimated size per field in bytes
	EstimatedFieldSize = 40
)

// Numeric field writers: strconv.Append* into a stack-allocated scratch
// buffer avoids the intermediate string allocation of strconv.FormatInt /
// FormatUint / FormatFloat, which profiling showed as ~20% of allocation
// objects on the structured-logging path (one string per numeric field).
// The scratch sizes are worst-case for the type: int64 fits 19 digits plus a
// sign; float64's shortest 'g' representation fits well within 32 bytes.

// appendInt writes v in base 10 to buf without allocating.
func appendInt(buf *bytes.Buffer, v int64) {
	var scratch [20]byte
	buf.Write(strconv.AppendInt(scratch[:0], v, 10))
}

// appendUint writes v in base 10 to buf without allocating.
func appendUint(buf *bytes.Buffer, v uint64) {
	var scratch [20]byte
	buf.Write(strconv.AppendUint(scratch[:0], v, 10))
}

// appendFloat64 writes v's shortest 'g' representation to buf without
// allocating (matching strconv.FormatFloat(v, 'g', -1, 64)).
func appendFloat64(buf *bytes.Buffer, v float64) {
	var scratch [32]byte
	buf.Write(strconv.AppendFloat(scratch[:0], v, 'g', -1, 64))
}

// appendFloat32 writes v's shortest 'g' representation to buf without
// allocating (matching strconv.FormatFloat(v, 'g', -1, 32)).
func appendFloat32(buf *bytes.Buffer, v float32) {
	var scratch [32]byte
	buf.Write(strconv.AppendFloat(scratch[:0], float64(v), 'g', -1, 32))
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
				switch {
				case c == '"' || c == '\\':
					buf.WriteByte('\\')
					buf.WriteByte(c)
				case c == '\n':
					// SECURITY: escape line breaks so a quoted field value can
					// never forge additional log lines (log injection); the
					// quotes alone do not protect consumers that split output
					// on newlines. \r gets the same treatment.
					buf.WriteString("\\n")
				case c == '\r':
					buf.WriteString("\\r")
				case (c < 0x20 && c != '\t') || c == 0x7f:
					// SECURITY: remaining control bytes (incl. DEL) become
					// visible \xNN escapes — inert for terminals and parsers,
					// unlike raw bytes. \t stays raw (allowed, matches
					// SanitizeControlChars' policy for messages).
					buf.WriteString("\\x")
					buf.WriteByte(HexChars[c>>4])
					buf.WriteByte(HexChars[c&0x0f])
				default:
					buf.WriteByte(c)
				}
			}
			buf.WriteByte('"')
		} else {
			buf.WriteString(val)
		}
	case int:
		appendInt(buf, int64(val))
	case int64:
		appendInt(buf, val)
	case int32:
		appendInt(buf, int64(val))
	case int16:
		appendInt(buf, int64(val))
	case int8:
		appendInt(buf, int64(val))
	case uint:
		appendUint(buf, uint64(val))
	case uint64:
		appendUint(buf, val)
	case uint32:
		appendUint(buf, uint64(val))
	case uint16:
		appendUint(buf, uint64(val))
	case uint8:
		appendUint(buf, uint64(val))
	case float64:
		appendFloat64(buf, val)
	case float32:
		appendFloat32(buf, val)
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
		// SECURITY: 0x7f (DEL) is included so control bytes can never reach
		// the unquoted path — the escape logic lives in the quoted branch.
		if c <= ' ' || c == '"' || c == '\\' || c == 0x7f {
			return true
		}
	}
	return false
}
