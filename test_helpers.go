package dd

import (
	"bytes"
	"io"
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
		_ = logger.Close()
	})

	return logger, &buf
}
