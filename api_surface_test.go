package dd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// ============================================================================
// D-005 Phase 2: package-level API surface freeze
//
// The two-layer mirror policy (doc.go "Two API Layers", MIGRATION.md) froze
// the package-level function set: logging, level checks, writer management,
// sampling, and lifecycle are mirrored; runtime configuration is
// instance-only. This test pins the exported package-level function set so
// that adding or removing one fails CI with a pointer to the policy instead
// of drifting silently.
//
// To change this list intentionally: update the policy in doc.go and
// MIGRATION.md first, then edit goldenPackageLevelFuncs in the same commit.
// ============================================================================

// goldenPackageLevelFuncs is the frozen set of exported package-level
// functions (112 as of 2026-10-05), grouped by policy role.
var goldenPackageLevelFuncs = []string{
	// Logging (mirrored from *Logger)
	"Debug", "Debugf", "DebugWith",
	"Info", "Infof", "InfoWith",
	"Warn", "Warnf", "WarnWith",
	"Error", "Errorf", "ErrorWith",
	"Fatal", "Fatalf", "FatalWith",
	"Log", "Logf", "LogWith",
	"Print", "Printf", "Println",

	// Level checks and management (mirrored)
	"IsDebugEnabled", "IsErrorEnabled", "IsFatalEnabled", "IsInfoEnabled",
	"IsLevelEnabled", "IsWarnEnabled", "SetLevel", "GetLevel",

	// Global default-logger management
	"Default", "DefaultWithErr", "DefaultInitError", "InitDefault", "SetDefault",

	// Entry chaining (mirrored)
	"WithField", "WithFields",

	// Writer management (mirrored)
	"AddWriter", "RemoveWriter", "WriterCount",

	// Sampling (mirrored)
	"SetSampling", "GetSampling",

	// Lifecycle (mirrored)
	"Flush", "Close", "Shutdown",

	// Debug utilities (package-level only, no filter — see debug_visual.go)
	"Text", "Textf", "JSON", "JSONF", "Exit", "Exitf",

	// Field constructors
	"Any", "Bool", "Duration", "Err", "ErrWithKey", "ErrWithStack",
	"Float32", "Float64",
	"Int", "Int8", "Int16", "Int32", "Int64",
	"String", "Time",
	"Uint", "Uint8", "Uint16", "Uint32", "Uint64",

	// Context storage helpers
	"WithTraceID", "WithSpanID", "WithRequestID",
	"GetTraceID", "GetSpanID", "GetRequestID",

	// Config presets and Default*Config constructors
	"DefaultConfig", "DevelopmentConfig", "JSONConfig", "ProductionConfig",
	"DefaultSamplingConfig", "DefaultJSONOptions",
	"DefaultSecurityConfig", "DefaultSecureConfig", // Deprecated; remove in v2
	"DefaultRateLimitConfig", "DefaultFieldValidationConfig",
	"StrictSnakeCaseConfig", "StrictCamelCaseConfig",
	"DefaultAuditConfig", "DefaultFileWriterConfig",
	"DefaultBufferedWriterConfig", "DefaultIntegrityConfigSafe",

	// Security presets
	"SecurityConfigForLevel", "HealthcareConfig", "FinancialConfig", "GovernmentConfig",

	// Output target helpers
	"ConsoleOutput", "FileOutput", "CustomOutput",

	// Constructors
	"New", "NewAuditLogger", "NewBufferedWriter", "NewCustomSensitiveDataFilter",
	"NewEmptySensitiveDataFilter", "NewFileWriter", "NewHookRegistry",
	"NewHooksFromConfig", "NewIntegritySigner", "NewLoggerRecorder",
	"NewMultiWriter", "NewSensitiveDataFilter",

	// Verification
	"VerifyAuditEvent",
}

// TestPackageLevelAPISurface compares the exported package-level function
// set against goldenPackageLevelFuncs and reports additions/removals with a
// reference to the mirror policy.
func TestPackageLevelAPISurface(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source directory")
	}
	pkgDir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	fset := token.NewFileSet()
	got := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(pkgDir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || !fd.Name.IsExported() {
				continue
			}
			got[fd.Name.Name] = true
		}
	}

	var added, removed []string
	for name := range got {
		if !slices.Contains(goldenPackageLevelFuncs, name) {
			added = append(added, name)
		}
	}
	for _, name := range goldenPackageLevelFuncs {
		if !got[name] {
			removed = append(removed, name)
		}
	}

	// Self-check: a duplicated golden entry is invisible to the set diff
	// below (it is neither added nor removed), so guard it explicitly.
	if len(goldenPackageLevelFuncs) != len(got) && len(added) == 0 && len(removed) == 0 {
		t.Errorf("golden list has %d entries but %d unique functions exist — check for duplicates in goldenPackageLevelFuncs",
			len(goldenPackageLevelFuncs), len(got))
	}

	if len(added) > 0 || len(removed) > 0 {
		t.Errorf(`package-level API surface drifted from the frozen snapshot (see doc.go "Two API Layers" and MIGRATION.md):
  added:   %v
  removed: %v
The mirror policy froze this set: new capabilities go on the instance layer
(*Logger methods), not the package layer. To change the set intentionally,
update the policy docs and goldenPackageLevelFuncs in the same commit.`,
			added, removed)
	}
}
