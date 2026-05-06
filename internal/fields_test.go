package internal

import (
	"strings"
	"testing"
	"time"
)

// ============================================================================
// FORMAT FIELDS TESTS
// ============================================================================

func TestFormatFields(t *testing.T) {
	t.Run("empty fields returns empty string", func(t *testing.T) {
		result := FormatFields(nil)
		if result != "" {
			t.Errorf("FormatFields(nil) = %q, want empty string", result)
		}

		result = FormatFields([]Field{})
		if result != "" {
			t.Errorf("FormatFields([]) = %q, want empty string", result)
		}
	})

	t.Run("single field", func(t *testing.T) {
		fields := []Field{{Key: "key", Value: "value"}}
		result := FormatFields(fields)
		expected := "key=value"
		if result != expected {
			t.Errorf("FormatFields() = %q, want %q", result, expected)
		}
	})

	t.Run("multiple fields", func(t *testing.T) {
		fields := []Field{
			{Key: "service", Value: "api"},
			{Key: "port", Value: 8080},
		}
		result := FormatFields(fields)
		if !strings.Contains(result, "service=api") {
			t.Errorf("Expected 'service=api' in result, got: %s", result)
		}
		if !strings.Contains(result, "port=8080") {
			t.Errorf("Expected 'port=8080' in result, got: %s", result)
		}
	})

	t.Run("empty key is skipped", func(t *testing.T) {
		fields := []Field{
			{Key: "", Value: "ignored"},
			{Key: "valid", Value: "value"},
		}
		result := FormatFields(fields)
		if strings.Contains(result, "ignored") {
			t.Errorf("Empty key field should be skipped, got: %s", result)
		}
		if !strings.Contains(result, "valid=value") {
			t.Errorf("Expected 'valid=value', got: %s", result)
		}
	})
}

// ============================================================================
// FORMAT FIELD VALUE TESTS - ALL TYPE BRANCHES
// ============================================================================

func TestFormatFieldValue(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		contains string
	}{
		// String types
		{"string simple", "hello", "hello"},
		{"string with spaces", "hello world", `"hello world"`},
		{"string with special chars", `test"quote`, `"test\"quote"`},

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
			result := FormatFields(fields)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("FormatFields() = %q, should contain %q", result, tt.contains)
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
		result := FormatFields(fields)
		if !strings.Contains(result, "neg=-42") {
			t.Errorf("Expected 'neg=-42', got: %s", result)
		}
	})

	t.Run("zero values", func(t *testing.T) {
		fields := []Field{
			{Key: "int_zero", Value: int(0)},
			{Key: "str_empty", Value: ""},
			{Key: "bool_false", Value: false},
		}
		result := FormatFields(fields)
		if !strings.Contains(result, "int_zero=0") {
			t.Errorf("Expected 'int_zero=0', got: %s", result)
		}
		if !strings.Contains(result, `str_empty=""`) {
			t.Errorf("Expected 'str_empty=\"\"', got: %s", result)
		}
		if !strings.Contains(result, "bool_false=false") {
			t.Errorf("Expected 'bool_false=false', got: %s", result)
		}
	})

	t.Run("struct default formatting", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		p := Person{Name: "John", Age: 30}
		fields := []Field{{Key: "person", Value: p}}
		result := FormatFields(fields)
		// Should use fmt.Fprint for structs without custom JSON marshaler
		if !strings.Contains(result, "person=") {
			t.Errorf("Expected 'person=' in output, got: %s", result)
		}
	})

	t.Run("very long string", func(t *testing.T) {
		longStr := strings.Repeat("a", 10000)
		fields := []Field{{Key: "long", Value: longStr}}
		result := FormatFields(fields)
		if !strings.Contains(result, longStr) {
			t.Errorf("Expected long string in output")
		}
	})

	t.Run("unicode string", func(t *testing.T) {
		fields := []Field{{Key: "unicode", Value: "日本語テスト"}}
		result := FormatFields(fields)
		if !strings.Contains(result, "unicode=日本語テスト") {
			t.Errorf("Expected unicode string in output, got: %s", result)
		}
	})
}

// ============================================================================
// BENCHMARK TESTS
// ============================================================================

func BenchmarkFormatFields(b *testing.B) {
	fields := []Field{
		{Key: "service", Value: "api"},
		{Key: "port", Value: 8080},
		{Key: "status", Value: "ok"},
		{Key: "latency_ms", Value: 42},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FormatFields(fields)
	}
}

func BenchmarkFormatFields_Single(b *testing.B) {
	fields := []Field{{Key: "key", Value: "value"}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FormatFields(fields)
	}
}

func BenchmarkNeedsQuoting(b *testing.B) {
	s := "hello world test value with spaces"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NeedsQuoting(s)
	}
}
