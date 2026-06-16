package internal

import (
	"strings"
	"testing"
)

func TestGetCallerComprehensive(t *testing.T) {
	tests := []struct {
		name        string
		depth       int
		fullPath    bool
		wantContain string
		dontWant    string
		wantEmpty   bool
	}{
		{
			name:        "depth 1 with full path",
			depth:       1,
			fullPath:    true,
			wantContain: "caller_test.go",
		},
		{
			name:        "depth 1 without full path",
			depth:       1,
			fullPath:    false,
			wantContain: "caller_test.go",
		},
		{
			name:      "invalid high depth",
			depth:     1000,
			fullPath:  false,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCaller(tt.depth, tt.fullPath)

			if tt.wantEmpty {
				if result != "" {
					t.Errorf("GetCaller(%d, %v) should return empty, got %q", tt.depth, tt.fullPath, result)
				}
				return
			}

			if !strings.Contains(result, ":") {
				t.Errorf("GetCaller() should contain ':', got %q", result)
			}

			if tt.wantContain != "" && !strings.Contains(result, tt.wantContain) {
				t.Errorf("GetCaller() should contain %q, got %q", tt.wantContain, result)
			}

			if !tt.fullPath && (strings.Contains(result, "\\") || strings.Contains(result, "/")) {
				t.Errorf("GetCaller(fullPath=false) should not contain path separators, got %q", result)
			}
		})
	}
}

func TestGetCallerConsistency(t *testing.T) {
	results := make([]string, 10)
	for i := range 10 {
		results[i] = GetCaller(1, false)
	}

	for i := 1; i < 10; i++ {
		if results[i] != results[0] {
			t.Errorf("Inconsistent results: [%d]=%q vs [0]=%q", i, results[i], results[0])
		}
	}
}

func TestGetCallerLineNumber(t *testing.T) {
	callerInfo := GetCaller(1, false)

	parts := strings.Split(callerInfo, ":")
	if len(parts) != 2 {
		t.Errorf("Expected format 'file:line', got: %s", callerInfo)
		return
	}

	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			t.Errorf("Line number should be numeric, got: %s", parts[1])
			return
		}
	}
}

func TestGetCallerFromHelper(t *testing.T) {
	caller := getCallerHelper()
	if !strings.Contains(caller, "caller_test.go") {
		t.Errorf("Caller should be from test file, got: %s", caller)
	}
}

func getCallerHelper() string {
	return GetCaller(2, false)
}

func TestGetCallerWithDeepStack(t *testing.T) {
	result := deepCallStack(5)
	if result == "" {
		t.Error("Expected non-empty caller info")
	}
}

func deepCallStack(depth int) string {
	if depth == 0 {
		return GetCaller(1, false)
	}
	return deepCallStack(depth - 1)
}

func TestCallerBuilderPool(t *testing.T) {
	for range 1000 {
		_ = GetCaller(1, true)
	}
}
