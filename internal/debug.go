package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sync"
)

// MaxDebugBufferSize is the maximum buffer size to return to pool (64KB)
const MaxDebugBufferSize = 64 * 1024

// debugBufPool pools bytes.Buffer objects for debug output
var debugBufPool = sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

// DebugBuffer is a helper type that manages getting and returning a buffer from the pool.
type DebugBuffer struct {
	*bytes.Buffer
}

// NewDebugBuffer creates a new DebugBuffer from the pool.
func NewDebugBuffer() *DebugBuffer {
	return &DebugBuffer{Buffer: debugBufPool.Get().(*bytes.Buffer)}
}

// Release returns the buffer to the pool if it's not too large.
func (b *DebugBuffer) Release() {
	if b.Buffer != nil {
		// SECURITY: zero contents before the buffer leaves our hands —
		// discarded and pooled alike. These buffers carry full debug payloads
		// built from user field values; every other pool in this package
		// (line, args, json builders) zeroes on release for exactly this
		// reason, and leaving plaintext sensitive data resident in pooled
		// memory here would contradict that policy.
		zeroBuffer(b.Buffer)
		// Discard buffers that grew too large to prevent unbounded memory growth
		if b.Cap() <= MaxDebugBufferSize {
			debugBufPool.Put(b.Buffer)
		}
		b.Buffer = nil
	}
}

// newDebugEncoder returns a JSON encoder writing into buf with the security
// defaults shared by all debug output paths.
//
// SECURITY: HTML escaping stays ON, matching the structured JSON formatter
// (internal/json.go): debug payloads contain user data that may end up
// rendered in HTML contexts, so & < > must not pass through raw. The encoder
// call sites in this file used to disagree (single-arg FormatJSONData true,
// multi-arg and writeTextItems false), applying the XSS rationale
// inconsistently within one function's output.
func newDebugEncoder(w io.Writer) *json.Encoder {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	return encoder
}

// writeString writes to w ignoring errors, for debug/diagnostic output
// where write failures are not actionable.
func writeString(w io.Writer, s string) {
	_, _ = io.WriteString(w, s)
}

// IsSimpleType checks if a value is a simple type that doesn't need JSON formatting.
func IsSimpleType(v any) bool {
	if v == nil {
		return true
	}

	if _, ok := v.(error); ok {
		return true
	}

	return !IsComplexValue(v)
}

// FormatSimpleValue formats a simple value as a string.
func FormatSimpleValue(v any) string {
	if v == nil {
		return "nil"
	}

	if err, ok := v.(error); ok {
		if err == nil {
			return "nil"
		}
		// SEC-003: user Error method — the debug Text/JSON helpers run outside
		// any recover, so this call must be panic-safe.
		return SafeErrorString(err)
	}

	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return "nil"
		}
		val = val.Elem()
	}

	return fmt.Sprintf("%v", val.Interface())
}

// FormatJSONData formats data as JSON using intelligent type conversion.
func FormatJSONData(data ...any) string {
	if len(data) == 0 {
		return "{}"
	}

	if len(data) == 1 {
		buf := NewDebugBuffer()
		defer buf.Release()

		converted := ConvertValue(data[0])

		encoder := newDebugEncoder(buf)
		if err := encoder.Encode(converted); err != nil {
			if jsonData, err := json.Marshal(data[0]); err == nil {
				return string(jsonData)
			}
			return "{}"
		}

		result := buf.String()
		if len(result) > 0 && result[len(result)-1] == '\n' {
			result = result[:len(result)-1]
		}
		return result
	}

	// Multiple arguments: treat as key-value pairs
	result := make(map[string]any, len(data)/2)
	for i := 0; i < len(data); i += 2 {
		key := fmt.Sprintf("%v", ConvertValue(data[i]))

		var value any
		if i+1 < len(data) {
			value = ConvertValue(data[i+1])
		}

		if key != "" {
			result[key] = value
		}
	}

	buf := NewDebugBuffer()
	defer buf.Release()

	encoder := newDebugEncoder(buf)
	if err := encoder.Encode(result); err != nil {
		if jsonData, err := json.Marshal(result); err == nil {
			return string(jsonData)
		}
		return "{}"
	}

	output := buf.String()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}
	return output
}

// writeTextItems writes each item separated by single spaces, terminated by a
// trailing newline. leadingSpace emits one space before the first item, for
// callers that prefix the list with e.g. a caller string. Simple types are
// written as-is; complex types are rendered as indented JSON; encoding
// failures fall back to "[i] value".
func writeTextItems(w io.Writer, data []any, leadingSpace bool) {
	buf := NewDebugBuffer()
	defer buf.Release()

	encoder := newDebugEncoder(buf)
	encoder.SetIndent("", "  ")

	for i, item := range data {
		if leadingSpace || i > 0 {
			writeString(w, " ")
		}

		if IsSimpleType(item) {
			writeString(w, FormatSimpleValue(item))
			continue
		}

		buf.Reset()
		convertedItem := ConvertValue(item)

		if err := encoder.Encode(convertedItem); err != nil {
			writeString(w, fmt.Sprintf("[%d] %v", i, item))
			continue
		}

		out := buf.Bytes()
		if len(out) > 0 && out[len(out)-1] == '\n' {
			out = out[:len(out)-1]
		}
		writeString(w, string(out))
	}

	writeString(w, "\n")
}

// OutputTextData writes formatted data to the specified writer.
// It outputs complex types as pretty-printed JSON and simple types as-is.
func OutputTextData(w io.Writer, data ...any) {
	if len(data) == 0 {
		writeString(w, "\n")
		return
	}
	writeTextItems(w, data, false)
}

// OutputJSON writes JSON-formatted data to the specified writer with caller info.
func OutputJSON(w io.Writer, caller string, data ...any) {
	if len(data) == 0 {
		writeString(w, caller+" {}\n")
		return
	}

	converted := FormatJSONData(data...)
	writeString(w, caller+" "+converted+"\n")
}

// OutputText writes text-formatted data to the specified writer with caller info.
func OutputText(w io.Writer, caller string, data ...any) {
	if len(data) == 0 {
		writeString(w, caller+"\n")
		return
	}

	writeString(w, caller)
	writeTextItems(w, data, true)
}
