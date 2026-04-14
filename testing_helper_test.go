package dd

import (
	"bytes"
	"strings"
	"testing"
)

// ============================================================================
// TEST INFRASTRUCTURE - REDUCES BOILERPLATE IN TEST FILES
// ============================================================================

// TestLoggerBuilder provides a fluent API for creating test loggers.
// This reduces repetitive DefaultConfig() + modification patterns across tests.
type TestLoggerBuilder struct {
	cfg Config
}

// NewTestLoggerBuilder creates a new test logger builder with default config.
func NewTestLoggerBuilder() *TestLoggerBuilder {
	return &TestLoggerBuilder{
		cfg: DefaultConfig(),
	}
}

// WithOutput sets the output writer for the test logger.
func (b *TestLoggerBuilder) WithOutput(buf *bytes.Buffer) *TestLoggerBuilder {
	b.cfg.Output = buf
	return b
}

// WithLevel sets the log level for the test logger.
func (b *TestLoggerBuilder) WithLevel(level LogLevel) *TestLoggerBuilder {
	b.cfg.Level = level
	return b
}

// WithFormat sets the output format for the test logger.
func (b *TestLoggerBuilder) WithFormat(format LogFormat) *TestLoggerBuilder {
	b.cfg.Format = format
	return b
}

// WithJSON enables JSON format with default options.
func (b *TestLoggerBuilder) WithJSON() *TestLoggerBuilder {
	b.cfg.Format = FormatJSON
	if b.cfg.JSON == nil {
		b.cfg.JSON = DefaultJSONOptions()
	}
	return b
}

// WithDynamicCaller enables dynamic caller detection.
func (b *TestLoggerBuilder) WithDynamicCaller(fullPath bool) *TestLoggerBuilder {
	b.cfg.DynamicCaller = true
	b.cfg.FullPath = fullPath
	return b
}

// WithSecurity enables security configuration with sensitive filter.
func (b *TestLoggerBuilder) WithSecurity() *TestLoggerBuilder {
	b.cfg.Security = &SecurityConfig{
		SensitiveFilter: NewSensitiveDataFilter(),
	}
	return b
}

// WithFatalHandler sets a fatal handler for testing fatal-level logs.
func (b *TestLoggerBuilder) WithFatalHandler(handler func()) *TestLoggerBuilder {
	b.cfg.FatalHandler = handler
	return b
}

// WithFile sets file output configuration.
func (b *TestLoggerBuilder) WithFile(path string, maxSizeMB int, maxBackups int) *TestLoggerBuilder {
	b.cfg.File = &FileConfig{
		Path:       path,
		MaxSizeMB:  maxSizeMB,
		MaxBackups: maxBackups,
	}
	return b
}

// WithSampling enables log sampling.
func (b *TestLoggerBuilder) WithSampling(initial, thereafter int) *TestLoggerBuilder {
	b.cfg.Sampling = &SamplingConfig{
		Enabled:    true,
		Initial:    initial,
		Thereafter: thereafter,
	}
	return b
}

// Build creates the logger from the configured builder.
func (b *TestLoggerBuilder) Build(t *testing.T) *Logger {
	t.Helper()
	logger, err := New(b.cfg)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	return logger
}

// BuildNoError creates the logger without requiring *testing.T (caller handles error).
func (b *TestLoggerBuilder) BuildNoError() (*Logger, error) {
	return New(b.cfg)
}

// Config returns a pointer to the underlying config for advanced modifications.
func (b *TestLoggerBuilder) Config() *Config {
	return &b.cfg
}


// ============================================================================
// ASSERTION HELPERS
// ============================================================================

// AssertContains fails the test if output does not contain the expected substring.
func AssertContains(t *testing.T, output, expected, msg string) {
	t.Helper()
	if !strings.Contains(output, expected) {
		t.Errorf("%s: expected %q in output, got: %s", msg, expected, output)
	}
}

// AssertNotContains fails the test if output contains the unexpected substring.
func AssertNotContains(t *testing.T, output, unexpected, msg string) {
	t.Helper()
	if strings.Contains(output, unexpected) {
		t.Errorf("%s: unexpected %q in output, got: %s", msg, unexpected, output)
	}
}

// AssertEqual fails the test if actual != expected.
func AssertEqual(t *testing.T, actual, expected any, msg string) {
	t.Helper()
	if actual != expected {
		t.Errorf("%s: expected %v, got %v", msg, expected, actual)
	}
}

// AssertNotEmpty fails the test if value is empty.
func AssertNotEmpty(t *testing.T, value, msg string) {
	t.Helper()
	if value == "" {
		t.Errorf("%s: expected non-empty value", msg)
	}
}

// AssertLen fails the test if len(value) != expected.
func AssertLen(t *testing.T, value, expected int, msg string) {
	t.Helper()
	if value != expected {
		t.Errorf("%s: expected length %d, got %d", msg, expected, value)
	}
}

// ============================================================================
// FATAL HANDLER HELPERS
// ============================================================================

// NewFatalHandler creates a fatal handler that sets a flag when called.
// Useful for testing fatal-level logs without actually exiting.
func NewFatalHandler() (handler func(), wasCalled *bool) {
	called := false
	return func() { called = true }, &called
}

// NewFatalHandlerWithBuffer creates a fatal handler that captures the exit state.
func NewFatalHandlerWithBuffer(buf *bytes.Buffer) func() {
	return func() {
		buf.WriteString("[FATAL_HANDLER_CALLED]")
	}
}

// ============================================================================
// TEST BUFFER HELPERS
// ============================================================================

// NewTestBuffer creates a bytes.Buffer for capturing log output.
func NewTestBuffer() *bytes.Buffer {
	return &bytes.Buffer{}
}

// ResetAndGet resets the buffer and returns its contents.
func ResetAndGet(buf *bytes.Buffer) string {
	output := buf.String()
	buf.Reset()
	return output
}

// ============================================================================
// COMMON TEST CONFIGURATIONS
// ============================================================================

// NewTestConfigWithBuffer returns a default config with output set to the buffer.
// This is a convenience function for simple test cases.
func NewTestConfigWithBuffer(buf *bytes.Buffer) Config {
	cfg := DefaultConfig()
	cfg.Output = buf
	cfg.Level = LevelDebug
	return cfg
}

// NewTestJSONConfigWithBuffer returns a JSON format config with output set to the buffer.
func NewTestJSONConfigWithBuffer(buf *bytes.Buffer) Config {
	cfg := DefaultConfig()
	cfg.Output = buf
	cfg.Level = LevelDebug
	cfg.Format = FormatJSON
	cfg.JSON = DefaultJSONOptions()
	return cfg
}
