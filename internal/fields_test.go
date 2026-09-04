package internal

import (
	"strings"
	"testing"
	"time"
)

// formatFieldsForTest renders fields through the production text formatter
// (formatText → formatFieldValueBytes), the path used by real log calls.
func formatFieldsForTest(fields []Field) string {
	formatter := NewMessageFormatter(&FormatterConfig{Format: LogFormatText})
	return formatter.FormatWithMessage(LevelInfo, 10, "msg", fields)
}

// ============================================================================
// FORMAT FIELD VALUE TESTS - ALL TYPE BRANCHES
// ============================================================================

func TestFormatFieldValue(t *testing.T) {
	t.Run("empty fields produce message only", func(t *testing.T) {
		if got := formatFieldsForTest(nil); got != "msg" {
			t.Errorf("nil fields: got %q, want %q", got, "msg")
		}
		if got := formatFieldsForTest([]Field{}); got != "msg" {
			t.Errorf("empty fields: got %q, want %q", got, "msg")
		}
	})

	t.Run("single field", func(t *testing.T) {
		got := formatFieldsForTest([]Field{{Key: "key", Value: "value"}})
		if expected := "msg key=value"; got != expected {
			t.Errorf("got %q, want %q", got, expected)
		}
	})

	t.Run("multiple fields", func(t *testing.T) {
		got := formatFieldsForTest([]Field{
			{Key: "service", Value: "api"},
			{Key: "port", Value: 8080},
		})
		if !strings.Contains(got, "service=api") {
			t.Errorf("Expected 'service=api' in result, got: %s", got)
		}
		if !strings.Contains(got, "port=8080") {
			t.Errorf("Expected 'port=8080' in result, got: %s", got)
		}
	})

	t.Run("empty key is skipped", func(t *testing.T) {
		got := formatFieldsForTest([]Field{
			{Key: "", Value: "ignored"},
			{Key: "valid", Value: "value"},
		})
		if strings.Contains(got, "ignored") {
			t.Errorf("Empty key field should be skipped, got: %s", got)
		}
		if !strings.Contains(got, "valid=value") {
			t.Errorf("Expected 'valid=value' in result, got: %s", got)
		}
	})

	tests := []struct {
		name     string
		value    any
		contains string
	}{
		// String types
		{"string simple", "hello", "hello"},
		{"string with spaces", "hello world", `"hello world"`},
		{"string with special chars", `test"quote`, `"test\"quote"`},
		{"string with backslash", `test\path`, `"test\\path"`},
		// SECURITY: control characters in field values must be escaped, never
		// emitted raw inside the quotes — a raw \n would forge an extra log
		// line for any consumer that splits on newlines (log injection).
		{"string with newline", "a\nb", `"a\nb"`},
		{"string with CR", "a\rb", `"a\rb"`},
		{"string with control byte", "a\x01b", `"a\x01b"`},
		{"string with DEL", "a\x7fb", `"a\x7fb"`},
		{"string with tab kept raw inside quotes", "a\tb", "\"a\tb\""},
		{"log forgery via field value", "ok\n[2099-01-01T00:00:00Z] FATAL forged", `"ok\n[2099-01-01T00:00:00Z] FATAL forged"`},

		// Integer types
		{"int", int(42), "42"},
		{"int64", int64(12345678901234), "12345678901234"},
		{"int32", int32(12345), "12345"},
		{"int16", int16(1000), "1000"},
		{"int8", int8(100), "100"},

		// Unsigned integer types
		{"uint", uint(42), "42"},
		{"uint64", uint64(12345678901234), "12345678901234"},
		{"uint32", uint32(12345), "12345"},
		{"uint16", uint16(1000), "1000"},
		{"uint8", uint8(100), "100"},

		// Float types
		{"float64", float64(3.14159), "3.14159"},
		{"float32", float32(2.5), "2.5"},

		// Boolean
		{"bool true", true, "true"},
		{"bool false", false, "false"},

		// Time types
		{"time.Duration", time.Hour, "1h0m0s"},
		{"time.Time", time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC), "2024-01-15T10:30:00Z"},

		// Nil
		{"nil", nil, "<nil>"},

		// Complex types (use JSON marshaling)
		{"slice", []string{"a", "b"}, `["a","b"]`},
		{"map", map[string]int{"x": 1}, `{"x":1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := []Field{{Key: "test", Value: tt.value}}
			result := formatFieldsForTest(fields)
			if !strings.Contains(result, "test="+tt.contains) {
				t.Errorf("got %q, should contain %q", result, "test="+tt.contains)
			}
		})
	}
}

// ============================================================================
// NEEDS QUOTING TESTS
// ============================================================================

func TestNeedsQuoting(t *testing.T) {
	tests := []struct {
		s     string
		needs bool
	}{
		// Empty string needs quoting
		{"", true},

		// Simple strings don't need quoting
		{"hello", false},
		{"test123", false},
		{"user_id", false},
		{"service-name", false},

		// Strings with spaces need quoting
		{"hello world", true},
		{"test value", true},

		// Strings with control characters need quoting
		{"test\tvalue", true},
		{"test\nvalue", true},

		// Strings with quotes need quoting
		{`test"value`, true},

		// Strings with backslash need quoting
		{`test\value`, true},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			result := NeedsQuoting(tt.s)
			if result != tt.needs {
				t.Errorf("NeedsQuoting(%q) = %v, want %v", tt.s, result, tt.needs)
			}
		})
	}
}

// ============================================================================
// FIELD VALUE FORMATTING EDGE CASES
// ============================================================================

func TestFormatFieldValue_EdgeCases(t *testing.T) {
	t.Run("negative integers", func(t *testing.T) {
		fields := []Field{{Key: "neg", Value: int(-42)}}
		result := formatFieldsForTest(fields)
		if !strings.Contains(result, "neg=-42") {
			t.Errorf("Expected 'neg=-42' in result, got: %s", result)
		}
	})

	t.Run("zero values", func(t *testing.T) {
		fields := []Field{
			{Key: "int_zero", Value: int(0)},
			{Key: "str_empty", Value: ""},
			{Key: "bool_false", Value: false},
		}
		result := formatFieldsForTest(fields)
		if !strings.Contains(result, "int_zero=0") {
			t.Errorf("Expected 'int_zero=0' in result, got: %s", result)
		}
		if !strings.Contains(result, `str_empty=""`) {
			t.Errorf("Expected 'str_empty=\"\"' in result, got: %s", result)
		}
		if !strings.Contains(result, "bool_false=false") {
			t.Errorf("Expected 'bool_false=false' in result, got: %s", result)
		}
	})

	t.Run("struct default formatting", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		p := Person{Name: "John", Age: 30}
		fields := []Field{{Key: "person", Value: p}}
		result := formatFieldsForTest(fields)
		// Should use fmt.Fprint for structs without custom JSON marshaler
		if !strings.Contains(result, "person=") {
			t.Errorf("Expected 'person=' in output, got: %s", result)
		}
	})

	t.Run("very long string", func(t *testing.T) {
		longStr := strings.Repeat("a", 10000)
		fields := []Field{{Key: "long", Value: longStr}}
		result := formatFieldsForTest(fields)
		if !strings.Contains(result, longStr) {
			t.Errorf("Expected long string in output")
		}
	})

	t.Run("unicode string", func(t *testing.T) {
		fields := []Field{{Key: "unicode", Value: "日本語テスト"}}
		result := formatFieldsForTest(fields)
		if !strings.Contains(result, "unicode=日本語テスト") {
			t.Errorf("Expected unicode string in output, got: %s", result)
		}
	})
}

// ============================================================================
// BENCHMARK TESTS
// ============================================================================

func BenchmarkFormatTextWithFields(b *testing.B) {
	formatter := NewMessageFormatter(&FormatterConfig{Format: LogFormatText})
	fields := []Field{
		{Key: "service", Value: "api"},
		{Key: "port", Value: 8080},
		{Key: "status", Value: "ok"},
		{Key: "latency_ms", Value: 42},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatter.FormatWithMessage(LevelInfo, 10, "msg", fields)
	}
}

func BenchmarkNeedsQuoting(b *testing.B) {
	s := "hello world test value with spaces"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NeedsQuoting(s)
	}
}
