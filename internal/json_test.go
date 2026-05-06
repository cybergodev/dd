package internal

import (
	"encoding/json"
	"strings"
	"testing"
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
				"bool":   true,
				"null":   nil,
				"array":  []int{1, 2, 3},
				"object": map[string]string{"nested": "value"},
			},
			opts:         nil,
			wantContains: []string{`"string":"value"`, `"int":42`, `"bool":true`},
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

func TestFormatJSONComplexFields(t *testing.T) {
	complexFields := map[string]any{
		"string": "value",
		"int":    42,
		"float":  3.14,
		"bool":   true,
		"null":   nil,
		"array":  []int{1, 2, 3},
		"object": map[string]string{"nested": "value"},
	}

	result := FormatJSON(complexFields, nil)

	// Verify it's valid JSON
	var jsonData map[string]any
	if err := json.Unmarshal([]byte(result), &jsonData); err != nil {
		t.Fatalf("Result is not valid JSON: %v", err)
	}

	// Verify fields are present
	if jsonData["string"] != "value" {
		t.Error("String field not preserved")
	}
	if jsonData["int"].(float64) != 42 {
		t.Error("Int field not preserved")
	}
	if jsonData["bool"] != true {
		t.Error("Bool field not preserved")
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
