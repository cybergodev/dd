package caller

import (
	"strings"
	"testing"
)

func TestGetCaller(t *testing.T) {
	tests := []struct {
		name     string
		depth    int
		fullPath bool
		wantFile string
		wantPath bool
	}{
		{
			name:     "valid depth with full path",
			depth:    1,
			fullPath: true,
			wantFile: "caller_test.go",
			wantPath: true,
		},
		{
			name:     "valid depth without full path",
			depth:    1,
			fullPath: false,
			wantFile: "caller_test.go",
			wantPath: false,
		},
		{
			name:     "invalid depth",
			depth:    100,
			fullPath: false,
			wantFile: "",
			wantPath: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCaller(tt.depth, tt.fullPath)

			if tt.wantFile == "" {
				if result != "" {
					t.Errorf("GetCaller(%d, %v) = %q, want empty string", tt.depth, tt.fullPath, result)
				}
				return
			}

			if !strings.Contains(result, tt.wantFile) {
				t.Errorf("GetCaller(%d, %v) = %q, want to contain %q", tt.depth, tt.fullPath, result, tt.wantFile)
			}

			if !strings.Contains(result, ":") {
				t.Errorf("GetCaller(%d, %v) = %q, want to contain line number", tt.depth, tt.fullPath, result)
			}

			hasPathSep := strings.Contains(result, "/") || strings.Contains(result, "\\")
			if tt.wantPath && !hasPathSep {
				t.Errorf("GetCaller(%d, true) = %q, want to contain path separator", tt.depth, result)
			}
			if !tt.wantPath && hasPathSep {
				t.Errorf("GetCaller(%d, false) = %q, want no path separator", tt.depth, result)
			}
		})
	}
}

func TestGetCallerConsistency(t *testing.T) {
	result := GetCaller(1, false)

	if !strings.Contains(result, "caller_test.go") {
		t.Errorf("GetCaller should contain test file name, got: %q", result)
	}

	if !strings.Contains(result, ":") {
		t.Errorf("GetCaller should contain line number, got: %q", result)
	}

	// Verify format is "filename:line"
	parts := strings.Split(result, ":")
	if len(parts) != 2 {
		t.Errorf("GetCaller should return 'filename:line' format, got: %q", result)
	}
}
