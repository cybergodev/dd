package internal

// Boundary tests targeting previously-uncovered functions in the internal
// package (added as part of FIX-001 test-suite optimization). Each test
// exercises a concrete behavior branch that had 0% / low coverage.

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ----------------------------------------------------------------------------
// ValidateTimeFormat (validation.go)
// ----------------------------------------------------------------------------

func TestValidateTimeFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		// Empty is valid (the caller falls back to the default format).
		{"empty", "", false},
		// Standard layouts round-trip cleanly through Format/Parse.
		{"RFC3339", time.RFC3339, false},
		{"Kitchen", time.Kitchen, false},
		{"DateOnly", "2006-01-02", false},
		// "1_2" cannot round-trip: Format renders month+day with no separator
		// ("616" for June 16) and Parse then reads a two-digit month (61),
		// which is out of range. This is exactly the malformed-format case
		// ValidateTimeFormat exists to reject.
		{"non-roundtripping", "1_2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeFormat(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTimeFormat(%q) error = %v, wantErr %v", tt.format, err, tt.wantErr)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// SecureBuffer nil-safety + Grow reallocation (securemem.go)
// ----------------------------------------------------------------------------

func TestSecureBufferNilSafe(t *testing.T) {
	// Every method must tolerate a nil receiver without panicking.
	var sb *SecureBuffer
	if got := sb.Len(); got != 0 {
		t.Errorf("nil Len() = %d, want 0", got)
	}
	if got := sb.Cap(); got != 0 {
		t.Errorf("nil Cap() = %d, want 0", got)
	}
	sb.Reset()
	sb.Release()
	sb.Grow(8)
}

func TestSecureBufferGrowReallocates(t *testing.T) {
	// Regression: Grow panicked whenever len(data) < cap(data). It ranged
	// over sb.data[:cap] (the full backing array) but wrote through sb.data
	// (whose len < cap), so any index >= len was out of range.
	//
	// Write a short payload so len < cap, then force reallocation.
	sb := NewSecureBuffer()
	defer sb.Release()

	if _, err := sb.WriteString("secret"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if sb.Len() >= sb.Cap() {
		t.Fatalf("precondition: need len < cap, got len=%d cap=%d", sb.Len(), sb.Cap())
	}

	// Capture the pre-grow backing array to verify it is zeroed afterward.
	oldBacking := sb.data
	oldCap := cap(sb.data)

	before := sb.Cap()
	sb.Grow(sb.Cap()) // request cap extra bytes > free space → forces realloc

	// (1) Reached here without panicking; capacity grew.
	if sb.Cap() <= before {
		t.Errorf("after Grow, Cap() = %d, want > %d", sb.Cap(), before)
	}
	// (2) Content preserved across reallocation.
	if got := sb.String(); got != "secret" {
		t.Errorf("content not preserved: got %q, want %q", got, "secret")
	}
	// (3) Security: old backing array fully zeroed, including the capacity tail.
	for i, b := range oldBacking[:oldCap] {
		if b != 0 {
			t.Errorf("old backing array not zeroed at index %d: got %d", i, b)
		}
	}
}

// ----------------------------------------------------------------------------
// isSimpleJSONValue + formatJSON slow path (formatting.go)
// ----------------------------------------------------------------------------

func TestIsSimpleJSONValue(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want bool
	}{
		{"string", "s", true},
		{"int", 1, true},
		{"int64", int64(1), true},
		{"uint", uint(1), true},
		{"float64", 1.5, true},
		{"bool", true, true},
		{"nil", nil, true},
		{"time", time.Time{}, true},
		{"duration", time.Second, true},
		{"[]string", []string{"a"}, true},
		{"[]int", []int{1}, true},
		{"struct", struct{ A int }{}, false},
		{"map", map[string]int{"a": 1}, false},
		{"[]any", []any{1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSimpleJSONValue(tt.v); got != tt.want {
				t.Errorf("isSimpleJSONValue(%T) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func TestFormatJSONSlowPath(t *testing.T) {
	// Force the map-based JSON path (formatJSON): PrettyPrint disables the
	// fast path, and a complex field value is not a "simple" JSON value.
	formatter := NewMessageFormatter(&FormatterConfig{
		Format:      LogFormatJSON,
		TimeFormat:  time.RFC3339,
		IncludeTime: true,
		JSON: &JSONOptions{
			PrettyPrint: true,
			Indent:      "  ",
		},
	})
	out := formatter.FormatWithMessage(LevelInfo, 0, "hello", []Field{
		{Key: "complex", Value: map[string]any{"nested": 1}},
	})
	if !strings.Contains(out, "hello") {
		t.Errorf("JSON slow-path output missing message: %s", out)
	}
	if !strings.Contains(out, "complex") {
		t.Errorf("JSON slow-path output missing field key: %s", out)
	}
}

// ----------------------------------------------------------------------------
// MapKeyToString (types.go)
// ----------------------------------------------------------------------------

func TestMapKeyToString(t *testing.T) {
	tests := []struct {
		name string
		key  reflect.Value
		want string
	}{
		{"string", reflect.ValueOf("abc"), "abc"},
		{"int", reflect.ValueOf(int(42)), "42"},
		{"int64", reflect.ValueOf(int64(42)), "42"},
		{"float64", reflect.ValueOf(3.5), "3.5"},
		{"bool_true", reflect.ValueOf(true), "true"},
		{"bool_false", reflect.ValueOf(false), "false"},
		{"other", reflect.ValueOf(struct{ X int }{X: 1}), "{1}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapKeyToString(tt.key); got != tt.want {
				t.Errorf("MapKeyToString() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// OpenFileExclusive (rotation.go)
// ----------------------------------------------------------------------------

func TestOpenFileExclusive(t *testing.T) {
	tmpDir := t.TempDir()
	fresh := filepath.Join(tmpDir, "fresh.log")

	// Fresh path: exclusive create succeeds, reports a new (size-0) file.
	file, size, err := OpenFileExclusive(fresh, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
	if err != nil {
		t.Fatalf("OpenFileExclusive(fresh) error = %v", err)
	}
	if size != 0 {
		t.Errorf("fresh file size = %d, want 0", size)
	}
	if _, err := file.Write([]byte("payload")); err != nil {
		t.Fatalf("write error = %v", err)
	}
	file.Close()

	// Pre-existing file: O_EXCL fails, falls back to OpenFile, which reports
	// the existing size.
	file2, size2, err := OpenFileExclusive(fresh, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
	if err != nil {
		t.Fatalf("OpenFileExclusive(existing) error = %v", err)
	}
	file2.Close()
	if size2 != 7 {
		t.Errorf("existing file size = %d, want 7", size2)
	}
}

func TestOpenFileExclusiveRejectsHardlink(t *testing.T) {
	tmpDir := t.TempDir()
	original := filepath.Join(tmpDir, "original.log")
	hardlink := filepath.Join(tmpDir, "hardlink.log")

	if err := os.WriteFile(original, []byte("x"), 0o644); err != nil {
		t.Fatalf("create original: %v", err)
	}
	if err := os.Link(original, hardlink); err != nil {
		t.Skipf("hard links not supported on this system: %v", err)
	}

	// Exclusive create fails (link target exists) → falls back to OpenFile →
	// hardlink detected → hardlinkErr returned.
	file, _, err := OpenFileExclusive(hardlink, testErrSymlinkNotAllowed, testErrHardlinkNotAllowed)
	if file != nil {
		file.Close()
	}
	if err == nil {
		t.Fatal("OpenFileExclusive should reject a hardlinked file")
	}
	if !errors.Is(err, testErrHardlinkNotAllowed) {
		t.Errorf("OpenFileExclusive hardlink error = %v, want %v", err, testErrHardlinkNotAllowed)
	}
}
