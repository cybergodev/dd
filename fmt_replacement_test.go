package dd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintf(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []any
		expected string
	}{
		{"simple string", "Hello %s", []any{"World"}, "Hello World"},
		{"multiple args", "%s %d %v", []any{"test", 42, true}, "test 42 true"},
		{"no args", "simple text", []any{}, "simple text"},
		{"empty format", "", []any{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			n := Printf(tt.format, tt.args...)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			if output != tt.expected {
				t.Errorf("Printf() output = %q, want %q", output, tt.expected)
			}
			if n != len(tt.expected) {
				t.Errorf("Printf() bytes written = %d, want %d", n, len(tt.expected))
			}
		})
	}
}

func TestPrint(t *testing.T) {
	tests := []struct {
		name     string
		args     []any
		expected string
	}{
		{"single string", []any{"Hello"}, "Hello"},
		{"multiple args", []any{"Hello", "World", 42}, "HelloWorld42"}, // No spaces when string is involved
		{"no args", []any{}, ""},
		{"mixed types", []any{123, "test", true}, "123testtrue"}, // No spaces when string is involved
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			n := Print(tt.args...)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			if output != tt.expected {
				t.Errorf("Print() output = %q, want %q", output, tt.expected)
			}
			if n != len(tt.expected) {
				t.Errorf("Print() bytes written = %d, want %d", n, len(tt.expected))
			}
		})
	}
}

func TestPrintln(t *testing.T) {
	tests := []struct {
		name     string
		args     []any
		expected string
	}{
		{"single string", []any{"Hello"}, "Hello\n"},
		{"multiple args", []any{"Hello", "World"}, "Hello World\n"},
		{"no args", []any{}, "\n"},
		{"mixed types", []any{123, true}, "123 true\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			n := Println(tt.args...)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			if output != tt.expected {
				t.Errorf("Println() output = %q, want %q", output, tt.expected)
			}
			if n != len(tt.expected) {
				t.Errorf("Println() bytes written = %d, want %d", n, len(tt.expected))
			}
		})
	}
}

func TestSprintf(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []any
		expected string
	}{
		{"simple format", "Hello %s", []any{"World"}, "Hello World"},
		{"multiple args", "%d + %d = %d", []any{1, 2, 3}, "1 + 2 = 3"},
		{"no args", "constant", []any{}, "constant"},
		{"complex format", "%s: %v (%.2f%%)", []any{"Progress", true, 85.67}, "Progress: true (85.67%)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Sprintf(tt.format, tt.args...)
			if result != tt.expected {
				t.Errorf("Sprintf() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSprint(t *testing.T) {
	tests := []struct {
		name     string
		args     []any
		expected string
	}{
		{"single string", []any{"Hello"}, "Hello"},
		{"multiple strings", []any{"Hello", "World"}, "HelloWorld"}, // No spaces between strings
		{"no args", []any{}, ""},
		{"mixed types", []any{42, "answer", true}, "42answertrue"}, // No spaces when string is involved
		{"numbers only", []any{42, 43}, "42 43"},                   // Space between non-strings
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Sprint(tt.args...)
			if result != tt.expected {
				t.Errorf("Sprint() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSprintln(t *testing.T) {
	tests := []struct {
		name     string
		args     []any
		expected string
	}{
		{"single string", []any{"Hello"}, "Hello\n"},
		{"multiple args", []any{"Hello", "World"}, "Hello World\n"},
		{"no args", []any{}, "\n"},
		{"mixed types", []any{42, true}, "42 true\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Sprintln(tt.args...)
			if result != tt.expected {
				t.Errorf("Sprintln() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFprintf(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []any
		expected string
	}{
		{"simple format", "Hello %s", []any{"World"}, "Hello World"},
		{"number format", "Value: %d", []any{42}, "Value: 42"},
		{"no args", "constant", []any{}, "constant"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			n := Fprintf(&buf, tt.format, tt.args...)

			if buf.String() != tt.expected {
				t.Errorf("Fprintf() output = %q, want %q", buf.String(), tt.expected)
			}
			if n != len(tt.expected) {
				t.Errorf("Fprintf() bytes written = %d, want %d", n, len(tt.expected))
			}
		})
	}
}

func TestFprint(t *testing.T) {
	tests := []struct {
		name     string
		args     []any
		expected string
	}{
		{"single arg", []any{"Hello"}, "Hello"},
		{"multiple args", []any{"Hello", 42, true}, "Hello42 true"}, // Space only between non-strings
		{"no args", []any{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			n := Fprint(&buf, tt.args...)

			if buf.String() != tt.expected {
				t.Errorf("Fprint() output = %q, want %q", buf.String(), tt.expected)
			}
			if n != len(tt.expected) {
				t.Errorf("Fprint() bytes written = %d, want %d", n, len(tt.expected))
			}
		})
	}
}

func TestFprintln(t *testing.T) {
	tests := []struct {
		name     string
		args     []any
		expected string
	}{
		{"single arg", []any{"Hello"}, "Hello\n"},
		{"multiple args", []any{"Hello", "World"}, "Hello World\n"},
		{"no args", []any{}, "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			n := Fprintln(&buf, tt.args...)

			if buf.String() != tt.expected {
				t.Errorf("Fprintln() output = %q, want %q", buf.String(), tt.expected)
			}
			if n != len(tt.expected) {
				t.Errorf("Fprintln() bytes written = %d, want %d", n, len(tt.expected))
			}
		})
	}
}

func TestScan(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		args     []any
		expected []any
		wantN    int
		wantErr  bool
	}{
		{
			name:     "single string",
			input:    "Hello",
			args:     []any{new(string)},
			expected: []any{"Hello"},
			wantN:    1,
			wantErr:  false,
		},
		{
			name:     "multiple values",
			input:    "42 true test",
			args:     []any{new(int), new(bool), new(string)},
			expected: []any{42, true, "test"},
			wantN:    3,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Redirect stdin
			old := os.Stdin
			r, w, _ := os.Pipe()
			os.Stdin = r

			go func() {
				defer w.Close()
				w.Write([]byte(tt.input))
			}()

			n := Scan(tt.args...)

			os.Stdin = old

			if n != tt.wantN {
				t.Errorf("Scan() n = %v, want %v", n, tt.wantN)
			}

			// Check scanned values
			for i, arg := range tt.args {
				var actual any
				switch v := arg.(type) {
				case *string:
					actual = *v
				case *int:
					actual = *v
				case *bool:
					actual = *v
				}
				if actual != tt.expected[i] {
					t.Errorf("Scan() arg[%d] = %v, want %v", i, actual, tt.expected[i])
				}
			}
		})
	}
}

func TestSscan(t *testing.T) {
	tests := []struct {
		name     string
		str      string
		args     []any
		expected []any
		wantN    int
		wantErr  bool
	}{
		{
			name:     "single string",
			str:      "Hello",
			args:     []any{new(string)},
			expected: []any{"Hello"},
			wantN:    1,
			wantErr:  false,
		},
		{
			name:     "multiple values",
			str:      "42 true test",
			args:     []any{new(int), new(bool), new(string)},
			expected: []any{42, true, "test"},
			wantN:    3,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := Sscan(tt.str, tt.args...)

			if n != tt.wantN {
				t.Errorf("Sscan() n = %v, want %v", n, tt.wantN)
			}

			// Check scanned values
			for i, arg := range tt.args {
				var actual any
				switch v := arg.(type) {
				case *string:
					actual = *v
				case *int:
					actual = *v
				case *bool:
					actual = *v
				}
				if actual != tt.expected[i] {
					t.Errorf("Sscan() arg[%d] = %v, want %v", i, actual, tt.expected[i])
				}
			}
		})
	}
}

func TestNewError(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		args     []any
		expected string
	}{
		{"simple error", "error: %s", []any{"something went wrong"}, "error: something went wrong"},
		{"wrapped error", "failed: %w", []any{errors.New("original")}, "failed: original"},
		{"no args", "constant error", []any{}, "constant error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewError(tt.format, tt.args...)
			if err.Error() != tt.expected {
				t.Errorf("NewError() = %q, want %q", err.Error(), tt.expected)
			}
		})
	}
}

func TestAppendFormat(t *testing.T) {
	tests := []struct {
		name     string
		dst      []byte
		format   string
		args     []any
		expected string
	}{
		{"empty dst", []byte{}, "Hello %s", []any{"World"}, "Hello World"},
		{"existing dst", []byte("Prefix: "), "Value %d", []any{42}, "Prefix: Value 42"},
		{"no args", []byte("Test"), "constant", []any{}, "Testconstant"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendFormat(tt.dst, tt.format, tt.args...)
			if string(result) != tt.expected {
				t.Errorf("AppendFormat() = %q, want %q", string(result), tt.expected)
			}
		})
	}
}

func TestAppend(t *testing.T) {
	tests := []struct {
		name     string
		dst      []byte
		args     []any
		expected string
	}{
		{"empty dst", []byte{}, []any{"Hello"}, "Hello"},
		{"existing dst", []byte("Prefix: "), []any{"Value", 42}, "Prefix: Value42"}, // No space when string is involved
		{"no args", []byte("Test"), []any{}, "Test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Append(tt.dst, tt.args...)
			if string(result) != tt.expected {
				t.Errorf("Append() = %q, want %q", string(result), tt.expected)
			}
		})
	}
}

func TestAppendln(t *testing.T) {
	tests := []struct {
		name     string
		dst      []byte
		args     []any
		expected string
	}{
		{"empty dst", []byte{}, []any{"Hello"}, "Hello\n"},
		{"existing dst", []byte("Prefix: "), []any{"Value"}, "Prefix: Value\n"},
		{"no args", []byte("Test"), []any{}, "Test\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Appendln(tt.dst, tt.args...)
			if string(result) != tt.expected {
				t.Errorf("Appendln() = %q, want %q", string(result), tt.expected)
			}
		})
	}
}

func TestPrintfWith(t *testing.T) {
	// Create a test logger to capture log output
	var logBuf bytes.Buffer
	logger, err := NewWithOptions(Options{
		Console:           false,
		AdditionalWriters: []io.Writer{&logBuf},
	})
	if err != nil {
		t.Fatalf("Failed to create test logger: %v", err)
	}
	defer logger.Close()

	// Set as default logger
	oldDefault := Default()
	SetDefault(logger)
	defer SetDefault(oldDefault)

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	n := PrintfWith("Test %s %d", "message", 42)

	w.Close()
	os.Stdout = old

	var stdoutBuf bytes.Buffer
	io.Copy(&stdoutBuf, r)

	if err != nil {
		t.Errorf("PrintfWith() error = %v", err)
	}

	expected := "Test message 42"
	if stdoutBuf.String() != expected {
		t.Errorf("PrintfWith() stdout = %q, want %q", stdoutBuf.String(), expected)
	}
	if n != len(expected) {
		t.Errorf("PrintfWith() bytes written = %d, want %d", n, len(expected))
	}

	// Check that it was also logged
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "Test message 42") {
		t.Errorf("PrintfWith() did not log message, log output: %q", logOutput)
	}
}

func TestNewErrorWith(t *testing.T) {
	// Create a test logger to capture log output
	var logBuf bytes.Buffer
	logger, err := NewWithOptions(Options{
		Console:           false,
		AdditionalWriters: []io.Writer{&logBuf},
	})
	if err != nil {
		t.Fatalf("Failed to create test logger: %v", err)
	}
	defer logger.Close()

	// Set as default logger
	oldDefault := Default()
	SetDefault(logger)
	defer SetDefault(oldDefault)

	err = NewErrorWith("test error: %s", "something failed")

	expectedMsg := "test error: something failed"
	if err.Error() != expectedMsg {
		t.Errorf("NewErrorWith() error = %q, want %q", err.Error(), expectedMsg)
	}

	// Check that it was also logged
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, expectedMsg) {
		t.Errorf("NewErrorWith() did not log error, log output: %q", logOutput)
	}
}

// Benchmark tests for performance comparison
func BenchmarkPrintf(b *testing.B) {
	// Redirect stdout to discard output
	old := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = old }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Printf("Test %s %d", "message", i)
	}
}

func BenchmarkSprintf(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Sprintf("Test %s %d", "message", i)
	}
}

func BenchmarkFprintf(b *testing.B) {
	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		Fprintf(&buf, "Test %s %d", "message", i)
	}
}

func BenchmarkAppendFormat(b *testing.B) {
	dst := make([]byte, 0, 64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = dst[:0]
		_ = AppendFormat(dst, "Test %s %d", "message", i)
	}
}
