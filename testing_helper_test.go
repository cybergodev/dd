package dd

import (
	"bytes"
	"testing"
)

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

// NewTestJSONConfigWithBuffer returns a JSON format config with output set to the buffer.
func NewTestJSONConfigWithBuffer(buf *bytes.Buffer) Config {
	cfg := DefaultConfig()
	cfg.Targets = []OutputTarget{CustomOutput(buf)}
	cfg.Level = LevelDebug
	cfg.Format = FormatJSON
	cfg.JSON = DefaultJSONOptions()
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
