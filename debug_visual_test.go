package dd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestJson(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		validate func(t *testing.T, output string)
	}{
		{
			name:  "simple string",
			input: "hello world",
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "./debug_visual_test.go:") {
					t.Errorf("expected caller info, got: %s", output)
				}
				if !strings.Contains(output, `"hello world"`) {
					t.Errorf("expected quoted string, got: %s", output)
				}
			},
		},
		{
			name:  "integer",
			input: 42,
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "./debug_visual_test.go:") {
					t.Errorf("expected caller info, got: %s", output)
				}
				if !strings.Contains(output, "42") {
					t.Errorf("expected 42, got: %s", output)
				}
			},
		},
		{
			name: "struct",
			input: struct {
				Name string `json:"name"`
				Age  int    `json:"age"`
			}{Name: "John", Age: 30},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, `"name":"John"`) {
					t.Errorf("expected name field, got: %s", output)
				}
				if !strings.Contains(output, `"age":30`) {
					t.Errorf("expected age field, got: %s", output)
				}
			},
		},
		{
			name:  "map",
			input: map[string]any{"key1": "value1", "key2": 123},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "./debug_visual_test.go:") {
					t.Errorf("expected caller info, got: %s", output)
				}
				if !strings.Contains(output, `"key1"`) || !strings.Contains(output, `"value1"`) {
					t.Errorf("expected map content in output, got: %s", output)
				}
			},
		},
		{
			name:  "slice",
			input: []int{1, 2, 3, 4, 5},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "[1,2,3,4,5]") {
					t.Errorf("expected array, got: %s", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				Json(tt.input)
			})
			tt.validate(t, output)
		})
	}
}

func TestText(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		validate func(t *testing.T, output string)
	}{
		{
			name:  "simple string",
			input: "hello world",
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "./debug_visual_test.go:") {
					t.Errorf("expected caller info, got: %s", output)
				}
				if !strings.Contains(output, "hello world") {
					t.Errorf("expected raw string without quotes, got: %s", output)
				}
			},
		},
		{
			name:  "integer",
			input: 42,
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "./debug_visual_test.go:") {
					t.Errorf("expected caller info, got: %s", output)
				}
				if !strings.Contains(output, "42") {
					t.Errorf("expected raw integer, got: %s", output)
				}
			},
		},
		{
			name:  "float",
			input: 3.14,
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "./debug_visual_test.go:") {
					t.Errorf("expected caller info, got: %s", output)
				}
				if !strings.Contains(output, "3.14") {
					t.Errorf("expected raw float, got: %s", output)
				}
			},
		},
		{
			name:  "boolean",
			input: true,
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "./debug_visual_test.go:") {
					t.Errorf("expected caller info, got: %s", output)
				}
				if !strings.Contains(output, "true") {
					t.Errorf("expected raw boolean, got: %s", output)
				}
			},
		},
		{
			name: "struct with pretty print",
			input: struct {
				Name    string `json:"name"`
				Age     int    `json:"age"`
				Address string `json:"address"`
			}{Name: "John", Age: 30, Address: "123 Main St"},
			validate: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) < 3 {
					t.Errorf("expected multi-line output, got: %s", output)
				}
				if !strings.Contains(output, `"name": "John"`) {
					t.Errorf("expected formatted name field, got: %s", output)
				}
			},
		},
		{
			name:  "nested structure",
			input: map[string]any{"user": map[string]any{"name": "Alice", "age": 25}},
			validate: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) < 3 {
					t.Errorf("expected multi-line output for nested structure")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				Text(tt.input)
			})
			tt.validate(t, output)
		})
	}
}

func TestLoggerJson(t *testing.T) {
	logger := ToConsole()
	defer logger.Close()

	testData := map[string]any{
		"user":   "test_user",
		"action": "login",
		"status": "success",
	}

	output := captureStdout(func() {
		logger.Json(testData)
	})

	// Check for caller info
	if !strings.Contains(output, "./debug_visual_test.go:") {
		t.Errorf("expected caller info, got: %s", output)
	}

	// Check for JSON content
	if !strings.Contains(output, `"user"`) || !strings.Contains(output, `"test_user"`) {
		t.Errorf("expected JSON content with user field, got: %s", output)
	}
}

func TestLoggerText(t *testing.T) {
	logger := ToConsole()
	defer logger.Close()

	testData := struct {
		User   string `json:"user"`
		Action string `json:"action"`
		Status string `json:"status"`
	}{
		User:   "test_user",
		Action: "login",
		Status: "success",
	}

	output := captureStdout(func() {
		logger.Text(testData)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 {
		t.Errorf("expected multi-line pretty-printed output")
	}
}

func TestJsonWithInvalidData(t *testing.T) {
	invalidData := make(chan int)

	output := captureStdout(func() {
		Json(invalidData)
	})

	if !strings.Contains(output, "<chan:chan int>") {
		t.Errorf("expected channel representation for invalid data, got: %s", output)
	}
}

func TestTextWithInvalidData(t *testing.T) {
	invalidData := make(chan int)

	output := captureStdout(func() {
		Text(invalidData)
	})

	if !strings.Contains(output, "<chan:chan int>") {
		t.Errorf("expected channel representation for invalid data, got: %s", output)
	}
}

func TestJsonWithNil(t *testing.T) {
	output := captureStdout(func() {
		Json(nil)
	})

	if !strings.Contains(output, "null") {
		t.Errorf("expected null for nil input, got: %s", output)
	}
}

func TestTextWithComplexStructure(t *testing.T) {
	type Address struct {
		Street  string `json:"street"`
		City    string `json:"city"`
		ZipCode string `json:"zip_code"`
	}

	type User struct {
		Name    string   `json:"name"`
		Age     int      `json:"age"`
		Email   string   `json:"email"`
		Address Address  `json:"address"`
		Tags    []string `json:"tags"`
	}

	user := User{
		Name:  "John Doe",
		Age:   30,
		Email: "john@example.com",
		Address: Address{
			Street:  "123 Main St",
			City:    "New York",
			ZipCode: "10001",
		},
		Tags: []string{"developer", "golang", "backend"},
	}

	output := captureStdout(func() {
		Text(user)
	})

	if !strings.Contains(output, `"name": "John Doe"`) {
		t.Errorf("expected formatted name field")
	}
	if !strings.Contains(output, `"street": "123 Main St"`) {
		t.Errorf("expected formatted nested address field")
	}
}

func TestJsonMultipleArgs(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []any
		validate func(t *testing.T, output string)
	}{
		{
			name:   "two arguments",
			inputs: []any{"hello", 42},
			validate: func(t *testing.T, output string) {
				// Should be on same line with space separator
				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) != 1 {
					t.Errorf("expected single line output, got: %d lines", len(lines))
				}
				if !strings.Contains(output, `"hello"`) {
					t.Errorf("expected first argument, got: %s", output)
				}
				if !strings.Contains(output, "42") {
					t.Errorf("expected second argument, got: %s", output)
				}
			},
		},
		{
			name: "mixed types",
			inputs: []any{
				"string value",
				123,
				map[string]string{"key": "value"},
				[]int{1, 2, 3},
			},
			validate: func(t *testing.T, output string) {
				// Should be on same line with space separators
				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) != 1 {
					t.Errorf("expected single line output, got: %d lines", len(lines))
				}
				if !strings.Contains(output, `"string value"`) {
					t.Errorf("expected string value in output")
				}
				if !strings.Contains(output, "123") {
					t.Errorf("expected 123 in output")
				}
			},
		},
		{
			name: "with pointers",
			inputs: func() []any {
				str := "pointer value"
				num := 999
				return []any{&str, &num}
			}(),
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, `"pointer value"`) {
					t.Errorf("expected dereferenced pointer string, got: %s", output)
				}
				if !strings.Contains(output, "999") {
					t.Errorf("expected dereferenced pointer int, got: %s", output)
				}
			},
		},
		{
			name:   "empty call",
			inputs: []any{},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "./debug_visual_test.go:") {
					t.Errorf("expected caller info, got: %s", output)
				}
			},
		},
		{
			name:   "single argument",
			inputs: []any{"single"},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "./debug_visual_test.go:") {
					t.Errorf("expected caller info, got: %s", output)
				}
				if !strings.Contains(output, `"single"`) {
					t.Errorf("expected single value, got: %s", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				Json(tt.inputs...)
			})
			tt.validate(t, output)
		})
	}
}

func TestTextMultipleArgs(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []any
		validate func(t *testing.T, output string)
	}{
		{
			name: "two structs",
			inputs: []any{
				struct {
					Name string `json:"name"`
				}{Name: "Alice"},
				struct {
					Age int `json:"age"`
				}{Age: 25},
			},
			validate: func(t *testing.T, output string) {
				// Should be on same line with space separator
				if !strings.Contains(output, `"name": "Alice"`) {
					t.Errorf("expected formatted first struct, got: %s", output)
				}
				if !strings.Contains(output, `"age": 25`) {
					t.Errorf("expected formatted second struct, got: %s", output)
				}
			},
		},
		{
			name: "complex nested structures",
			inputs: []any{
				map[string]any{"user": map[string]string{"name": "Bob"}},
				[]map[string]int{{"count": 10}, {"count": 20}},
			},
			validate: func(t *testing.T, output string) {
				// Should contain both structures
				if !strings.Contains(output, "Bob") {
					t.Errorf("expected first structure in output")
				}
				if !strings.Contains(output, "count") {
					t.Errorf("expected second structure in output")
				}
			},
		},
		{
			name:   "empty call",
			inputs: []any{},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "./debug_visual_test.go:") {
					t.Errorf("expected caller info, got: %s", output)
				}
			},
		},
		{
			name: "single complex object",
			inputs: []any{
				map[string]any{"key1": "value1", "key2": 123},
			},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "./debug_visual_test.go:") {
					t.Errorf("expected caller info, got: %s", output)
				}
				if !strings.Contains(output, `"key1"`) {
					t.Errorf("expected JSON formatted map, got: %s", output)
				}
			},
		},
		{
			name:   "mixed simple and complex",
			inputs: []any{"text", 42, map[string]int{"num": 100}},
			validate: func(t *testing.T, output string) {
				// Should be on same line with space separators
				if !strings.Contains(output, "text") {
					t.Errorf("expected simple string, got: %s", output)
				}
				if !strings.Contains(output, "42") {
					t.Errorf("expected simple number, got: %s", output)
				}
				if !strings.Contains(output, `"num"`) {
					t.Errorf("expected JSON formatted map, got: %s", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				Text(tt.inputs...)
			})
			tt.validate(t, output)
		})
	}
}

func TestLoggerJsonMultipleArgs(t *testing.T) {
	logger := ToConsole()
	defer logger.Close()

	output := captureStdout(func() {
		logger.Json("test", 123, map[string]string{"key": "value"})
	})

	// Should be on same line with space separators
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Errorf("expected single line output, got: %d lines", len(lines))
	}
	if !strings.Contains(output, `"test"`) {
		t.Errorf("expected first argument in output")
	}
	if !strings.Contains(output, "123") {
		t.Errorf("expected second argument in output")
	}
}

func TestLoggerTextMultipleArgs(t *testing.T) {
	logger := ToConsole()
	defer logger.Close()

	output := captureStdout(func() {
		logger.Text(
			struct{ Name string }{Name: "Alice"},
			struct{ Age int }{Age: 30},
		)
	})

	// Should be on same line with space separator
	if !strings.Contains(output, `"Name": "Alice"`) {
		t.Errorf("expected first struct in output")
	}
	if !strings.Contains(output, `"Age": 30`) {
		t.Errorf("expected second struct in output")
	}
}

func TestJsonWithMixedValidAndInvalidData(t *testing.T) {
	validData := "valid string"
	invalidData := make(chan int)

	output := captureStdout(func() {
		Json(validData, invalidData, 42)
	})

	if !strings.Contains(output, `"valid string"`) {
		t.Errorf("expected valid data to be printed")
	}
	if !strings.Contains(output, "<chan:chan int>") {
		t.Errorf("expected channel representation for invalid data")
	}
	if !strings.Contains(output, "42") {
		t.Errorf("expected third argument to be printed")
	}
}

func TestTextSimpleTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"string", "hello", "hello"},
		{"int", 123, "123"},
		{"int64", int64(9999), "9999"},
		{"float32", float32(2.5), "2.5"},
		{"float64", 3.14159, "3.14159"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, "nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				Text(tt.input)
			})
			if !strings.Contains(output, "./debug_visual_test.go:") {
				t.Errorf("expected caller info, got: %s", output)
			}
			if !strings.Contains(output, tt.expected) {
				t.Errorf("expected %q in output, got %q", tt.expected, output)
			}
		})
	}
}

func TestTextPointers(t *testing.T) {
	str := "pointer string"
	num := 999
	flag := true

	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"string pointer", &str, "pointer string"},
		{"int pointer", &num, "999"},
		{"bool pointer", &flag, "true"},
		{"nil pointer", (*string)(nil), "nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				Text(tt.input)
			})
			if !strings.Contains(output, "./debug_visual_test.go:") {
				t.Errorf("expected caller info, got: %s", output)
			}
			if !strings.Contains(output, tt.expected) {
				t.Errorf("expected %q in output, got %q", tt.expected, output)
			}
		})
	}
}

func TestTextMixedSimpleAndComplex(t *testing.T) {
	output := captureStdout(func() {
		Text("hello", 42, map[string]int{"count": 10})
	})

	// Should contain caller info
	if !strings.Contains(output, "./debug_visual_test.go:") {
		t.Errorf("expected caller info, got: %s", output)
	}

	// Should contain all values with space separators
	// Note: complex types like maps are pretty-printed with indentation (multiple lines)
	// but they're still separated by spaces from other arguments
	if !strings.Contains(output, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", output)
	}

	if !strings.Contains(output, "42") {
		t.Errorf("expected '42' in output, got: %s", output)
	}

	// Should contain JSON formatted map
	if !strings.Contains(output, `"count"`) {
		t.Errorf("expected JSON formatted map in output")
	}
}

func TestTextf(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []any
		expected string
	}{
		{
			name:     "simple string format",
			format:   "Hello, %s!",
			args:     []any{"World"},
			expected: "Hello, World!",
		},
		{
			name:     "multiple arguments",
			format:   "User: %s, Age: %d, Active: %t",
			args:     []any{"Alice", 30, true},
			expected: "User: Alice, Age: 30, Active: true",
		},
		{
			name:     "numeric formatting",
			format:   "Price: $%.2f",
			args:     []any{19.99},
			expected: "Price: $19.99",
		},
		{
			name:     "no arguments",
			format:   "Static message",
			args:     []any{},
			expected: "Static message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				Textf(tt.format, tt.args...)
			})
			if !strings.Contains(output, "./debug_visual_test.go:") {
				t.Errorf("expected caller info, got: %s", output)
			}
			if !strings.Contains(output, tt.expected) {
				t.Errorf("expected %q in output, got %q", tt.expected, output)
			}
		})
	}
}

func TestJsonf(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []any
		validate func(t *testing.T, output string)
	}{
		{
			name:   "simple string format",
			format: "Status: %s",
			args:   []any{"success"},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, `"Status: success"`) {
					t.Errorf("expected formatted JSON string, got: %s", output)
				}
			},
		},
		{
			name:   "multiple arguments",
			format: "User %s logged in at %d",
			args:   []any{"admin", 1234567890},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, "User admin logged in at 1234567890") {
					t.Errorf("expected formatted string in JSON, got: %s", output)
				}
			},
		},
		{
			name:   "no arguments",
			format: "Static log message",
			args:   []any{},
			validate: func(t *testing.T, output string) {
				if !strings.Contains(output, `"Static log message"`) {
					t.Errorf("expected static message in JSON, got: %s", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(func() {
				Jsonf(tt.format, tt.args...)
			})
			tt.validate(t, output)
		})
	}
}

func TestLoggerTextf(t *testing.T) {
	logger := ToConsole()
	defer logger.Close()

	output := captureStdout(func() {
		logger.Textf("Name: %s, Count: %d", "TestItem", 42)
	})

	// Check for caller info
	if !strings.Contains(output, "./debug_visual_test.go:") {
		t.Errorf("expected caller info, got: %s", output)
	}

	// Check for formatted content
	if !strings.Contains(output, "Name: TestItem, Count: 42") {
		t.Errorf("expected formatted content, got: %s", output)
	}
}

func TestLoggerJsonf(t *testing.T) {
	logger := ToConsole()
	defer logger.Close()

	output := captureStdout(func() {
		logger.Jsonf("Result: %s, Code: %d", "OK", 200)
	})

	if !strings.Contains(output, "Result: OK, Code: 200") {
		t.Errorf("expected formatted string in JSON output, got: %s", output)
	}
}

func TestTextWithMixedNilAndError(t *testing.T) {
	err := errors.New("sample error")
	var nilErr error

	output := captureStdout(func() {
		Text("text", nil, err, nilErr, 42)
	})

	if !strings.Contains(output, "text") {
		t.Errorf("expected string in output")
	}
	if !strings.Contains(output, "nil") {
		t.Errorf("expected nil for nil values")
	}
	if !strings.Contains(output, "sample error") {
		t.Errorf("expected error message in output")
	}
	if !strings.Contains(output, "42") {
		t.Errorf("expected number in output")
	}
}
