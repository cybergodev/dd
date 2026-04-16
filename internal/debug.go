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
		// Discard buffers that grew too large to prevent unbounded memory growth
		if b.Cap() <= MaxDebugBufferSize {
			b.Reset()
			debugBufPool.Put(b.Buffer)
		}
		b.Buffer = nil
	}
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
		return err.Error()
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

		encoder := json.NewEncoder(buf)
		// SECURITY: Enable HTML escaping to prevent XSS attacks when debug
		// output is rendered in HTML contexts. Matches writeJSONString behavior.
		encoder.SetEscapeHTML(true)
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
		var key string
		var value any

		if i < len(data) {
			key = fmt.Sprintf("%v", ConvertValue(data[i]))
		}

		if i+1 < len(data) {
			value = ConvertValue(data[i+1])
		}

		if key != "" {
			result[key] = value
		}
	}

	buf := NewDebugBuffer()
	defer buf.Release()

	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
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

// OutputTextData writes formatted data to the specified writer.
// It outputs complex types as pretty-printed JSON and simple types as-is.
func OutputTextData(w io.Writer, data ...any) {
	if len(data) == 0 {
		writeString(w, "\n")
		return
	}

	buf := NewDebugBuffer()
	defer buf.Release()

	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	for i, item := range data {
		if IsSimpleType(item) {
			output := FormatSimpleValue(item)
			if i < len(data)-1 {
				writeString(w, output+" ")
			} else {
				writeString(w, output+"\n")
			}
			continue
		}

		buf.Reset()
		convertedItem := ConvertValue(item)

		if err := encoder.Encode(convertedItem); err != nil {
			writeString(w, fmt.Sprintf("[%d] %v", i, item))
			if i < len(data)-1 {
				writeString(w, " ")
			} else {
				writeString(w, "\n")
			}
			continue
		}

		out := buf.Bytes()
		if len(out) > 0 && out[len(out)-1] == '\n' {
			out = out[:len(out)-1]
		}

		if i < len(data)-1 {
			writeString(w, string(out)+" ")
		} else {
			writeString(w, string(out)+"\n")
		}
	}
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

	buf := NewDebugBuffer()
	defer buf.Release()

	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	for i, item := range data {
		if IsSimpleType(item) {
			output := FormatSimpleValue(item)
			if i < len(data)-1 {
				writeString(w, " "+output)
			} else {
				writeString(w, " "+output+"\n")
			}
			continue
		}

		buf.Reset()
		convertedItem := ConvertValue(item)

		if err := encoder.Encode(convertedItem); err != nil {
			writeString(w, fmt.Sprintf(" [%d] %v", i, item))
			continue
		}

		output := buf.Bytes()
		if len(output) > 0 && output[len(output)-1] == '\n' {
			output = output[:len(output)-1]
		}

		if i < len(data)-1 {
			writeString(w, " "+string(output))
		} else {
			writeString(w, " "+string(output)+"\n")
		}
	}
}
