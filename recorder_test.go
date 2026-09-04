package dd

import (
	"testing"
)

func TestLoggerRecorder_Levels(t *testing.T) {
	recorder := NewLoggerRecorder()

	cfg := DefaultConfig()
	cfg.Level = LevelDebug
	logger, _ := recorder.NewLogger(cfg)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	entries := recorder.Entries()
	if len(entries) != 4 {
		t.Errorf("Expected 4 entries, got %d", len(entries))
	}

	expectedLevels := []LogLevel{LevelDebug, LevelInfo, LevelWarn, LevelError}
	for i, entry := range entries {
		if entry.Level != expectedLevels[i] {
			t.Errorf("Entry %d: expected level %v, got %v", i, expectedLevels[i], entry.Level)
		}
	}
}

func TestLoggerRecorder_EntriesAtLevel(t *testing.T) {
	recorder := NewLoggerRecorder()
	logger, _ := recorder.NewLogger()

	logger.Info("info 1")
	logger.Warn("warn 1")
	logger.Info("info 2")
	logger.Error("error 1")

	infoEntries := recorder.EntriesAtLevel(LevelInfo)
	if len(infoEntries) != 2 {
		t.Errorf("Expected 2 info entries, got %d", len(infoEntries))
	}

	warnEntries := recorder.EntriesAtLevel(LevelWarn)
	if len(warnEntries) != 1 {
		t.Errorf("Expected 1 warn entry, got %d", len(warnEntries))
	}
}

func TestLoggerRecorder_LastEntry(t *testing.T) {
	recorder := NewLoggerRecorder()
	logger, _ := recorder.NewLogger()

	if recorder.LastEntry() != nil {
		t.Error("Expected nil for empty recorder")
	}

	logger.Info("first")
	logger.Warn("second")

	last := recorder.LastEntry()
	if last == nil {
		t.Fatal("Expected last entry, got nil")
	}
	if last.Level != LevelWarn {
		t.Errorf("Expected level %v, got %v", LevelWarn, last.Level)
	}
}

func TestLoggerRecorder_ContainsMessage(t *testing.T) {
	recorder := NewLoggerRecorder()
	logger, _ := recorder.NewLogger()

	logger.Info("hello world")
	logger.Info("another message")

	if !recorder.ContainsMessage("hello world") {
		t.Error("Expected to find 'hello world'")
	}
	if !recorder.ContainsMessage("hello") {
		t.Error("Expected to find 'hello' as substring")
	}
	if recorder.ContainsMessage("not found") {
		t.Error("Did not expect to find 'not found'")
	}
}

// TestLoggerRecorder_Clear was removed: coverage_test.go
// TestLoggerRecorder_ClearComplete asserts the same post-Clear state plus
// LastEntry/EntriesAtLevel.

func TestLoggerRecorder_WithFields(t *testing.T) {
	recorder := NewLoggerRecorder()
	logger, _ := recorder.NewLogger()

	logger.InfoWith("test message",
		String("user", "john"),
		Int("count", 42),
	)

	entries := recorder.Entries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if !recorder.ContainsField("user") {
		t.Error("Expected to find 'user' field")
	}
	if !recorder.ContainsField("count") {
		t.Error("Expected to find 'count' field")
	}

	val := recorder.GetFieldValue("user")
	if val == nil {
		t.Error("Expected to get value for 'user' field")
	} else if val.(string) != "john" {
		t.Errorf("Expected 'john', got %v", val)
	}
}

func TestLoggerRecorder_RawOutput(t *testing.T) {
	recorder := NewLoggerRecorder()
	logger, _ := recorder.NewLogger()

	logger.Info("test")

	entries := recorder.Entries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].RawOutput == "" {
		t.Error("Expected RawOutput to be populated")
	}
}

func TestLoggerRecorder_JSONFormat(t *testing.T) {
	recorder := NewLoggerRecorder()
	recorder.SetFormat(FormatJSON)

	cfg := DefaultConfig()
	cfg.Format = FormatJSON
	logger, _ := recorder.NewLogger(cfg)

	logger.Info("json test")

	entries := recorder.Entries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].Format != FormatJSON {
		t.Errorf("Expected JSON format, got %v", entries[0].Format)
	}
}

func TestLoggerRecorder_CustomConfig(t *testing.T) {
	recorder := NewLoggerRecorder()

	cfg := DefaultConfig()
	cfg.Level = LevelDebug
	logger, _ := recorder.NewLogger(cfg)

	logger.Debug("debug message")

	if !recorder.HasEntries() {
		t.Error("Expected debug entry to be logged")
	}
}

func TestLoggerRecorder_ThreadSafety(t *testing.T) {
	recorder := NewLoggerRecorder()
	logger, _ := recorder.NewLogger()

	// Run multiple goroutines writing to the same recorder
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				logger.InfoWith("message",
					Int("goroutine", id),
					Int("iteration", j),
				)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have 1000 entries
	if recorder.Count() != 1000 {
		t.Errorf("Expected 1000 entries, got %d", recorder.Count())
	}
}

func TestParseLevelString(t *testing.T) {
	tests := []struct {
		input    string
		expected LogLevel
	}{
		{"DEBUG", LevelDebug},
		{"INFO", LevelInfo},
		{"WARN", LevelWarn},
		{"WARNING", LevelWarn},
		{"ERROR", LevelError},
		{"FATAL", LevelFatal},
		{"UNKNOWN", LevelInfo}, // defaults to Info
	}

	for _, tc := range tests {
		result := parseLevelString(tc.input)
		if result != tc.expected {
			t.Errorf("parseLevelString(%q) = %v, want %v", tc.input, result, tc.expected)
		}
	}
}

// TestTrimNewline pins the line-ending normalization applied to recorded
// output: exactly one trailing LF and then one trailing CR are removed.
func TestTrimNewline(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"no newline", "value", "value"},
		{"trailing LF", "value\n", "value"},
		{"trailing CR", "value\r", "value"},
		{"trailing CRLF", "value\r\n", "value"},
		{"LF only", "\n", ""},
		{"CRLF only", "\r\n", ""},
		{"embedded LF kept", "a\nb", "a\nb"},
		{"double LF strips one", "value\n\n", "value\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimNewline(tt.input); got != tt.want {
				t.Errorf("trimNewline(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestLoggerRecorder_QuotedFieldValues pins that the recorder's text parser
// handles the formatter's value quoting correctly. A value ENDING with a
// quote made the old quote-state tracker unbalanced (escaped quotes toggled
// it), so the token swallowed the space separator and every following field
// ("b" below) silently vanished from the parsed entry; quoted values were
// also returned with their surrounding quotes as part of the string.
func TestLoggerRecorder_QuotedFieldValues(t *testing.T) {
	recorder := NewLoggerRecorder()
	logger, _ := recorder.NewLogger()

	logger.InfoWith("quoted values",
		String("note", `value "quoted"`), // ends with a quote: old parser went unbalanced
		String("b", "2"),                 // old parser swallowed this token into "note"
		String("spaced", "hello world"),
	)

	if got, ok := recorder.GetFieldValue("note").(string); !ok || got != `value "quoted"` {
		t.Errorf("GetFieldValue(note) = %v, want %q", recorder.GetFieldValue("note"), `value "quoted"`)
	}
	if got := recorder.GetFieldValue("b"); got != "2" {
		t.Errorf("GetFieldValue(b) = %v, want \"2\" — field after escaped-quote value must not be swallowed", got)
	}
	if got, ok := recorder.GetFieldValue("spaced").(string); !ok || got != "hello world" {
		t.Errorf("GetFieldValue(spaced) = %v, want %q (surrounding quotes must be stripped)", recorder.GetFieldValue("spaced"), "hello world")
	}
}
