package dd

import (
	"bytes"
	"testing"
)

// ============================================================================
// LOGGERERROR TEST CONSTRUCTORS
// ============================================================================

// newError creates a new LoggerError with the given code and message.
// Test-only constructor: production code returns plain sentinel-wrapped
// errors (fmt.Errorf + %w), so these helpers live with the tests that
// exercise LoggerError's Is/Unwrap behavior directly.
func newError(code, message string) *LoggerError {
	return &LoggerError{
		Code:    code,
		Message: message,
	}
}

// wrapError wraps an existing error with a code and message.
// If the error is nil, returns nil. Test-only (see newError).
func wrapError(code, message string, cause error) *LoggerError {
	if cause == nil {
		return nil
	}
	return &LoggerError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// ============================================================================
// COMMON TEST CONFIGURATIONS
// ============================================================================

// NewTestConfigWithBuffer returns a default config with output set to the buffer.
// This is a convenience function for simple test cases.
func NewTestConfigWithBuffer(buf *bytes.Buffer) Config {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(buf)}
	cfg.Level = LevelDebug
	return cfg
}

// ============================================================================
// ENUM STRING() ROUND-TRIP HELPER
// ============================================================================

// stringerCase is one row of an enum String() round-trip table.
type stringerCase[T any] struct {
	value T
	want  string
}

// assertEnumStringer drives the *_String tests that used to each duplicate the
// same "build slice, range, t.Run, compare" boilerplate. stringer is typically a
// method expression such as HookEvent.String.
func assertEnumStringer[T any](t *testing.T, name string, cases []stringerCase[T], stringer func(T) string) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := stringer(tc.value); got != tc.want {
				t.Errorf("%s(%v).String() = %q, want %q", name, tc.value, got, tc.want)
			}
		})
	}
}
