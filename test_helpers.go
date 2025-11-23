package dd

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// newTestLogger creates a logger for testing with a buffer
// Automatically registers cleanup
func newTestLogger(t *testing.T, config *LoggerConfig) (*Logger, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	if config == nil {
		config = DefaultConfig()
	}

	// Ensure we have a buffer writer
	hasBuffer := false
	for _, w := range config.Writers {
		if w == &buf {
			hasBuffer = true
			break
		}
	}

	if !hasBuffer {
		if config.Writers == nil {
			config.Writers = []io.Writer{&buf}
		} else {
			config.Writers = append(config.Writers, &buf)
		}
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create test logger: %v", err)
	}

	t.Cleanup(func() {
		logger.Close()
	})

	return logger, &buf
}

// newTestLoggerWithLevel creates a logger with specific level
func newTestLoggerWithLevel(t *testing.T, level LogLevel) (*Logger, *bytes.Buffer) {
	t.Helper()

	config := DefaultConfig()
	config.Level = level

	return newTestLogger(t, config)
}

// newTestJSONLogger creates a JSON logger for testing
func newTestJSONLogger(t *testing.T) (*Logger, *bytes.Buffer) {
	t.Helper()

	config := JSONConfig()
	return newTestLogger(t, config)
}

// assertContains checks if output contains expected string
func assertContains(t *testing.T, output, expected string) {
	t.Helper()

	if !strings.Contains(output, expected) {
		t.Errorf("Output does not contain %q\nGot: %s", expected, output)
	}
}

// assertNotContains checks if output does not contain unexpected string
func assertNotContains(t *testing.T, output, unexpected string) {
	t.Helper()

	if strings.Contains(output, unexpected) {
		t.Errorf("Output should not contain %q\nGot: %s", unexpected, output)
	}
}

// assertNoError checks that error is nil
func assertNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// assertError checks that error is not nil
func assertError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("Expected error but got nil")
	}
}

// assertBufferNotEmpty checks that buffer has content
func assertBufferNotEmpty(t *testing.T, buf *bytes.Buffer) {
	t.Helper()

	if buf.Len() == 0 {
		t.Error("Buffer should not be empty")
	}
}

// assertBufferEmpty checks that buffer is empty
func assertBufferEmpty(t *testing.T, buf *bytes.Buffer) {
	t.Helper()

	if buf.Len() > 0 {
		t.Errorf("Buffer should be empty, got: %s", buf.String())
	}
}
