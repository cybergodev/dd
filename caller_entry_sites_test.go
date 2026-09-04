package dd_test

// This file deliberately lives in the EXTERNAL test package (dd_test, not dd):
// its functions' runtime names carry a package path the logger's dynamic
// caller detection does NOT classify as its own frames, so the tests exercise
// the same "first user frame" resolution real callers get. From the in-package
// tests, every frame looks like library code to the detector and is skipped.

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cybergodev/dd"
)

// TestEntrySiteCallerAccuracy exercises every logging entry method with
// DynamicCaller enabled and asserts the reported caller is the EXACT line of
// the logging call in this test file. The entry-site capture resolves the
// caller from each entry method's own stack position (see
// internal.ResolveEntryCaller); a wrong anchor, offset, or hint slot would
// report a different line or a dd-internal file, failing here.
func TestEntrySiteCallerAccuracy(t *testing.T) {
	var buf bytes.Buffer
	cfg := dd.DefaultConfig()
	cfg.Targets = []dd.OutputTarget{dd.CustomOutput(&buf)}
	cfg.Level = dd.LevelDebug
	cfg.DynamicCaller = true
	cfg.FullPath = false
	cfg.Security = &dd.SecurityConfig{SensitiveFilter: nil}
	logger, err := dd.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer logger.Close()

	entry := logger.WithFields(dd.String("preset", "v"))

	// Route the package-level convenience functions (dd.Info, dd.Printf, ...)
	// to the test logger so their caller capture can be asserted. The previous
	// default is restored afterwards so this does not leak into other tests.
	prevDefault := dd.Default()
	dd.SetDefault(logger)
	t.Cleanup(func() { dd.SetDefault(prevDefault) })

	// want is set by mark to the file:line of the closure line that calls it;
	// each case's log call must sit on that same line (single-line closures),
	// since the caller reported is the line of the actual logging call.
	var want string
	mark := func() {
		_, file, line, _ := runtime.Caller(1)
		want = fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}

	cases := []struct {
		name string
		log  func()
	}{
		{"Log", func() { mark(); logger.Log(dd.LevelInfo, "m") }},
		{"Logf", func() { mark(); logger.Logf(dd.LevelInfo, "m") }},
		{"LogWith", func() { mark(); logger.LogWith(dd.LevelInfo, "m", dd.String("k", "v")) }},
		{"Info", func() { mark(); logger.Info("m") }},
		{"Infof", func() { mark(); logger.Infof("m") }},
		{"InfoWith", func() { mark(); logger.InfoWith("m", dd.String("k", "v")) }},
		{"Print", func() { mark(); logger.Print("m") }},
		{"Println", func() { mark(); logger.Println("m") }},
		{"Printf", func() { mark(); logger.Printf("m") }},
		{"EntryLog", func() { mark(); entry.Log(dd.LevelInfo, "m") }},
		{"EntryLogf", func() { mark(); entry.Logf(dd.LevelInfo, "m") }},
		{"EntryLogWith", func() { mark(); entry.LogWith(dd.LevelInfo, "m", dd.String("k", "v")) }},
		{"EntryInfo", func() { mark(); entry.Info("m") }},
		{"EntryInfof", func() { mark(); entry.Infof("m") }},
		{"EntryInfoWith", func() { mark(); entry.InfoWith("m", dd.String("k", "v")) }},

		// Package-level convenience functions. These delegate to the default
		// logger (swapped to the test logger above); until they called the
		// dispatch funnels directly (instead of the (*Logger).Log/Logf/LogWith/
		// Print entry methods), the extra wrapper frame made the capture report
		// the dd wrapper itself as the caller (e.g. "logger.go:NNN") — pinned
		// here so it cannot regress.
		{"PkgLog", func() { mark(); dd.Log(dd.LevelInfo, "m") }},
		{"PkgLogf", func() { mark(); dd.Logf(dd.LevelInfo, "m") }},
		{"PkgLogWith", func() { mark(); dd.LogWith(dd.LevelInfo, "m", dd.String("k", "v")) }},
		{"PkgInfo", func() { mark(); dd.Info("m") }},
		{"PkgInfof", func() { mark(); dd.Infof("m") }},
		{"PkgInfoWith", func() { mark(); dd.InfoWith("m", dd.String("k", "v")) }},
		{"PkgWarn", func() { mark(); dd.Warn("m") }},
		{"PkgWarnf", func() { mark(); dd.Warnf("m") }},
		{"PkgError", func() { mark(); dd.Error("m") }},
		{"PkgErrorf", func() { mark(); dd.Errorf("m") }},
		{"PkgErrorWith", func() { mark(); dd.ErrorWith("m", dd.String("k", "v")) }},
		{"PkgDebug", func() { mark(); dd.Debug("m") }},
		{"PkgDebugf", func() { mark(); dd.Debugf("m") }},
		{"PkgDebugWith", func() { mark(); dd.DebugWith("m", dd.String("k", "v")) }},
		{"PkgPrint", func() { mark(); dd.Print("m") }},
		{"PkgPrintln", func() { mark(); dd.Println("m") }},
		{"PkgPrintf", func() { mark(); dd.Printf("m") }},
		{"PkgEntryInfo", func() { mark(); dd.WithFields(dd.String("pk", "pv")).Info("m") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			tc.log()

			out := buf.String()
			if !strings.Contains(out, want) {
				t.Errorf("caller = %q, want it to contain %q (full output: %q)",
					firstCallerField(out), want, out)
			}
			// The capture must never leak a dd-internal file as the caller.
			for _, internal := range []string{"logger.go", "entry.go", "structured.go", "debug_visual.go", "formatting.go", "caller.go"} {
				if strings.Contains(out, internal) {
					t.Errorf("output reports dd-internal frame %q as caller: %q", internal, out)
				}
			}
		})
	}
}

// firstCallerField extracts a file:line token from a log line for error
// messages; returns the whole (trimmed) line when no token is found.
func firstCallerField(line string) string {
	fields := strings.Fields(line)
	for _, f := range fields {
		if strings.HasSuffix(f, ".go") || strings.Contains(f, ".go:") {
			return f
		}
	}
	return strings.TrimSpace(line)
}
