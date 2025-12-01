package logformat

import (
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/dd/internal/caller"
)

func TestFormatMessage(t *testing.T) {
	tests := []struct {
		name            string
		level           LogLevel
		includeTime     bool
		timeFormat      string
		includeLevel    bool
		includeCaller   bool
		callerDepth     int
		fullPath        bool
		args            []any
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:          "basic message",
			level:         LevelInfo,
			includeTime:   false,
			includeLevel:  true,
			includeCaller: false,
			args:          []any{"test message"},
			wantContains:  []string{"[INFO]", "test message"},
		},
		{
			name:          "with time",
			level:         LevelError,
			includeTime:   true,
			timeFormat:    "15:04:05",
			includeLevel:  true,
			includeCaller: false,
			args:          []any{"error occurred"},
			wantContains:  []string{"[ERROR]", "error occurred"},
		},
		{
			name:          "with caller",
			level:         LevelWarn,
			includeTime:   false,
			includeLevel:  true,
			includeCaller: true,
			callerDepth:   2, // Skip FormatMessage and this test function
			fullPath:      false,
			args:          []any{"warning message"},
			wantContains:  []string{"[WARN]", "warning message", "format_test.go"},
		},
		{
			name:          "with full path caller",
			level:         LevelDebug,
			includeTime:   false,
			includeLevel:  true,
			includeCaller: true,
			callerDepth:   2, // Skip FormatMessage and this test function
			fullPath:      true,
			args:          []any{"debug message"},
			wantContains:  []string{"[DEBUG]", "debug message", "format_test.go"},
		},
		{
			name:            "minimal config",
			level:           LevelInfo,
			includeTime:     false,
			includeLevel:    false,
			includeCaller:   false,
			args:            []any{"minimal message"},
			wantContains:    []string{"minimal message"},
			wantNotContains: []string{"[INFO]", "[", "]"},
		},
		{
			name:          "multiple args",
			level:         LevelInfo,
			includeTime:   false,
			includeLevel:  true,
			includeCaller: false,
			args:          []any{"user", "john", "has", 42, "items"},
			wantContains:  []string{"[INFO]", "user john has 42 items"},
		},
		{
			name:          "all features enabled",
			level:         LevelFatal,
			includeTime:   true,
			timeFormat:    time.RFC3339,
			includeLevel:  true,
			includeCaller: true,
			callerDepth:   2, // Skip FormatMessage and this test function
			fullPath:      false,
			args:          []any{"fatal error"},
			wantContains:  []string{"[FATAL]", "fatal error", "format_test.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMessage(
				tt.level,
				tt.includeTime,
				tt.timeFormat,
				tt.includeLevel,
				tt.includeCaller,
				tt.callerDepth,
				tt.fullPath,
				tt.args...,
			)

			// Check required content
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("Result should contain %q, got: %s", want, result)
				}
			}

			// Check excluded content
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(result, notWant) {
					t.Errorf("Result should not contain %q, got: %s", notWant, result)
				}
			}

			// Verify time format if enabled
			if tt.includeTime {
				// Should contain timestamp in brackets
				if !strings.Contains(result, "[") || !strings.Contains(result, "]") {
					t.Error("Time should be in brackets")
				}
			}

			// Verify caller format if enabled
			if tt.includeCaller {
				// Should contain filename and line number
				if !strings.Contains(result, ":") {
					t.Error("Caller should contain line number")
				}
			}
		})
	}
}

func TestLevelToString(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
		{LogLevel(-1), "UNKNOWN"},
		{LogLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.level.String()
			if got != tt.want {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetCaller(t *testing.T) {
	// Test with full path (depth 1 to get this test function)
	callerInfo := caller.GetCaller(1, true)
	if !strings.Contains(callerInfo, "format_test.go") {
		t.Errorf("GetCaller(true) should contain file name, got: %s", callerInfo)
	}
	if !strings.Contains(callerInfo, ":") {
		t.Error("GetCaller() should contain line number")
	}

	// Test without full path
	callerInfo = caller.GetCaller(1, false)
	if !strings.Contains(callerInfo, "format_test.go") {
		t.Errorf("GetCaller(false) should contain file name, got: %s", callerInfo)
	}
	// Should not contain full path separators
	if strings.Count(callerInfo, "/") > 0 || strings.Count(callerInfo, "\\") > 0 {
		t.Errorf("GetCaller(false) should not contain path separators, got: %s", callerInfo)
	}

	// Test with invalid depth
	callerInfo = caller.GetCaller(100, false)
	if callerInfo != "" {
		t.Errorf("GetCaller(100) should return empty string, got: %s", callerInfo)
	}
}

func TestFormatMessageWithDifferentTypes(t *testing.T) {
	tests := []struct {
		name string
		args []any
		want string
	}{
		{
			name: "string",
			args: []any{"hello world"},
			want: "hello world",
		},
		{
			name: "integer",
			args: []any{42},
			want: "42",
		},
		{
			name: "float",
			args: []any{3.14},
			want: "3.14",
		},
		{
			name: "boolean",
			args: []any{true},
			want: "true",
		},
		{
			name: "nil",
			args: []any{nil},
			want: "<nil>",
		},
		{
			name: "mixed types",
			args: []any{"count:", 42, "active:", true},
			want: "count: 42 active: true",
		},
		{
			name: "struct",
			args: []any{struct{ Name string }{"test"}},
			want: "{test}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMessage(
				LevelInfo,
				false,
				"",
				false,
				false,
				0,
				false,
				tt.args...,
			)

			if !strings.Contains(result, tt.want) {
				t.Errorf("FormatMessage() should contain %q, got: %s", tt.want, result)
			}
		})
	}
}

func TestFormatMessageTimeFormats(t *testing.T) {
	tests := []struct {
		name       string
		timeFormat string
		verify     func(string) bool
	}{
		{
			name:       "RFC3339",
			timeFormat: time.RFC3339,
			verify: func(result string) bool {
				// Should contain ISO format timestamp
				return strings.Contains(result, "T") && strings.Contains(result, ":")
			},
		},
		{
			name:       "simple time",
			timeFormat: "15:04:05",
			verify: func(result string) bool {
				// Should contain HH:MM:SS format
				parts := strings.Split(result, " ")
				for _, part := range parts {
					if strings.Count(part, ":") == 2 && len(part) >= 8 {
						return true
					}
				}
				return false
			},
		},
		{
			name:       "date only",
			timeFormat: "2006-01-02",
			verify: func(result string) bool {
				// Should contain date format
				return strings.Contains(result, "-")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMessage(
				LevelInfo,
				true,
				tt.timeFormat,
				true,
				false,
				0,
				false,
				"test message",
			)

			if !tt.verify(result) {
				t.Errorf("Time format verification failed for %s, got: %s", tt.timeFormat, result)
			}
		})
	}
}

func TestFormatMessageCallerDepth(t *testing.T) {
	// Helper function to test caller depth
	testCaller := func(depth int) string {
		return FormatMessage(
			LevelInfo,
			false,
			"",
			false,
			true,
			depth,
			false,
			"test",
		)
	}

	// Test different depths
	result0 := testCaller(0)
	result1 := testCaller(1)
	result2 := testCaller(2)

	// Results should be different (different line numbers or functions)
	if result0 == result1 {
		t.Error("Different caller depths should produce different results")
	}

	// All should contain some caller info
	for i, result := range []string{result0, result1, result2} {
		if !strings.Contains(result, ":") {
			t.Errorf("Result %d should contain caller info: %s", i, result)
		}
	}
}

func TestFormatMessageEmptyArgs(t *testing.T) {
	result := FormatMessage(
		LevelInfo,
		false,
		"",
		true,
		false,
		0,
		false,
	)

	// Should contain level but empty message
	if !strings.Contains(result, "[INFO]") {
		t.Error("Should contain level")
	}

	// Message part should be empty or minimal
	parts := strings.Split(result, " ")
	if len(parts) < 1 {
		t.Error("Should have at least level part")
	}
}

func TestFormatMessageSpecialCharacters(t *testing.T) {
	specialMessage := "message with\nnewlines\tand\ttabs"

	result := FormatMessage(
		LevelInfo,
		false,
		"",
		true,
		false,
		0,
		false,
		specialMessage,
	)

	// Should contain the special characters as-is (no escaping in text format)
	if !strings.Contains(result, specialMessage) {
		t.Errorf("Should preserve special characters, got: %s", result)
	}
}

func TestFormatMessageLongMessage(t *testing.T) {
	longMessage := strings.Repeat("a", 1000)

	result := FormatMessage(
		LevelInfo,
		false,
		"",
		true,
		false,
		0,
		false,
		longMessage,
	)

	// Should handle long messages without issues
	if !strings.Contains(result, longMessage) {
		t.Error("Should handle long messages")
	}
}
