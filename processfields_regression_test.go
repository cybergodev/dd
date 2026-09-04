package dd

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// Regression tests for processFields' filtered-vs-original comparison. The
// copy-on-write fast path used to compare the two `any` values with ==, which
// panics whenever both hold an uncomparable dynamic type (map, slice, func) —
// i.e., any structured log call with a container field while sensitive-data
// filtering is enabled.

func TestValueUnchanged(t *testing.T) {
	tests := []struct {
		name      string
		filtered  any
		original  any
		unchanged bool
	}{
		{"identical strings", "a", "a", true},
		{"different strings", "a", "b", false},
		{"identical ints", 42, 42, true},
		{"nil both", nil, nil, true},
		{"nil filtered only", nil, "a", false},
		{"nil original only", "a", nil, false},
		{"type change", map[string]any{}, map[string]int{}, false},
		// Uncomparable dynamic types must report changed, never panic.
		{"same map type", map[string]any{"a": 1}, map[string]any{"a": 1}, false},
		{"same slice type", []any{1}, []any{1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := valueUnchanged(tt.filtered, tt.original); got != tt.unchanged {
				t.Errorf("valueUnchanged(%v, %v) = %v, want %v",
					tt.filtered, tt.original, got, tt.unchanged)
			}
		})
	}
}

// TestProcessFieldsComplexValuesNoPanic: logging map/slice-typed field values
// with filtering enabled must not panic, and nested sensitive values must
// still be redacted.
func TestProcessFieldsComplexValuesNoPanic(t *testing.T) {
	var buf bytes.Buffer
	cfg := DefaultConfig()
	cfg.Format = FormatJSON
	cfg.Security = DefaultSecurityConfig()
	cfg.Targets = []OutputTarget{CustomOutput(&buf)}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	logger.InfoWith("complex field values",
		Any("request", map[string]any{
			"path": "/login",
			"body": map[string]any{"user": "john", "password": "secret123"},
		}),
		Any("tags", []string{"vip", "premium"}),
	)

	out := buf.String()
	if strings.Contains(out, "secret123") {
		t.Errorf("sensitive value leaked into output: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("expected [REDACTED] marker for nested password, got: %s", out)
	}
	if !strings.Contains(out, "vip") {
		t.Errorf("non-sensitive slice content missing from output: %s", out)
	}
}

// TestProcessFieldsComparableUnchangedSkipsHook: when nothing was redacted and
// the value is comparable and unchanged, the field must not be treated as
// filtered (no HookOnFilter fired).
func TestProcessFieldsComparableUnchangedSkipsHook(t *testing.T) {
	hookFired := false

	cfg := DefaultConfig()
	cfg.Security = DefaultSecurityConfig()
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer logger.Close()

	if err := logger.AddHook(HookOnFilter, func(_ context.Context, _ *HookContext) error {
		hookFired = true
		return nil
	}); err != nil {
		t.Fatalf("AddHook() error = %v", err)
	}

	// No sensitive key present anywhere; the string field forces a filter
	// pass (pattern scan) but nothing is redacted.
	logger.InfoWith("clean entry",
		String("user", "john"),
		Int("count", 42),
	)

	if hookFired {
		t.Error("HookOnFilter fired although nothing was redacted")
	}
}
