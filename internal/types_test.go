package internal

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestLogFormatString(t *testing.T) {
	tests := []struct {
		format   LogFormat
		expected string
	}{
		{LogFormatText, "text"},
		{LogFormatJSON, "json"},
		{LogFormat(99), "unknown"},
		{LogFormat(-1), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.format.String()
			if result != tt.expected {
				t.Errorf("LogFormat.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
		{LogLevel(99), "UNKNOWN"},
		{LogLevel(-1), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.level.String()
			if result != tt.expected {
				t.Errorf("LogLevel.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLogLevelIsValid(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected bool
	}{
		{LevelDebug, true},
		{LevelInfo, true},
		{LevelWarn, true},
		{LevelError, true},
		{LevelFatal, true},
		{LogLevel(-1), false},
		{LogLevel(99), false},
	}

	for _, tt := range tests {
		name := tt.level.String()
		t.Run(name, func(t *testing.T) {
			result := tt.level.IsValid()
			if result != tt.expected {
				t.Errorf("LogLevel(%d).IsValid() = %v, want %v", tt.level, result, tt.expected)
			}
		})
	}
}

func TestJSONFieldNamesIsComplete(t *testing.T) {
	tests := []struct {
		name     string
		names    *JSONFieldNames
		expected bool
	}{
		{
			name:     "nil",
			names:    nil,
			expected: false,
		},
		{
			name:     "empty",
			names:    &JSONFieldNames{},
			expected: false,
		},
		{
			name: "partial",
			names: &JSONFieldNames{
				Timestamp: "ts",
				Level:     "lvl",
			},
			expected: false,
		},
		{
			name: "complete",
			names: &JSONFieldNames{
				Timestamp: "ts",
				Level:     "lvl",
				Caller:    "src",
				Message:   "msg",
				Fields:    "data",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.names.IsComplete()
			if result != tt.expected {
				t.Errorf("IsComplete() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsComplexValueExtended(t *testing.T) {
	var errIface error = errorStub("iface")
	var stringerIface fmt.Stringer = stringerStub("iface")

	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		// Simple types
		{"nil", nil, false},
		{"string", "hello", false},
		{"bool", true, false},
		{"int", 42, false},
		{"int8", int8(8), false},
		{"int16", int16(16), false},
		{"int32", int32(32), false},
		{"int64", int64(64), false},
		{"uint", uint(42), false},
		{"uint8", uint8(8), false},
		{"uint16", uint16(16), false},
		{"uint32", uint32(32), false},
		{"uint64", uint64(64), false},
		{"float32", float32(3.14), false},
		{"float64", 3.14, false},
		{"complex64", complex64(1 + 2i), false},
		{"complex128", complex(1, 2), false},
		{"time.Time", time.Now(), false},
		{"time.Duration", 5 * time.Second, false},

		// Pointer types
		{"*string", ptr("hello"), false},
		{"*int", ptr(int(42)), false},
		{"*time.Time", ptr(time.Now()), false},
		{"*time.Duration", ptr(5 * time.Second), false},

		// Interface types - error and stringer are not complex
		{"error", errorStub("test"), false},
		{"stringerStub", stringerStub("test"), false},
		{"*error (nil)", (*errorStub)(nil), false},
		{"*error (non-nil)", ptr(errorStub("test")), false}, // pointer to error implements error interface
		{"*error interface (nil)", (*error)(nil), false},
		{"*error interface (non-nil)", &errIface, false},
		{"*fmt.Stringer interface (non-nil)", &stringerIface, false},

		// Complex types
		{"slice", []int{1, 2, 3}, true},
		{"map", map[string]int{"a": 1}, true},
		{"struct", struct{ Name string }{"test"}, true},
		{"[]byte", []byte("hello"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsComplexValue(tt.value)
			if result != tt.expected {
				t.Errorf("IsComplexValue(%T) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

func ptr[T any](v T) *T {
	return &v
}

// Kinds whose underlying type is complex128: they fall past the main switch
// in ConvertValue and exercise the default branch's error/Stringer handling
// and the final fmt fallback.
type complexError complex128

func (c complexError) Error() string { return "complex error" }

type complexStringer complex128

func (c complexStringer) String() string { return "stringer!" }

// TestConvertValueDefaultBranch pins ConvertValue's fallback rendering for
// kinds outside the main switch, plus interface unwrapping.
func TestConvertValueDefaultBranch(t *testing.T) {
	var nilErr error
	var stringerIface fmt.Stringer = stringerStub("wrapped")

	tests := []struct {
		name  string
		input any
		want  any
	}{
		// json.Marshal cannot encode complex numbers, so the fmt fallback runs.
		{"plain complex uses fmt fallback", complex(1, 2), "<complex128:(1+2i)>"},
		{"Error() method wins over fmt fallback", complexError(1), "complex error"},
		{"String() method wins over fmt fallback", complexStringer(2), "stringer!"},
		// A non-nil interface stored in any unwraps to its dynamic value.
		{"interface unwraps to dynamic value", stringerIface, stringerStub("wrapped")},
		{"nil interface converts to nil", nilErr, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertValue(tt.input); got != tt.want {
				t.Errorf("ConvertValue(%T) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertValue(t *testing.T) {
	tests := []struct {
		name  string
		input any
		check func(any) bool
	}{
		{
			name:  "nil",
			input: nil,
			check: func(v any) bool { return v == nil },
		},
		{
			name:  "string",
			input: "hello",
			check: func(v any) bool { return v == "hello" },
		},
		{
			name:  "int",
			input: 42,
			check: func(v any) bool { return v == 42 },
		},
		{
			name:  "bool",
			input: true,
			check: func(v any) bool { return v == true },
		},
		{
			name:  "slice",
			input: []int{1, 2, 3},
			check: func(v any) bool {
				arr, ok := v.([]any)
				return ok && len(arr) == 3
			},
		},
		{
			name:  "map",
			input: map[string]int{"a": 1, "b": 2},
			check: func(v any) bool {
				m, ok := v.(map[string]any)
				return ok && len(m) == 2
			},
		},
		{
			name:  "time.Time",
			input: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			check: func(v any) bool {
				s, ok := v.(string)
				return ok && s == "2024-01-01T12:00:00Z"
			},
		},
		{
			name:  "time.Duration",
			input: 5 * time.Second,
			check: func(v any) bool {
				// time.Duration is a named type for int64, so it's returned as-is
				d, ok := v.(time.Duration)
				return ok && d == 5*time.Second
			},
		},
		{
			name:  "error",
			input: errorStub("test error"),
			check: func(v any) bool {
				// errorStub is a string type alias, returned as-is
				s, ok := v.(errorStub)
				return ok && s == "test error"
			},
		},
		{
			name:  "func",
			input: func() {},
			check: func(v any) bool {
				s, ok := v.(string)
				return ok && s != ""
			},
		},
		{
			name:  "chan",
			input: make(chan int),
			check: func(v any) bool {
				s, ok := v.(string)
				return ok && s != ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertValue(tt.input)
			if !tt.check(result) {
				t.Errorf("ConvertValue(%T) check failed, got: %v (%T)", tt.input, result, result)
			}
		})
	}
}

func TestConvertValueMaxDepth(t *testing.T) {
	// Create deeply nested structure
	type Node struct {
		Children []Node
	}

	// Create a very deep nesting by embedding slices
	var createDeepNode func(depth int) Node
	createDeepNode = func(depth int) Node {
		if depth <= 0 {
			return Node{}
		}
		return Node{Children: []Node{createDeepNode(depth - 1)}}
	}

	// Create structure exceeding max depth
	deepNode := createDeepNode(MaxConvertDepth + 10)

	result := ConvertValue(deepNode)

	// Should contain MAX_DEPTH_EXCEEDED somewhere in the result
	// (the check happens at each struct conversion level)
	resultStr, ok := result.(map[string]any)
	if !ok {
		t.Errorf("Expected map result, got: %T", result)
		return
	}

	// Traverse to find MAX_DEPTH_EXCEEDED
	found := false
	var traverse func(map[string]any)
	traverse = func(m map[string]any) {
		for _, v := range m {
			if s, ok := v.(string); ok && s == "[MAX_DEPTH_EXCEEDED]" {
				found = true
				return
			}
			if child, ok := v.(map[string]any); ok {
				traverse(child)
			}
			if children, ok := v.([]any); ok {
				for _, child := range children {
					if childMap, ok := child.(map[string]any); ok {
						traverse(childMap)
					}
				}
			}
			if found {
				return
			}
		}
	}
	traverse(resultStr)

	if !found {
		t.Error("Expected to find MAX_DEPTH_EXCEEDED in deeply nested structure")
	}
}

func TestConvertValueStructWithJSONTags(t *testing.T) {
	type TestStruct struct {
		Name     string `json:"name"`
		Value    int    `json:"value"`
		NoTag    string
		EmptyTag string `json:""`
	}

	input := TestStruct{
		Name:     "test",
		Value:    42,
		NoTag:    "notag",
		EmptyTag: "empty",
	}

	result := ConvertValue(input)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map[string]any, got %T", result)
	}

	// Check JSON tag names are used
	if _, ok := m["name"]; !ok {
		t.Error("Expected 'name' key from JSON tag")
	}
	if _, ok := m["value"]; !ok {
		t.Error("Expected 'value' key from JSON tag")
	}
	// NoTag should be present (uses field name when no tag)
	if _, ok := m["NoTag"]; !ok {
		t.Error("Expected 'NoTag' key (no JSON tag)")
	}
}

func TestConvertSliceWithDepth(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int
	}{
		{"empty slice", []int{}, 0},
		{"slice of ints", []int{1, 2, 3}, 3},
		{"slice of strings", []string{"a", "b"}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertValue(tt.input)
			arr, ok := result.([]any)
			if !ok {
				t.Fatalf("Expected []any, got %T", result)
			}
			if len(arr) != tt.expected {
				t.Errorf("Expected length %d, got %d", tt.expected, len(arr))
			}
		})
	}
}

func TestConvertMapWithDepth(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int
	}{
		{"nil map", (*map[string]int)(nil), 0}, // Will be converted to nil
		{"empty map", map[string]int{}, 0},
		{"map with values", map[string]int{"a": 1, "b": 2}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertValue(tt.input)
			if result == nil {
				if tt.name != "nil map" {
					t.Errorf("Expected non-nil result for %s", tt.name)
				}
				return
			}
			m, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("Expected map[string]any, got %T", result)
			}
			if len(m) != tt.expected {
				t.Errorf("Expected length %d, got %d", tt.expected, len(m))
			}
		})
	}
}

func TestConvertStructWithDepth(t *testing.T) {
	type Nested struct {
		Value int
	}

	type Outer struct {
		Name   string
		Nested Nested
	}

	input := Outer{
		Name:   "test",
		Nested: Nested{Value: 42},
	}

	result := ConvertValue(input)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map[string]any, got %T", result)
	}

	if m["Name"] != "test" {
		t.Errorf("Expected Name='test', got %v", m["Name"])
	}

	nested, ok := m["Nested"].(map[string]any)
	if !ok {
		t.Fatalf("Expected Nested to be map[string]any, got %T", m["Nested"])
	}
	if nested["Value"] != 42 {
		t.Errorf("Expected Nested.Value=42, got %v", nested["Value"])
	}
}

func TestDefaultJSONIndent(t *testing.T) {
	if DefaultJSONIndent != "  " {
		t.Errorf("DefaultJSONIndent = %q, want '  '", DefaultJSONIndent)
	}
}

func TestMaxConvertDepth(t *testing.T) {
	if MaxConvertDepth != 100 {
		t.Errorf("MaxConvertDepth = %d, want 100", MaxConvertDepth)
	}
}

// TestJSONFieldName pins the json-struct-tag name resolution shared by
// ConvertValue and the root package's recursive sensitive-data filter, so the
// two call sites cannot drift apart.
func TestJSONFieldName(t *testing.T) {
	type tagged struct {
		NoTag      string
		Custom     string `json:"custom_name"`
		WithOpts   string `json:"opts_name,omitempty"`
		OptsOnly   string `json:",omitempty"`
		DropMarker string `json:"-"`
	}

	tests := []struct {
		field string
		want  string
	}{
		{"NoTag", "NoTag"},
		{"Custom", "custom_name"},
		{"WithOpts", "opts_name"},
		{"OptsOnly", "OptsOnly"},
		{"DropMarker", "DropMarker"},
	}

	typ := reflect.TypeFor[tagged]()
	for _, tt := range tests {
		field, ok := typ.FieldByName(tt.field)
		if !ok {
			t.Fatalf("field %s not found", tt.field)
		}
		if got := JSONFieldName(field); got != tt.want {
			t.Errorf("JSONFieldName(%s) = %q, want %q", tt.field, got, tt.want)
		}
	}

	// `json:","` (comma with empty name) keeps the Go field name. Built
	// manually because declaring that tag on a struct field trips SA5008.
	commaTag := reflect.StructField{Name: "EmptyName", Tag: `json:","`}
	if got := JSONFieldName(commaTag); got != "EmptyName" {
		t.Errorf("JSONFieldName(json comma tag) = %q, want %q", got, "EmptyName")
	}
}

// TestMapKeyToString pins the map-key rendering used when converting
// arbitrary maps to map[string]any for debug/JSON output.
func TestMapKeyToString(t *testing.T) {
	tests := []struct {
		name string
		key  reflect.Value
		want string
	}{
		{"string", reflect.ValueOf("abc"), "abc"},
		{"int", reflect.ValueOf(int(42)), "42"},
		{"int64", reflect.ValueOf(int64(42)), "42"},
		{"float64", reflect.ValueOf(3.5), "3.5"},
		{"bool_true", reflect.ValueOf(true), "true"},
		{"bool_false", reflect.ValueOf(false), "false"},
		{"other", reflect.ValueOf(struct{ X int }{X: 1}), "{1}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapKeyToString(tt.key); got != tt.want {
				t.Errorf("MapKeyToString() = %q, want %q", got, tt.want)
			}
		})
	}
}
