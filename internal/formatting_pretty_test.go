package internal

import (
	"strings"
	"testing"
)

// TestFormatJSONPrettyDirect pins the pretty-print fast path's byte layout.
// The direct writer is deterministic (timestamp, level, caller, message,
// fields — in that order), unlike the map-based path whose key order follows
// map iteration, so an exact-string comparison is possible and worthwhile:
// the layout must stay shape-compatible with encoding/json's indented output
// (newline + indent nesting, ": " after keys, closing braces on their own
// lines).
func TestFormatJSONPrettyDirect(t *testing.T) {
	formatter := NewMessageFormatter(&FormatterConfig{
		Format:       LogFormatJSON,
		TimeFormat:   "15:04:05",
		IncludeTime:  false,
		IncludeLevel: true,
		FullPath:     false,
		JSON: &JSONOptions{
			PrettyPrint: true,
			Indent:      "  ",
		},
	})
	buf := formatter.FormatWithMessageBytes(LevelInfo, "file.go:42", "hello", []Field{
		{Key: "k1", Value: "v1"},
		{Key: "k2", Value: 42},
		{Key: "k3", Value: true},
	})
	got := buf.String()
	PutLineBuffer(buf)

	want := strings.Join([]string{
		`{`,
		`  "level": "INFO",`,
		`  "caller": "file.go:42",`,
		`  "message": "hello",`,
		`  "fields": {`,
		`    "k1": "v1",`,
		`    "k2": 42,`,
		`    "k3": true`,
		`  }`,
		`}`,
	}, "\n")
	if got != want {
		t.Errorf("pretty fast path output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatJSONPrettyFallback checks that a complex field value in pretty
// mode bypasses the direct writer and takes the encoding/json path, which
// indents nested structures.
func TestFormatJSONPrettyFallback(t *testing.T) {
	formatter := NewMessageFormatter(&FormatterConfig{
		Format: LogFormatJSON,
		JSON: &JSONOptions{
			PrettyPrint: true,
			Indent:      "\t",
		},
	})
	buf := formatter.FormatWithMessageBytes(LevelInfo, "", "hi", []Field{
		{Key: "nested", Value: map[string]any{"a": 1}},
	})
	got := buf.String()
	PutLineBuffer(buf)

	if !strings.Contains(got, "\"nested\"") || !strings.Contains(got, "\"a\"") {
		t.Errorf("pretty fallback output missing nested field: %s", got)
	}
	if !strings.Contains(got, "\n\t") {
		t.Errorf("pretty fallback output not indented: %s", got)
	}
}

// TestFormatJSONPrettyCompactUnchanged verifies the compact direct path is
// untouched by the pretty support (no newlines, no spaces after colons).
func TestFormatJSONPrettyCompactUnchanged(t *testing.T) {
	formatter := NewMessageFormatter(&FormatterConfig{
		Format: LogFormatJSON,
		JSON:   &JSONOptions{PrettyPrint: false},
	})
	buf := formatter.FormatWithMessageBytes(LevelInfo, "", "hi", []Field{
		{Key: "k", Value: "v"},
	})
	got := buf.String()
	PutLineBuffer(buf)

	if strings.ContainsAny(got, "\n\t") || !strings.Contains(got, `"k":"v"`) {
		t.Errorf("compact fast path output changed: %s", got)
	}
}
