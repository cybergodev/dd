package dd

import "testing"

// Regression: dd's JSON output nests structured fields under the "fields"
// key; the recorder used to record that whole object as a single "fields"
// field, so ContainsField/GetFieldValue never saw the individual keys.
func TestRecorderJSONNestedFields(t *testing.T) {
	recorder := NewLoggerRecorder()
	recorder.SetFormat(FormatJSON)
	logger, err := recorder.NewLogger(JSONConfig())
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	logger.InfoWith("HTTP request",
		String("method", "GET"),
		Int("status", 200),
	)

	entries := recorder.Entries()
	if len(entries) != 1 {
		t.Fatalf("captured %d entries, want 1 (raw: %v)", len(entries), entries)
	}
	if entries[0].Message != "HTTP request" {
		t.Errorf("Message = %q, want %q", entries[0].Message, "HTTP request")
	}
	if !recorder.ContainsField("method") {
		t.Error("ContainsField(method) = false, want true")
	}
	if got := recorder.GetFieldValue("method"); got != "GET" {
		t.Errorf("GetFieldValue(method) = %v, want GET", got)
	}
	if !recorder.ContainsField("status") {
		t.Error("ContainsField(status) = false, want true")
	}
	// The wrapper object itself must not surface as a field.
	if recorder.ContainsField("fields") {
		t.Error(`ContainsField("fields") = true, want false (nested object must be flattened)`)
	}
}
