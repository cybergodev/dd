package dd

import (
	"bytes"
	"io"
)

// TestBuffer creates a buffer for testing logger output
func TestBuffer() *bytes.Buffer {
	return &bytes.Buffer{}
}

// TestConfig creates a test configuration with the provided buffer
func TestConfig(buf *bytes.Buffer) *LoggerConfig {
	config := DefaultConfig()
	if buf != nil {
		config.Writers = []io.Writer{buf}
	}
	return config
}
