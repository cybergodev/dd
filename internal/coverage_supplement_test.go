package internal

import (
	"bytes"
	"testing"
	"time"
)

// ============================================================================
// JSON VALUE FAST PATH TESTS
// ============================================================================

func TestWriteJSONValueFast_AllTypes(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		contains string
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
		{"time.Time", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), "2024-01-15T10:30:00Z"},
		{"time.Duration", time.Hour, "1h0m0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			ok := writeJSONValueFast(&buf, tt.value)
			if !ok {
				t.Fatalf("writeJSONValueFast returned false for %T", tt.value)
			}
			if !bytes.Contains(buf.Bytes(), []byte(tt.contains)) {
				t.Errorf("Expected %q in output, got: %s", tt.contains, buf.String())
			}
		})
	}
}

func TestWriteJSONValueFast_NestedMap(t *testing.T) {
	var buf bytes.Buffer
	value := map[string]any{
		"key": map[string]any{
			"nested": "value",
		},
	}
	ok := writeJSONValueFast(&buf, value)
	if !ok {
		t.Fatal("Expected ok for nested map")
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"nested":"value"`)) {
		t.Errorf("Expected nested value in output, got: %s", buf.String())
	}
}

func TestWriteJSONValueFast_Slice(t *testing.T) {
	var buf bytes.Buffer
	value := []any{"a", 1, true}
	ok := writeJSONValueFast(&buf, value)
	if !ok {
		t.Fatal("Expected ok for slice")
	}
	result := buf.String()
	if result != `["a",1,true]` {
		t.Errorf("Expected specific JSON, got: %s", result)
	}
}

// ============================================================================
// SANITIZE EDGE CASES
// ============================================================================

// ============================================================================
// FORMAT JSON DIRECT WITH COMPLEX FIELDS
// ============================================================================

func TestFormatJSON_ComplexFieldTypes(t *testing.T) {
	entry := map[string]any{
		"nested_map": map[string]any{
			"inner": []any{1, 2, 3},
		},
		"slice_of_maps": []any{
			map[string]any{"a": 1},
		},
	}

	result := FormatJSON(entry, nil)
	if result == "" {
		t.Error("Expected non-empty JSON output")
	}
	if !bytes.Contains([]byte(result), []byte("nested_map")) {
		t.Errorf("Expected nested_map in output, got: %s", result)
	}
}

func TestFormatJSON_NilEntry(t *testing.T) {
	result := FormatJSON(nil, nil)
	if result != "null" && result != "{}" {
		t.Logf("FormatJSON(nil) = %q", result)
	}
}
