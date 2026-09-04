package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// This file targets error paths and boundary conditions the main suites
// leave unexercised: backup-cleanup filtering edges, and the nil/empty and
// non-letter edges of small helpers.

func TestCleanupExcessBackupsBoundaries(t *testing.T) {
	cases := []struct {
		name       string
		maxBackups int
		compress   bool
		backups    []int    // indexes to create as regular backup files
		extraDirs  []int    // backup-named directories (must be skipped)
		extraFiles []string // malformed/unrelated names (must survive)
		wantKept   []int    // backup indexes that must survive
	}{
		{
			name:       "fewer than limit removes nothing",
			maxBackups: 3,
			backups:    []int{1, 2},
			wantKept:   []int{1, 2},
		},
		{
			name:       "oldest indexes beyond the limit are removed",
			maxBackups: 2,
			backups:    []int{1, 2, 3, 4},
			wantKept:   []int{3, 4},
		},
		{
			name:       "directories and malformed names are not backups",
			maxBackups: 1,
			backups:    []int{1, 2},
			extraDirs:  []int{9},
			extraFiles: []string{"app_log_abc.log", "unrelated.log"},
			wantKept:   []int{2},
		},
		{
			name:       "compressed naming scheme",
			maxBackups: 2,
			compress:   true,
			backups:    []int{1, 2, 3},
			wantKept:   []int{2, 3},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			base := filepath.Join(dir, "app.log")

			for _, i := range tc.backups {
				p := GetBackupPath(base, i, tc.compress)
				if err := os.WriteFile(p, []byte("backup"), 0o600); err != nil {
					t.Fatalf("create backup %d: %v", i, err)
				}
			}
			for _, i := range tc.extraDirs {
				if err := os.Mkdir(GetBackupPath(base, i, false), 0o750); err != nil {
					t.Fatalf("create backup dir %d: %v", i, err)
				}
			}
			for _, name := range tc.extraFiles {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("keep"), 0o600); err != nil {
					t.Fatalf("create %s: %v", name, err)
				}
			}

			cleanupExcessBackups(base, tc.maxBackups, tc.compress)

			kept := make(map[int]bool, len(tc.wantKept))
			for _, i := range tc.wantKept {
				kept[i] = true
				if _, err := os.Stat(GetBackupPath(base, i, tc.compress)); err != nil {
					t.Errorf("backup %d should survive, stat: %v", i, err)
				}
			}
			for _, i := range tc.backups {
				if kept[i] {
					continue
				}
				if _, err := os.Stat(GetBackupPath(base, i, tc.compress)); !os.IsNotExist(err) {
					t.Errorf("backup %d should be removed, stat err = %v", i, err)
				}
			}
			for _, i := range tc.extraDirs {
				if _, err := os.Stat(GetBackupPath(base, i, false)); err != nil {
					t.Errorf("backup-named dir %d should be skipped, stat: %v", i, err)
				}
			}
			for _, name := range tc.extraFiles {
				if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
					t.Errorf("%s should survive, stat: %v", name, err)
				}
			}
		})
	}

	t.Run("missing directory is a no-op", func(t *testing.T) {
		base := filepath.Join(t.TempDir(), "missing", "app.log")
		cleanupExcessBackups(base, 2, false) // must not panic
	})

	t.Run("zero limit removes all backups", func(t *testing.T) {
		// Standalone semantics: excess = len - 0, so every backup goes.
		// RotateBackups guards maxBackups > 0 before calling in; this pins
		// the behavior should a future caller skip that guard.
		dir := t.TempDir()
		base := filepath.Join(dir, "app.log")
		for _, i := range rangeInts(1, 2) {
			if err := os.WriteFile(GetBackupPath(base, i, false), []byte("b"), 0o600); err != nil {
				t.Fatalf("create backup %d: %v", i, err)
			}
		}

		cleanupExcessBackups(base, 0, false)

		for _, i := range rangeInts(1, 2) {
			if _, err := os.Stat(GetBackupPath(base, i, false)); !os.IsNotExist(err) {
				t.Errorf("backup %d should be removed, stat err = %v", i, err)
			}
		}
	})
}

// rangeInts returns lo..hi inclusive.
func rangeInts(lo, hi int) []int {
	out := make([]int, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		out = append(out, i)
	}
	return out
}

func TestUpperASCIIBoundaries(t *testing.T) {
	cases := []struct{ in, want byte }{
		{'a', 'A'}, {'z', 'Z'}, {'m', 'M'}, // lowercase converts
		{'A', 'A'}, {'Z', 'Z'}, // already uppercase
		{'`', '`'}, {'{', '{'}, // just outside 'a'..'z'
		{'0', '0'}, {' ', ' '}, // non-letters
		{0x00, 0x00}, {0xFF, 0xFF}, // non-ASCII bytes untouched
	}
	for _, tc := range cases {
		if got := upperASCII(tc.in); got != tc.want {
			t.Errorf("upperASCII(%#02x) = %#02x, want %#02x", tc.in, got, tc.want)
		}
	}
}

func TestZeroSliceBoundaries(t *testing.T) {
	t.Run("non-empty slice is wiped and truncated", func(t *testing.T) {
		b := []byte("secret")
		zeroSlice(&b)
		if len(b) != 0 {
			t.Errorf("len = %d, want 0", len(b))
		}
		for i := 0; i < cap(b); i++ {
			if b[:cap(b)][i] != 0 {
				t.Errorf("byte %d not zeroed", i)
			}
		}
	})

	t.Run("empty and nil slices are no-ops", func(t *testing.T) {
		empty := make([]byte, 0, 8)
		zeroSlice(&empty)
		if len(empty) != 0 || cap(empty) != 8 {
			t.Errorf("empty: len = %d, cap = %d, want 0/8", len(empty), cap(empty))
		}

		var nilSlice []byte
		zeroSlice(&nilSlice)
		if nilSlice != nil {
			t.Error("nil slice became non-nil")
		}
	})
}

func TestGetJSONFieldNamesFallback(t *testing.T) {
	f := &MessageFormatter{} // zero value: pre-cached names absent
	got := f.getJSONFieldNames()
	if got == nil {
		t.Fatal("getJSONFieldNames() = nil, want default names")
	}
	if want := DefaultJSONFieldNames(); *got != *want {
		t.Errorf("getJSONFieldNames() fallback = %+v, want %+v", *got, *want)
	}
}

func TestFormatterResolveCaller(t *testing.T) {
	f := NewMessageFormatter(&FormatterConfig{Format: LogFormatText})
	if got := f.ResolveCaller(2); got != "" {
		t.Errorf("ResolveCaller without dynamic caller = %q, want empty", got)
	}

	// Called from inside the module, the resolver walks past dd frames to
	// the first out-of-module frame — from a test that is testing.tRunner.
	fd := NewMessageFormatter(&FormatterConfig{Format: LogFormatText, DynamicCaller: true})
	if got := fd.ResolveCaller(2); !strings.Contains(got, "testing.go") {
		t.Errorf("ResolveCaller(dynamic) = %q, want the first non-dd frame (testing.go)", got)
	}
}

func TestFindUserFrameEmptyWindow(t *testing.T) {
	if off, found := findUserFrame(nil, 0); found || off != 0 {
		t.Errorf("findUserFrame(empty) = (%d, %v), want (0, false)", off, found)
	}
}

// stringerSlice is a slice with a String() method: IsComplexValue must treat
// it as simple despite the slice kind.
type stringerSlice []int

func (s stringerSlice) String() string { return fmt.Sprint(len(s), " items") }

func TestIsComplexValueNilBoundaries(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want bool
	}{
		{"typed nil pointer to Stringer", (*fmt.Stringer)(nil), false},
		{"nil Stringer interface", fmt.Stringer(nil), false},
		{"nil typed pointer to slice", (*[]int)(nil), false},
		{"slice with String method stays simple", stringerSlice{1}, false},
		{"plain slice is complex", []int{1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsComplexValue(tc.in); got != tc.want {
				t.Errorf("IsComplexValue(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestConvertValueNilBoundaries(t *testing.T) {
	t.Run("nil typed pointer converts to nil", func(t *testing.T) {
		var p *int
		if got := ConvertValue(p); got != nil {
			t.Errorf("ConvertValue(nil pointer) = %v, want nil", got)
		}
	})

	t.Run("nil interface field inside struct", func(t *testing.T) {
		type errHolder struct {
			Err error `json:"err"`
		}
		got, ok := ConvertValue(errHolder{}).(map[string]any)
		if !ok {
			t.Fatalf("ConvertValue(struct) = %T, want map[string]any", got)
		}
		if v, exists := got["err"]; !exists || v != nil {
			t.Errorf("struct nil-error field = %v (exists=%v), want nil", v, exists)
		}
	})

	t.Run("non-nil any holding nil interface", func(t *testing.T) {
		// A non-nil any whose dynamic value is a nil error exercises the
		// reflect.Interface nil branch.
		var nilErr error
		var i any = nilErr
		if got := ConvertValue(i); got != nil {
			t.Errorf("ConvertValue(nil-in-any) = %v, want nil", got)
		}
	})
}

func TestConvertStructWithDepthNilErrorField(t *testing.T) {
	type errHolder struct {
		Err error `json:"err"`
	}
	got, ok := convertStructWithDepth(reflect.ValueOf(errHolder{}), 0).(map[string]any)
	if !ok {
		t.Fatalf("convertStructWithDepth() = %T, want map[string]any", got)
	}
	if v, exists := got["err"]; !exists || v != nil {
		t.Errorf("nil error field = %v (exists=%v), want nil", v, exists)
	}
}
