package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// jsonEncoderPool pools json.Encoder objects for JSON encoding.
// Each encoder is paired with a buffer and reused across calls.
var jsonEncoderPool = sync.Pool{
	New: func() any {
		buf := &bytes.Buffer{}
		buf.Grow(1024) // optimized for typical JSON entries
		enc := json.NewEncoder(buf)
		// SECURITY: Enable HTML escaping to prevent XSS attacks when logs are
		// rendered in HTML contexts (e.g., log viewers). This must match the
		// behavior in writeJSONString which also escapes <, >, & characters.
		enc.SetEscapeHTML(true)
		return &pooledEncoder{buf: buf, enc: enc}
	},
}

// (The former jsonBuilderPool is gone: the JSON fast path writes directly
// into the caller's line buffer — see formatJSONFastInto — so there is no
// intermediate builder buffer to pool or zero.)

// pooledEncoder holds a buffer and encoder pair for reuse.
type pooledEncoder struct {
	buf *bytes.Buffer
	enc *json.Encoder
}

// File system and retry configuration constants.
const (
	// FilePermissions is the permission mode for creating files (rw-------).
	// Only the owner has read and write permissions. This is more restrictive
	// than DirPermissions (0700) because files don't need execute permission.
	FilePermissions = 0600
	// RetryAttempts is the number of times to retry file operations before giving up.
	RetryAttempts = 3
	// RetryDelay is the duration to wait between retry attempts.
	RetryDelay = 10 * time.Millisecond
)

func DefaultJSONFieldNames() *JSONFieldNames {
	return &JSONFieldNames{
		Timestamp: "timestamp",
		Level:     "level",
		Caller:    "caller",
		Message:   "message",
		Fields:    "fields",
	}
}

func MergeWithDefaults(f *JSONFieldNames) *JSONFieldNames {
	if f == nil {
		return DefaultJSONFieldNames()
	}

	if f.IsComplete() {
		return f
	}

	result := &JSONFieldNames{
		Timestamp: f.Timestamp,
		Level:     f.Level,
		Caller:    f.Caller,
		Message:   f.Message,
		Fields:    f.Fields,
	}

	defaults := DefaultJSONFieldNames()
	if result.Timestamp == "" {
		result.Timestamp = defaults.Timestamp
	}
	if result.Level == "" {
		result.Level = defaults.Level
	}
	if result.Caller == "" {
		result.Caller = defaults.Caller
	}
	if result.Message == "" {
		result.Message = defaults.Message
	}
	if result.Fields == "" {
		result.Fields = defaults.Fields
	}

	return result
}

// FormatJSON formats a map as JSON using a fast path for simple types
// and falling back to encoding/json for complex types.
func FormatJSON(entry map[string]any, opts *JSONOptions) string {
	var buf bytes.Buffer
	FormatJSONInto(&buf, entry, opts)
	return buf.String()
}

// FormatJSONInto writes the JSON encoding of entry into dst, taking the same
// fast/standard paths as FormatJSON but writing bytes straight into dst so
// hot callers (the log formatter) avoid materializing an intermediate string
// per entry. On the standard-path encode error dst is reset and an error
// object is written instead.
func FormatJSONInto(dst *bytes.Buffer, entry map[string]any, opts *JSONOptions) {
	if opts == nil {
		opts = &JSONOptions{PrettyPrint: false, Indent: "  "}
	}

	// Use standard encoder for pretty print
	if opts.PrettyPrint {
		formatJSONStandardInto(dst, entry, opts)
		return
	}

	// Try fast path for simple entries
	if formatJSONFastInto(dst, entry) {
		return
	}

	// Fall back to standard encoder for complex entries
	formatJSONStandardInto(dst, entry, opts)
}

// formatJSONFastInto attempts to build compact JSON without reflection,
// writing directly into dst (which must be empty on entry).
// Returns true on success; on a complex type it resets dst and returns false
// so the caller can retry via the standard encoder.
// SECURITY: Uses no pooled buffers, so there is nothing to clear on exit —
// dst's lifecycle (including zeroing) belongs to the caller.
func formatJSONFastInto(dst *bytes.Buffer, entry map[string]any) bool {
	// SECURITY: Handle nil map gracefully
	if entry == nil {
		dst.WriteString("{}")
		return true
	}

	dst.WriteByte('{')
	first := true

	for k, v := range entry {
		if !first {
			dst.WriteByte(',')
		}
		first = false

		// Write key
		writeJSONString(dst, k)
		dst.WriteByte(':')

		// Write value - fast path for common types
		if !writeJSONValueFast(dst, v) {
			dst.Reset()  // partial line: discard before the caller's fallback
			return false // Need fallback for complex type
		}
	}

	dst.WriteByte('}')
	return true
}

// writeJSONValueFast writes a JSON value without reflection for common types.
// Returns true if successful, false if the type needs standard encoding.
// SECURITY: Includes depth limit to prevent stack overflow from deeply nested structures.
func writeJSONValueFast(buf *bytes.Buffer, v any) bool {
	return writeJSONValueFastWithDepth(buf, v, 0)
}

// maxJSONDepth limits the maximum nesting depth for JSON structures.
// SECURITY: Prevents stack overflow from deeply nested or malicious structures.
const maxJSONDepth = 100

// writeJSONValueFastWithDepth writes a JSON value with depth tracking.
// SECURITY: Returns false if depth exceeds maxJSONDepth to prevent stack overflow.
func writeJSONValueFastWithDepth(buf *bytes.Buffer, v any, depth int) bool {
	// SECURITY: Check depth limit to prevent stack overflow
	if depth > maxJSONDepth {
		return false // Fall back to standard encoder which handles this safely
	}

	switch val := v.(type) {
	case string:
		writeJSONString(buf, val)
		return true
	case int:
		appendInt(buf, int64(val))
		return true
	case int64:
		appendInt(buf, val)
		return true
	case int32:
		appendInt(buf, int64(val))
		return true
	case int16:
		appendInt(buf, int64(val))
		return true
	case int8:
		appendInt(buf, int64(val))
		return true
	case uint:
		appendUint(buf, uint64(val))
		return true
	case uint64:
		appendUint(buf, val)
		return true
	case uint32:
		appendUint(buf, uint64(val))
		return true
	case uint16:
		appendUint(buf, uint64(val))
		return true
	case uint8:
		appendUint(buf, uint64(val))
		return true
	case float64:
		appendFloat64(buf, val)
		return true
	case float32:
		appendFloat32(buf, val)
		return true
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
		return true
	case nil:
		buf.WriteString("null")
		return true
	case time.Time:
		writeJSONString(buf, val.Format(time.RFC3339))
		return true
	case time.Duration:
		writeJSONString(buf, val.String())
		return true
	case map[string]any:
		// Nested map - recurse with depth tracking
		buf.WriteByte('{')
		first := true
		for k2, v2 := range val {
			if !first {
				buf.WriteByte(',')
			}
			first = false
			writeJSONString(buf, k2)
			buf.WriteByte(':')
			if !writeJSONValueFastWithDepth(buf, v2, depth+1) {
				return false
			}
		}
		buf.WriteByte('}')
		return true
	case []string:
		// Fast path for string slices
		buf.WriteByte('[')
		for i, s := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, s)
		}
		buf.WriteByte(']')
		return true
	case []int:
		// Fast path for int slices
		buf.WriteByte('[')
		for i, n := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			appendInt(buf, int64(n))
		}
		buf.WriteByte(']')
		return true
	case []int64:
		// Fast path for int64 slices
		buf.WriteByte('[')
		for i, n := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			appendInt(buf, n)
		}
		buf.WriteByte(']')
		return true
	case []float64:
		// Fast path for float64 slices
		buf.WriteByte('[')
		for i, f := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			appendFloat64(buf, f)
		}
		buf.WriteByte(']')
		return true
	case []bool:
		// Fast path for bool slices
		buf.WriteByte('[')
		for i, b := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if b {
				buf.WriteString("true")
			} else {
				buf.WriteString("false")
			}
		}
		buf.WriteByte(']')
		return true
	case []any:
		// Fast path for generic slices
		buf.WriteByte('[')
		for i, elem := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if !writeJSONValueFastWithDepth(buf, elem, depth+1) {
				return false
			}
		}
		buf.WriteByte(']')
		return true
	default:
		// Complex type - need standard encoder
		return false
	}
}

// needsJSONEscape is the combined lookup table for writeJSONString's bulk scan.
// Characters requiring escape: 0x00-0x1F (control chars), '"', '\\', '<', '>', '&'.
// A single table (previously two, OR-ed per byte) halves the loads in the scan,
// which is the path every clean string takes.
var needsJSONEscape = func() (t [256]bool) {
	for i := range 0x20 {
		t[i] = true
	}
	t['"'] = true
	t['\\'] = true
	t['<'] = true
	t['>'] = true
	t['&'] = true
	return t
}()

// writeJSONString writes a JSON-escaped string.
// SECURITY: Also escapes HTML special characters (<, >, &) to prevent
// XSS attacks when logs are rendered in HTML contexts (e.g., log viewers).
func writeJSONString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')

	// Fast path: check if any character needs escaping.
	// Bulk scan first to avoid byte-by-byte switch overhead for clean strings.
	needsEscape := false
	for i := 0; i < len(s); i++ {
		if needsJSONEscape[s[i]] {
			needsEscape = true
			break
		}
	}
	if !needsEscape {
		buf.WriteString(s)
		buf.WriteByte('"')
		return
	}

	// Slow path: byte-by-byte escaping
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		case '<':
			// SECURITY: Escape < to prevent XSS in HTML contexts
			buf.WriteString(`\u003c`)
		case '>':
			// SECURITY: Escape > to prevent XSS in HTML contexts
			buf.WriteString(`\u003e`)
		case '&':
			// SECURITY: Escape & to prevent HTML entity injection
			buf.WriteString(`\u0026`)
		default:
			if c < 0x20 {
				buf.WriteString(`\u00`)
				buf.WriteByte(HexChars[c>>4])
				buf.WriteByte(HexChars[c&0xf])
			} else {
				buf.WriteByte(c)
			}
		}
	}
	buf.WriteByte('"')
}

// formatJSONStandardInto encodes entry with the standard library encoder into
// dst (which must be empty on entry), using a pooled encoder+buffer pair and
// copying the encoded bytes over — no intermediate string allocation.
func formatJSONStandardInto(dst *bytes.Buffer, entry map[string]any, opts *JSONOptions) {
	// Use pooled encoder (includes buffer) for better performance
	pe := jsonEncoderPool.Get().(*pooledEncoder)
	pe.buf.Reset()

	// SECURITY: Zero buffer contents before returning to pool
	defer func() {
		b := pe.buf.Bytes()
		for i := range b {
			b[i] = 0
		}
		pe.buf.Reset()
		jsonEncoderPool.Put(pe)
	}()

	// Reset encoder settings (escape HTML is already true from pool init)
	if opts.PrettyPrint {
		pe.enc.SetIndent("", opts.Indent)
	} else {
		pe.enc.SetIndent("", "") // Reset indent for non-pretty mode
	}

	if err := pe.enc.Encode(entry); err != nil {
		dst.Reset()
		fmt.Fprintf(dst, `{"error":"json marshal failed: %v"}`, err)
		return
	}

	// json.Encoder adds a trailing newline, remove it
	data := pe.buf.Bytes()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}

	dst.Write(data)
}
