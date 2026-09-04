package internal

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewDebugBuffer(t *testing.T) {
	buf := NewDebugBuffer()
	if buf == nil {
		t.Fatal("NewDebugBuffer returned nil")
	}
	if buf.Buffer == nil {
		t.Error("DebugBuffer.Buffer is nil")
	}
}

func TestDebugBufferRelease(t *testing.T) {
	buf := NewDebugBuffer()
	buf.WriteString("test data")

	// Release should reset and return to pool
	buf.Release()

	if buf.Buffer != nil {
		t.Error("Buffer should be nil after release")
	}

	// Test with buffer larger than max size
	largeBuf := NewDebugBuffer()
	// Write data to exceed max size
	largeData := make([]byte, MaxDebugBufferSize+1)
	largeBuf.Write(largeData)

	largeBuf.Release()

	if largeBuf.Buffer != nil {
		t.Error("Large buffer should be nil after release")
	}
}

func TestDebugBufferReuse(t *testing.T) {
	// Get buffer, write, release
	buf1 := NewDebugBuffer()
	buf1.WriteString("first")
	buf1.Release()

	// Get another buffer - should be reset
	buf2 := NewDebugBuffer()
	if buf2.Len() != 0 {
		t.Errorf("Reused buffer should be empty, got len %d", buf2.Len())
	}
	buf2.Release()
}

func TestIsSimpleType(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{"nil", nil, true},
		{"string", "hello", true},
		{"int", 42, true},
		{"bool", true, true},
		{"error", errorStub("test"), true},
		{"slice", []int{1, 2, 3}, false},
		{"map", map[string]int{"a": 1}, false},
		{"struct", struct{ Name string }{"test"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSimpleType(tt.value)
			if result != tt.expected {
				t.Errorf("IsSimpleType(%T) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestFormatSimpleValue(t *testing.T) {
	val := 42
	var nilPtr *int
	var nilErr error

	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"nil", nil, "nil"},
		{"nil error", nilErr, "nil"},
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"error", errorStub("test error"), "test error"},
		{"*int dereferenced", &val, "42"},
		{"nil *int", nilPtr, "nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatSimpleValue(tt.value)
			if result != tt.expected {
				t.Errorf("FormatSimpleValue(%T) = %q, want %q", tt.value, result, tt.expected)
			}
		})
	}
}

func TestFormatJSONDataEmpty(t *testing.T) {
	result := FormatJSONData()
	if result != "{}" {
		t.Errorf("FormatJSONData() = %q, want '{}'", result)
	}
}

func TestFormatJSONDataSingleArg(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		contains string
	}{
		{"string", "hello", `"hello"`},
		{"int", 42, `42`},
		{"bool", true, `true`},
		{"slice", []int{1, 2, 3}, `[`},
		{"map", map[string]int{"a": 1}, `"a"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatJSONData(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("FormatJSONData(%T) = %q, should contain %q", tt.input, result, tt.contains)
			}
		})
	}
}

func TestFormatJSONDataMultipleArgs(t *testing.T) {
	// Test key-value pairs
	result := FormatJSONData("key1", "value1", "key2", 42)

	if !strings.Contains(result, `"key1"`) {
		t.Error("Should contain key1")
	}
	if !strings.Contains(result, `"value1"`) {
		t.Error("Should contain value1")
	}
	if !strings.Contains(result, `"key2"`) {
		t.Error("Should contain key2")
	}

	// Test odd number of args (missing value)
	result = FormatJSONData("key1", "value1", "key2")
	if !strings.Contains(result, `"key2"`) {
		t.Error("Should contain key2 even with missing value")
	}
}

func TestOutputTextData(t *testing.T) {
	tests := []struct {
		name  string
		data  []any
		check func(string) bool
	}{
		{
			name:  "empty",
			data:  []any{},
			check: func(s string) bool { return s == "\n" },
		},
		{
			name:  "simple string",
			data:  []any{"hello"},
			check: func(s string) bool { return strings.Contains(s, "hello") },
		},
		{
			name:  "simple int",
			data:  []any{42},
			check: func(s string) bool { return strings.Contains(s, "42") },
		},
		{
			name: "multiple simple",
			data: []any{"hello", 42, true},
			check: func(s string) bool {
				return strings.Contains(s, "hello") &&
					strings.Contains(s, "42") &&
					strings.Contains(s, "true")
			},
		},
		{
			name:  "complex type",
			data:  []any{map[string]int{"a": 1}},
			check: func(s string) bool { return strings.Contains(s, "{") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			OutputTextData(&buf, tt.data...)
			result := buf.String()
			if !tt.check(result) {
				t.Errorf("OutputTextData check failed, got: %q", result)
			}
		})
	}
}

func TestOutputJSON(t *testing.T) {
	tests := []struct {
		name   string
		caller string
		data   []any
		check  func(string) bool
	}{
		{
			name:   "empty data",
			caller: "test.go:10",
			data:   []any{},
			check: func(s string) bool {
				return strings.Contains(s, "test.go:10") &&
					strings.Contains(s, "{}")
			},
		},
		{
			name:   "single arg",
			caller: "test.go:20",
			data:   []any{"key", "value"},
			check: func(s string) bool {
				return strings.Contains(s, "test.go:20") &&
					strings.Contains(s, `"key"`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			OutputJSON(&buf, tt.caller, tt.data...)
			result := buf.String()
			if !tt.check(result) {
				t.Errorf("OutputJSON check failed, got: %q", result)
			}
		})
	}
}

func TestOutputText(t *testing.T) {
	tests := []struct {
		name   string
		caller string
		data   []any
		check  func(string) bool
	}{
		{
			name:   "empty data",
			caller: "test.go:10",
			data:   []any{},
			check: func(s string) bool {
				return strings.Contains(s, "test.go:10")
			},
		},
		{
			name:   "simple data",
			caller: "test.go:20",
			data:   []any{"hello", 42},
			check: func(s string) bool {
				return strings.Contains(s, "test.go:20") &&
					strings.Contains(s, "hello") &&
					strings.Contains(s, "42")
			},
		},
		{
			name:   "complex data",
			caller: "test.go:30",
			data:   []any{map[string]int{"a": 1}},
			check: func(s string) bool {
				return strings.Contains(s, "test.go:30") &&
					strings.Contains(s, `"a": 1`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			OutputText(&buf, tt.caller, tt.data...)
			result := buf.String()
			if !tt.check(result) {
				t.Errorf("OutputText check failed, got: %q", result)
			}
		})
	}
}

func TestMaxDebugBufferSize(t *testing.T) {
	if MaxDebugBufferSize != 64*1024 {
		t.Errorf("MaxDebugBufferSize = %d, want %d", MaxDebugBufferSize, 64*1024)
	}
}

func TestFormatJSONDataWithEncoder(t *testing.T) {
	// Test complex nested structure
	data := map[string]any{
		"nested": map[string]any{
			"array":  []int{1, 2, 3},
			"string": "value",
		},
	}

	result := FormatJSONData(data)
	if !strings.Contains(result, "nested") {
		t.Error("Should contain nested key")
	}
	if !strings.Contains(result, "array") {
		t.Error("Should contain array key")
	}
}

func TestOutputTextDataEncodingError(t *testing.T) {
	// Test with a type that might cause encoding issues
	var buf bytes.Buffer
	// Channel cannot be JSON encoded normally
	OutputTextData(&buf, make(chan int))
	// Should not panic and produce some output
	if buf.Len() == 0 {
		t.Error("Should produce output even for unencodable types")
	}
}

// TestOutputTextUnifiedItemFormatting locks the exact byte layout produced by
// the shared item writer behind OutputTextData/OutputText, so the unification
// cannot drift: items separated by single spaces, one leading space only when
// a caller prefix precedes the list, and a trailing newline.
func TestOutputTextUnifiedItemFormatting(t *testing.T) {
	var buf bytes.Buffer
	OutputTextData(&buf, "a", 42, true)
	if got, want := buf.String(), "a 42 true\n"; got != want {
		t.Errorf("OutputTextData output = %q, want %q", got, want)
	}

	buf.Reset()
	OutputText(&buf, "c:1", "a", 42)
	if got, want := buf.String(), "c:1 a 42\n"; got != want {
		t.Errorf("OutputText output = %q, want %q", got, want)
	}
}
