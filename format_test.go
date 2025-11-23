package dd

import (
	"testing"
)

// ============================================================================
// LOG FORMAT TESTS
// ============================================================================

func TestLogFormatString(t *testing.T) {
	tests := []struct {
		format LogFormat
		want   string
	}{
		{FormatText, "text"},
		{FormatJSON, "json"},
		{LogFormat(99), "unknown"},
		{LogFormat(-1), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.format.String()
			if got != tt.want {
				t.Errorf("LogFormat.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogFormatValues(t *testing.T) {
	// Verify format constants have expected values
	if FormatText != 0 {
		t.Errorf("FormatText = %d, want 0", FormatText)
	}

	if FormatJSON != 1 {
		t.Errorf("FormatJSON = %d, want 1", FormatJSON)
	}
}

// ============================================================================
// LOG LEVEL TESTS
// ============================================================================

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
		{LogLevel(99), "UNKNOWN"},
		{LogLevel(-1), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.level.String()
			if got != tt.want {
				t.Errorf("LogLevel.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogLevelValues(t *testing.T) {
	// Verify level constants have expected values
	if LevelDebug != 0 {
		t.Errorf("LevelDebug = %d, want 0", LevelDebug)
	}

	if LevelInfo != 1 {
		t.Errorf("LevelInfo = %d, want 1", LevelInfo)
	}

	if LevelWarn != 2 {
		t.Errorf("LevelWarn = %d, want 2", LevelWarn)
	}

	if LevelError != 3 {
		t.Errorf("LevelError = %d, want 3", LevelError)
	}

	if LevelFatal != 4 {
		t.Errorf("LevelFatal = %d, want 4", LevelFatal)
	}
}

func TestLogLevelOrdering(t *testing.T) {
	// Verify levels are in correct order
	if !(LevelDebug < LevelInfo) {
		t.Error("LevelDebug should be less than LevelInfo")
	}

	if !(LevelInfo < LevelWarn) {
		t.Error("LevelInfo should be less than LevelWarn")
	}

	if !(LevelWarn < LevelError) {
		t.Error("LevelWarn should be less than LevelError")
	}

	if !(LevelError < LevelFatal) {
		t.Error("LevelError should be less than LevelFatal")
	}
}
