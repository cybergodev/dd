package dd

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// ============================================================================
// STRUCTURED LOGGING TESTS
// ============================================================================

func TestAny(t *testing.T) {
	field := Any("key", "value")
	if field.Key != "key" {
		t.Errorf("Field key = %q, want %q", field.Key, "key")
	}
	if field.Value != "value" {
		t.Errorf("Field value = %q, want %q", field.Value, "value")
	}
}

func TestErr(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantKey string
		wantNil bool
	}{
		{
			name:    "nil error",
			err:     nil,
			wantKey: "error",
			wantNil: true,
		},
		{
			name:    "non-nil error",
			err:     &testError{msg: "test error"},
			wantKey: "error",
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := Err(tt.err)
			if field.Key != tt.wantKey {
				t.Errorf("Err() key = %q, want %q", field.Key, tt.wantKey)
			}
			if tt.wantNil && field.Value != nil {
				t.Errorf("Err(nil) value should be nil, got %v", field.Value)
			}
			if !tt.wantNil && field.Value == nil {
				t.Error("Err(error) value should not be nil")
			}
		})
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestFormatFields(t *testing.T) {
	tests := []struct {
		name     string
		fields   []Field
		contains []string
	}{
		{
			name:     "empty fields",
			fields:   []Field{},
			contains: []string{},
		},
		{
			name: "single field",
			fields: []Field{
				Any("key", "value"),
			},
			contains: []string{"key=value"},
		},
		{
			name: "multiple fields",
			fields: []Field{
				Any("name", "john"),
				Any("age", 30),
			},
			contains: []string{"name=john", "age=30"},
		},
		{
			name: "string with spaces",
			fields: []Field{
				Any("message", "hello world"),
			},
			contains: []string{"message=", "hello world"},
		},
		{
			name: "nil value",
			fields: []Field{
				Any("null_field", nil),
			},
			contains: []string{"null_field=<nil>"},
		},
		{
			name: "various types",
			fields: []Field{
				Any("string", "text"),
				Any("int", 42),
				Any("float", 3.14),
				Any("bool", true),
			},
			contains: []string{"string=text", "int=42", "float=3.14", "bool=true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatFields(tt.fields)

			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("formatFields() result should contain %q, got: %s", expected, result)
				}
			}
		})
	}
}

func TestFieldsToMap(t *testing.T) {
	// Helper function for testing field conversion
	fieldsToMap := func(fields []Field) map[string]any {
		if len(fields) == 0 {
			return nil
		}
		m := make(map[string]any, len(fields))
		for _, field := range fields {
			m[field.Key] = field.Value
		}
		return m
	}

	tests := []struct {
		name   string
		fields []Field
		want   map[string]any
	}{
		{
			name:   "empty fields",
			fields: []Field{},
			want:   nil,
		},
		{
			name: "single field",
			fields: []Field{
				Any("key", "value"),
			},
			want: map[string]any{"key": "value"},
		},
		{
			name: "multiple fields",
			fields: []Field{
				Any("name", "john"),
				Any("age", 30),
				Any("active", true),
			},
			want: map[string]any{
				"name":   "john",
				"age":    30,
				"active": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fieldsToMap(tt.fields)

			if tt.want == nil {
				if result != nil {
					t.Errorf("fieldsToMap() = %v, want nil", result)
				}
				return
			}

			if len(result) != len(tt.want) {
				t.Errorf("fieldsToMap() length = %d, want %d", len(result), len(tt.want))
			}

			for key, wantValue := range tt.want {
				gotValue, ok := result[key]
				if !ok {
					t.Errorf("fieldsToMap() missing key %q", key)
					continue
				}
				if gotValue != wantValue {
					t.Errorf("fieldsToMap()[%q] = %v, want %v", key, gotValue, wantValue)
				}
			}
		})
	}
}

func TestLogWith(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test basic structured logging
	logger.LogWith(LevelInfo, "user action", Any("user", "john"), Any("action", "login"))

	output := buf.String()
	if !strings.Contains(output, "user action") {
		t.Error("Output should contain message")
	}
	if !strings.Contains(output, "user=john") {
		t.Error("Output should contain user field")
	}
	if !strings.Contains(output, "action=login") {
		t.Error("Output should contain action field")
	}
}

func TestLogWithJSON(t *testing.T) {
	var buf bytes.Buffer
	config := JSONConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test JSON structured logging
	logger.LogWith(LevelInfo, "user action", Any("user", "john"), Any("action", "login"))

	output := buf.String()
	if !strings.Contains(output, `"message"`) {
		t.Error("JSON output should contain message field")
	}
	if !strings.Contains(output, `"user"`) {
		t.Error("JSON output should contain user field")
	}
	if !strings.Contains(output, `"action"`) {
		t.Error("JSON output should contain action field")
	}
}

func TestDebugWith(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Level = LevelDebug
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.DebugWith("debug message", Any("key", "value"))

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Error("Output should contain debug message")
	}
	if !strings.Contains(output, "key=value") {
		t.Error("Output should contain field")
	}
}

func TestInfoWith(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.InfoWith("info message", Any("key", "value"))

	output := buf.String()
	if !strings.Contains(output, "info message") {
		t.Error("Output should contain info message")
	}
}

func TestWarnWith(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.WarnWith("warn message", Any("key", "value"))

	output := buf.String()
	if !strings.Contains(output, "warn message") {
		t.Error("Output should contain warn message")
	}
}

func TestErrorWith(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	logger.ErrorWith("error message", Any("key", "value"))

	output := buf.String()
	if !strings.Contains(output, "error message") {
		t.Error("Output should contain error message")
	}
}

func TestFatalWith(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}
	config.FatalHandler = func() {
		// Override to prevent exit
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.FatalWith("fatal message", Any("key", "value"))

	output := buf.String()
	if !strings.Contains(output, "fatal message") {
		t.Error("Output should contain fatal message")
	}
}

func TestLogWithLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Level = LevelWarn
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// These should be filtered
	logger.DebugWith("debug", Any("key", "value"))
	logger.InfoWith("info", Any("key", "value"))

	// These should appear
	logger.WarnWith("warn", Any("key", "value"))
	logger.ErrorWith("error", Any("key", "value"))

	output := buf.String()
	if strings.Contains(output, "debug") || strings.Contains(output, "info") {
		t.Error("Lower level messages should be filtered")
	}
	if !strings.Contains(output, "warn") || !strings.Contains(output, "error") {
		t.Error("Higher level messages should appear")
	}
}

func TestLogWithInvalidLevel(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test with invalid levels
	logger.LogWith(LogLevel(-1), "invalid level", Any("key", "value"))
	logger.LogWith(LogLevel(99), "invalid level", Any("key", "value"))

	// Should not panic and should not produce output
	if buf.Len() > 0 {
		t.Error("Invalid log levels should not produce output")
	}
}

func TestLogWithEmptyFields(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test with no fields
	logger.LogWith(LevelInfo, "message without fields")

	output := buf.String()
	if !strings.Contains(output, "message without fields") {
		t.Error("Output should contain message")
	}
}

func TestLogWithManyFields(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	fields := make([]Field, 50)
	for i := 0; i < 50; i++ {
		fields[i] = Any("key"+string(rune(i)), i)
	}

	logger.LogWith(LevelInfo, "many fields", fields...)

	output := buf.String()
	if !strings.Contains(output, "many fields") {
		t.Error("Output should contain message")
	}
}

func TestPackageLevelStructuredFunctions(t *testing.T) {
	var buf bytes.Buffer

	// Create new logger for testing
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Save original and set test logger as default
	original := GetDefaultLogger()
	SetDefault(logger)
	defer func() {
		if original != nil {
			SetDefault(original)
		}
	}()

	// Test package-level structured functions
	DebugWith("debug", Any("key", "value"))
	InfoWith("info", Any("key", "value"))
	WarnWith("warn", Any("key", "value"))
	ErrorWith("error", Any("key", "value"))

	output := buf.String()
	// Debug might be filtered depending on default level
	if !strings.Contains(output, "info") {
		t.Error("Package level InfoWith() not working")
	}
	if !strings.Contains(output, "warn") {
		t.Error("Package level WarnWith() not working")
	}
	if !strings.Contains(output, "error") {
		t.Error("Package level ErrorWith() not working")
	}
}

func TestLogWithClosedLogger(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	logger.Close()

	// Should not panic
	logger.LogWith(LevelInfo, "after close", Any("key", "value"))
	logger.InfoWith("after close", Any("key", "value"))
}

func TestLogWithSpecialCharactersInFields(t *testing.T) {
	var buf bytes.Buffer
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test with special characters
	logger.InfoWith("message",
		Any("newline", "value\nwith\nnewlines"),
		Any("tab", "value\twith\ttabs"),
		Any("quote", `value"with"quotes`),
	)

	// Should not panic
	if buf.Len() == 0 {
		t.Error("Should produce output")
	}
}
