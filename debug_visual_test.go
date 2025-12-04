package dd

import (
	"bytes"
	"encoding/json"
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
				if !strings.Contains(output, `"hello world"`) {
					t.Errorf("expected quoted string, got: %s", output)
				}
			},
		},
		{
			name:  "integer",
			input: 42,
			validate: func(t *testing.T, output string) {
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
				var result map[string]any
				if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &result); err != nil {
					t.Errorf("failed to unmarshal output: %v", err)
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
				if !strings.Contains(output, `"hello world"`) {
					t.Errorf("expected quoted string, got: %s", output)
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

	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &result); err != nil {
		t.Errorf("failed to unmarshal logger.Json output: %v", err)
	}

	if result["user"] != "test_user" {
		t.Errorf("expected user=test_user, got: %v", result["user"])
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

	if !strings.Contains(output, "JSON marshal error") {
		t.Errorf("expected error message for invalid data, got: %s", output)
	}
}

func TestTextWithInvalidData(t *testing.T) {
	invalidData := make(chan int)

	output := captureStdout(func() {
		Text(invalidData)
	})

	if !strings.Contains(output, "JSON marshal error") {
		t.Errorf("expected error message for invalid data, got: %s", output)
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
