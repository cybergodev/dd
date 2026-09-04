package internal

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDefaultJSONFieldNames(t *testing.T) {
	names := DefaultJSONFieldNames()

	if names.Timestamp != "timestamp" {
		t.Errorf("Timestamp = %q, want 'timestamp'", names.Timestamp)
	}
	if names.Level != "level" {
		t.Errorf("Level = %q, want 'level'", names.Level)
	}
	if names.Caller != "caller" {
		t.Errorf("Caller = %q, want 'caller'", names.Caller)
	}
	if names.Message != "message" {
		t.Errorf("Message = %q, want 'message'", names.Message)
	}
	if names.Fields != "fields" {
		t.Errorf("Fields = %q, want 'fields'", names.Fields)
	}
}

func TestJSONFieldNamesMergeWithDefaults(t *testing.T) {
	tests := []struct {
		name   string
		input  *JSONFieldNames
		verify func(*testing.T, *JSONFieldNames)
	}{
		{
			name:  "nil input",
			input: nil,
			verify: func(t *testing.T, result *JSONFieldNames) {
				defaults := DefaultJSONFieldNames()
				if result.Timestamp != defaults.Timestamp {
					t.Error("Should use default timestamp")
				}
			},
		},
		{
			name: "partial custom",
			input: &JSONFieldNames{
				Level:   "severity",
				Message: "msg",
				// Others empty - should use defaults
			},
			verify: func(t *testing.T, result *JSONFieldNames) {
				if result.Level != "severity" {
					t.Error("Should use custom level")
				}
				if result.Message != "msg" {
					t.Error("Should use custom message")
				}
				if result.Timestamp != "timestamp" {
					t.Error("Should use default timestamp")
				}
				if result.Caller != "caller" {
					t.Error("Should use default caller")
				}
				if result.Fields != "fields" {
					t.Error("Should use default fields")
				}
			},
		},
		{
			name: "all custom",
			input: &JSONFieldNames{
				Timestamp: "ts",
				Level:     "lvl",
				Caller:    "src",
				Message:   "msg",
				Fields:    "data",
			},
			verify: func(t *testing.T, result *JSONFieldNames) {
				if result.Timestamp != "ts" {
					t.Error("Should use custom timestamp")
				}
				if result.Level != "lvl" {
					t.Error("Should use custom level")
				}
				if result.Caller != "src" {
					t.Error("Should use custom caller")
				}
				if result.Message != "msg" {
					t.Error("Should use custom message")
				}
				if result.Fields != "data" {
					t.Error("Should use custom fields")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeWithDefaults(tt.input)
			tt.verify(t, result)
		})
	}
}

func TestFormatJSON(t *testing.T) {
	tests := []struct {
		name         string
		entry        map[string]any
		opts         *JSONOptions
		wantContains []string
	}{
		{
			name: "basic entry",
			entry: map[string]any{
				"message": "test",
				"level":   "INFO",
			},
			opts:         nil,
			wantContains: []string{`"message":"test"`, `"level":"INFO"`},
		},
		{
			name: "pretty print",
			entry: map[string]any{
				"message": "test",
			},
			opts: &JSONOptions{
				PrettyPrint: true,
				Indent:      "  ",
			},
			wantContains: []string{`"message"`, "test"},
		},
		{
			name: "complex entry",
			entry: map[string]any{
				"string": "value",
				"int":    42,
				"float":  3.14,
				"bool":   true,
				"null":   nil,
				"array":  []int{1, 2, 3},
				"object": map[string]string{"nested": "value"},
			},
			opts:         nil,
			wantContains: []string{`"string":"value"`, `"int":42`, `"float":3.14`, `"bool":true`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatJSON(tt.entry, tt.opts)

			// Verify it's valid JSON
			var jsonData map[string]any
			if err := json.Unmarshal([]byte(result), &jsonData); err != nil {
				t.Fatalf("Result is not valid JSON: %v, got: %s", err, result)
			}

			// Check required content
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("Result should contain %q, got: %s", want, result)
				}
			}

			// Test pretty print
			if tt.opts != nil && tt.opts.PrettyPrint {
				if !strings.Contains(result, "\n") {
					t.Error("Pretty print should contain newlines")
				}
			}
		})
	}
}

// TestWriteJSONValueFast exercises the allocation-free fast-path encoder for
// every supported type. Expected output is exact to pin the emitted JSON shape.
func TestWriteJSONValueFast(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"string", "hello", `"hello"`},
		{"int", int(42), "42"},
		{"int8", int8(8), "8"},
		{"int16", int16(16), "16"},
		{"int32", int32(32), "32"},
		{"int64", int64(64), "64"},
		{"uint", uint(42), "42"},
		{"uint8", uint8(8), "8"},
		{"uint16", uint16(16), "16"},
		{"uint32", uint32(32), "32"},
		{"uint64", uint64(64), "64"},
		{"float32", float32(3.14), "3.14"},
		{"float64", float64(3.14), "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, "null"},
		{"time.Time", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), `"2024-01-15T10:30:00Z"`},
		{"time.Duration", time.Hour, `"1h0m0s"`},
		{"nested map", map[string]any{"key": map[string]any{"nested": "value"}}, `{"key":{"nested":"value"}}`},
		{"string slice", []string{"a", "b"}, `["a","b"]`},
		{"int slice", []int{1, 2}, `[1,2]`},
		{"int64 slice", []int64{1, 2}, `[1,2]`},
		{"float64 slice", []float64{1.5, 2.5}, `[1.5,2.5]`},
		{"bool slice", []bool{true, false}, `[true,false]`},
		{"heterogeneous slice", []any{"a", 1, true}, `["a",1,true]`},
		{"empty slice", []any{}, `[]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if ok := writeJSONValueFast(&buf, tt.value); !ok {
				t.Fatalf("writeJSONValueFast(%T) returned false, want true", tt.value)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("writeJSONValueFast(%T) = %s, want %s", tt.value, got, tt.want)
			}
		})
	}
}

// TestWriteJSONValueFast_Unsupported verifies unsupported types report
// ok=false (so callers fall back to the standard encoder) instead of
// emitting malformed JSON.
func TestWriteJSONValueFast_Unsupported(t *testing.T) {
	for _, v := range []any{
		func() {},
		make(chan int),
		struct{ A int }{A: 1},
		[]any{func() {}}, // unsupported element poisons the whole slice
	} {
		var buf bytes.Buffer
		if ok := writeJSONValueFast(&buf, v); ok {
			t.Errorf("writeJSONValueFast(%T) returned true for unsupported type, output %q", v, buf.String())
		}
	}
}

// TestWriteJSONValueFast_DepthLimit verifies the recursion guard: nesting
// beyond maxJSONDepth reports false instead of overflowing the stack.
func TestWriteJSONValueFast_DepthLimit(t *testing.T) {
	// Build maxJSONDepth+1 levels of nested single-key maps.
	value := any("leaf")
	for range maxJSONDepth + 1 {
		value = map[string]any{"k": value}
	}

	var buf bytes.Buffer
	if ok := writeJSONValueFast(&buf, value); ok {
		t.Error("writeJSONValueFast should return false beyond maxJSONDepth")
	}
}

// TestWriteJSONStringEscapes pins the exact escaping behavior of the
// hand-rolled JSON string writer, including the HTML-safe escapes (<, >, &)
// and \uNNNN control-character escapes.
func TestWriteJSONStringEscapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"clean string needs no escape", "hello world", `"hello world"`},
		{"double quote", `say "hi"`, `"say \"hi\""`},
		{"backslash", `C:\path`, `"C:\\path"`},
		{"newline", "a\nb", `"a\nb"`},
		{"carriage return", "a\rb", `"a\rb"`},
		{"tab", "a\tb", `"a\tb"`},
		{"less than (XSS)", "<script>", `"\u003cscript\u003e"`},
		{"ampersand (entity injection)", "a&b", `"a\u0026b"`},
		{"control char", "a\x01b", `"a\u0001b"`},
		{"DEL passes through raw", "a\x7fb", "\"ab\""},
		{"multi-byte UTF-8 passes through", "日本語", `"日本語"`},
		{"empty string", "", `""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeJSONString(&buf, tt.input)
			if got := buf.String(); got != tt.want {
				t.Errorf("writeJSONString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatJSONSpecialCharacters(t *testing.T) {
	entry := map[string]any{
		"message": `message with "quotes" and \backslashes and
newlines`,
	}

	result := FormatJSON(entry, nil)

	// Verify it's valid JSON (should properly escape special characters)
	var jsonData map[string]any
	if err := json.Unmarshal([]byte(result), &jsonData); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	// Verify message is preserved correctly
	expected := `message with "quotes" and \backslashes and
newlines`
	if jsonData["message"] != expected {
		t.Error("Message with special characters not preserved correctly")
	}
}
