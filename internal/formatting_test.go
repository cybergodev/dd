package internal

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewMessageFormatter(t *testing.T) {
	tests := []struct {
		name   string
		config *FormatterConfig
	}{
		{
			name: "default config",
			config: &FormatterConfig{
				Format:        LogFormatText,
				TimeFormat:    time.RFC3339,
				IncludeTime:   true,
				IncludeLevel:  true,
				FullPath:      false,
				DynamicCaller: true,
			},
		},
		{
			name: "json format",
			config: &FormatterConfig{
				Format:       LogFormatJSON,
				TimeFormat:   time.RFC3339,
				IncludeTime:  true,
				IncludeLevel: true,
				FullPath:     true,
				JSON: &JSONOptions{
					PrettyPrint: true,
					Indent:      "  ",
				},
			},
		},
		{
			name: "custom json field names",
			config: &FormatterConfig{
				Format:     LogFormatJSON,
				TimeFormat: time.RFC3339,
				JSON: &JSONOptions{
					FieldNames: &JSONFieldNames{
						Timestamp: "ts",
						Level:     "lvl",
						Caller:    "src",
						Message:   "msg",
						Fields:    "data",
					},
				},
			},
		},
		{
			name: "nil json config",
			config: &FormatterConfig{
				Format:     LogFormatText,
				TimeFormat: time.RFC3339,
				JSON:       nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewMessageFormatter(tt.config)
			if formatter == nil {
				t.Fatal("NewMessageFormatter returned nil")
			}
			if formatter.timeCache == nil {
				t.Error("timeCache should be initialized")
			}
		})
	}
}

func TestFormatArgsToString(t *testing.T) {
	formatter := NewMessageFormatter(&FormatterConfig{
		Format:     LogFormatText,
		TimeFormat: time.RFC3339,
	})

	tests := []struct {
		name     string
		args     []any
		expected string
	}{
		{"empty", []any{}, ""},
		{"single string", []any{"hello"}, "hello"},
		{"single int", []any{42}, "42"},
		{"single int64", []any{int64(123456789)}, "123456789"},
		{"single int32", []any{int32(1000)}, "1000"},
		{"single int16", []any{int16(100)}, "100"},
		{"single int8", []any{int8(10)}, "10"},
		{"single uint", []any{uint(42)}, "42"},
		{"single uint64", []any{uint64(123456789)}, "123456789"},
		{"single uint32", []any{uint32(1000)}, "1000"},
		{"single uint16", []any{uint16(100)}, "100"},
		{"single uint8", []any{uint8(10)}, "10"},
		{"single float64", []any{3.14}, "3.14"},
		{"single float32", []any{float32(2.5)}, "2.5"},
		{"single bool true", []any{true}, "true"},
		{"single bool false", []any{false}, "false"},
		{"single duration", []any{5 * time.Second}, "5s"},
		{"single time", []any{time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)}, "2024-01-01T12:00:00Z"},
		{"single error", []any{errorStub("test error")}, "test error"},
		{"single nil", []any{nil}, "<nil>"},
		{"multiple args", []any{"hello", 42, true}, "hello 42 true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatArgsToString(tt.args...)
			if result != tt.expected {
				t.Errorf("FormatArgsToString() = %q, want %q", result, tt.expected)
			}
		})
	}
}

type errorStub string

func (e errorStub) Error() string { return string(e) }

type stringerStub string

func (s stringerStub) String() string { return string(s) }

func TestFormatArgsToStringComplexTypes(t *testing.T) {
	formatter := NewMessageFormatter(&FormatterConfig{
		Format:     LogFormatText,
		TimeFormat: time.RFC3339,
	})

	// Test stringer
	result := formatter.FormatArgsToString(stringerStub("stringer value"))
	if result != "stringer value" {
		t.Errorf("Stringer not handled: got %q", result)
	}

	// Test slice (complex type)
	result = formatter.FormatArgsToString([]int{1, 2, 3})
	if !strings.Contains(result, "1") {
		t.Errorf("Slice not formatted properly: got %q", result)
	}

	// Test map (complex type): rendered as JSON, not fmt's map[a:1]
	result = formatter.FormatArgsToString(map[string]int{"a": 1})
	if !strings.Contains(result, `{"a":1}`) {
		t.Errorf("Map not formatted properly: got %q", result)
	}
}

func TestFormatWithMessage(t *testing.T) {
	tests := []struct {
		name             string
		config           *FormatterConfig
		level            LogLevel
		message          string
		fields           []Field
		wantContains     []string
		dontWantContains []string
	}{
		{
			name: "text format with all options",
			config: &FormatterConfig{
				Format:        LogFormatText,
				TimeFormat:    time.RFC3339,
				IncludeTime:   true,
				IncludeLevel:  true,
				FullPath:      false,
				DynamicCaller: false,
			},
			level:        LevelInfo,
			message:      "test message",
			fields:       []Field{{Key: "key", Value: "value"}},
			wantContains: []string{"INFO", "test message", "key=value"},
		},
		{
			name: "text format no time",
			config: &FormatterConfig{
				Format:        LogFormatText,
				TimeFormat:    time.RFC3339,
				IncludeTime:   false,
				IncludeLevel:  true,
				DynamicCaller: false,
			},
			level:        LevelDebug,
			message:      "debug msg",
			wantContains: []string{"DEBUG", "debug msg"},
		},
		{
			name: "text format no level",
			config: &FormatterConfig{
				Format:        LogFormatText,
				TimeFormat:    time.RFC3339,
				IncludeTime:   true,
				IncludeLevel:  false,
				DynamicCaller: false,
			},
			level:            LevelWarn,
			message:          "warning msg",
			dontWantContains: []string{"WARN"},
		},
		{
			name: "json format",
			config: &FormatterConfig{
				Format:        LogFormatJSON,
				TimeFormat:    time.RFC3339,
				IncludeTime:   true,
				IncludeLevel:  true,
				DynamicCaller: false,
			},
			level:        LevelError,
			message:      "error message",
			fields:       []Field{{Key: "error_code", Value: 500}},
			wantContains: []string{`"level":"ERROR"`, `"message":"error message"`, `"error_code"`},
		},
		{
			name: "json format with custom field names",
			config: &FormatterConfig{
				Format:        LogFormatJSON,
				TimeFormat:    time.RFC3339,
				IncludeTime:   true,
				IncludeLevel:  true,
				DynamicCaller: false,
				JSON: &JSONOptions{
					FieldNames: &JSONFieldNames{
						Timestamp: "ts",
						Level:     "lvl",
						Message:   "msg",
						Fields:    "data",
					},
				},
			},
			level:        LevelInfo,
			message:      "custom fields",
			wantContains: []string{`"lvl":"INFO"`, `"msg":"custom fields"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewMessageFormatter(tt.config)
			result := formatter.FormatWithMessage(tt.level, 10, tt.message, tt.fields)

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("Result should contain %q, got: %s", want, result)
				}
			}

			for _, dontWant := range tt.dontWantContains {
				if strings.Contains(result, dontWant) {
					t.Errorf("Result should NOT contain %q, got: %s", dontWant, result)
				}
			}
		})
	}
}

func TestFormatWithMessageDynamicCaller(t *testing.T) {
	// Test dynamic caller detection
	formatter := NewMessageFormatter(&FormatterConfig{
		Format:        LogFormatText,
		TimeFormat:    time.RFC3339,
		IncludeTime:   false,
		IncludeLevel:  false,
		FullPath:      false,
		DynamicCaller: true,
	})

	result := formatter.FormatWithMessage(LevelInfo, 2, "test", nil)

	// Should contain caller info with file and line number
	if !strings.Contains(result, ":") {
		t.Errorf("Dynamic caller should contain line number, got: %s", result)
	}
	// Should contain the message
	if !strings.Contains(result, "test") {
		t.Errorf("Result should contain message, got: %s", result)
	}
}

func TestTimeCache(t *testing.T) {
	tc := newTimeCache(time.RFC3339)

	// First call - should format time
	result1 := tc.getFormattedTime()
	if result1 == "" {
		t.Error("getFormattedTime should return non-empty string")
	}

	// Second call within same second - should return cached value
	result2 := tc.getFormattedTime()
	if result1 != result2 {
		// This might happen if we crossed a second boundary
		// Just verify both are valid timestamps
		if len(result2) < 10 {
			t.Errorf("Invalid timestamp: %s", result2)
		}
	}
}

// TestTimeCacheSubSecondFormat pins that the per-second cache does not freeze
// sub-second digits. timeCache keyed its single cache entry by whole Unix
// seconds, so with a fractional-second layout (DevelopmentConfig's
// "15:04:05.000", or any custom ".000"/".999999" format) every entry logged
// within one second reused the first call's milliseconds — stale timestamps.
func TestTimeCacheSubSecondFormat(t *testing.T) {
	layout := "15:04:05.000"
	tc := newTimeCache(layout)
	if !tc.subSecond {
		t.Fatalf("formatHasSubSecond(%q) = false, want true", layout)
	}

	first := tc.getFormattedTime()
	time.Sleep(6 * time.Millisecond)
	second := tc.getFormattedTime()
	if first == second {
		t.Errorf("sub-second timestamp frozen within one second: first=%s second=%s", first, second)
	}
	if _, err := time.Parse(layout, second); err != nil {
		t.Errorf("formatted time %q does not parse with layout %q: %v", second, layout, err)
	}
}

// TestFormatHasSubSecond checks layout classification for the cache bypass.
func TestFormatHasSubSecond(t *testing.T) {
	cases := []struct {
		layout string
		want   bool
	}{
		{"2006-01-02T15:04:05Z07:00", false},
		{"15:04:05", false},
		{"15:04:05.000", true},
		{"15:04:05.999", true},
		{"2006-01-02 15:04:05.000000", true},
		{"15:04:05,000", true},
		{"", false},
	}
	for _, tc := range cases {
		if got := formatHasSubSecond(tc.layout); got != tc.want {
			t.Errorf("formatHasSubSecond(%q) = %v, want %v", tc.layout, got, tc.want)
		}
	}
}

func TestResolveDynamicCaller(t *testing.T) {
	formatter := NewMessageFormatter(&FormatterConfig{
		Format:        LogFormatText,
		TimeFormat:    time.RFC3339,
		DynamicCaller: true,
	})

	// resolveDynamicCaller must return a formatted "file:line" caller string for
	// every depth, including the negative-depth case (normalized to 0). It must
	// never return an empty value or panic.
	for _, depth := range []int{-1, 0, 5, 10} {
		result := formatter.resolveDynamicCaller(depth)
		if result == "" {
			t.Errorf("resolveDynamicCaller(%d) returned empty string", depth)
		}
		if !strings.Contains(result, ":") {
			t.Errorf("resolveDynamicCaller(%d) = %q, expected a \"file:line\" string", depth, result)
		}
	}

	// Repeated calls exercise the memoized offset cache (hot path) and must stay
	// consistent with the first resolution.
	first := formatter.resolveDynamicCaller(0)
	for i := 0; i < 20; i++ {
		if got := formatter.resolveDynamicCaller(0); got != first {
			t.Fatalf("resolveDynamicCaller returned inconsistent result after warming cache: %q vs %q", first, got)
		}
	}
}

func TestIsDDFunction(t *testing.T) {
	prefix := getDDPackagePrefix()
	pkgLen := len(prefix)

	cases := []struct {
		name string
		want bool
	}{
		// dd-internal helper
		{"github.com/cybergodev/dd/internal.resolveDynamicCaller", true},
		// dd root package method (e.g. (*Logger).Info)
		{"github.com/cybergodev/dd.(*Logger).Info", true},
		// dd subpackage
		{"github.com/cybergodev/dd/internal/caller.callerForPC", true},
		// External packages and user code are NOT dd frames
		{"github.com/someuser/theirapp.run", false},
		{"main.serve", false},
		{"example.com/foo/bar.Baz", false},
		// A name that merely shares the prefix as a substring but is a different module
		{"github.com/cybergodev/dd-fork/internal.x", false},
	}

	for _, tc := range cases {
		got := isDDFunction(tc.name, prefix, pkgLen)
		if got != tc.want {
			t.Errorf("isDDFunction(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestFormatPooledBuffers drives both format paths through many cycles so
// pooled buffers are reused and corrupted state (stale bytes, missed resets)
// would surface as a wrong message.
func TestFormatPooledBuffers(t *testing.T) {
	tests := []struct {
		name      string
		format    LogFormat
		wantFrag  string
		withField bool
	}{
		{"text", LogFormatText, "test message", false},
		{"json", LogFormatJSON, `"message":"test message"`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewMessageFormatter(&FormatterConfig{
				Format:       tt.format,
				TimeFormat:   time.RFC3339,
				IncludeTime:  true,
				IncludeLevel: true,
			})

			for i := 0; i < 100; i++ {
				var fields []Field
				if tt.withField {
					fields = []Field{
						{Key: "iteration", Value: i},
						{Key: "data", Value: "test"},
					}
				}
				result := formatter.FormatWithMessage(LevelInfo, 10, "test message", fields)
				if !strings.Contains(result, tt.wantFrag) {
					t.Errorf("iteration %d: result should contain %q, got %q", i, tt.wantFrag, result)
				}
			}
		})
	}
}

// TestIsSimpleJSONValue classifies values for the JSON fast path: scalars and
// homogeneous string/int slices are simple; structs, maps, and []any are not.
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

// TestFormatJSONSlowPath forces the map-based JSON path (formatJSON):
// PrettyPrint disables the fast path and a complex field value is not a
// "simple" JSON value.
func TestFormatJSONSlowPath(t *testing.T) {
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

// TestDDPackagePrefix verifies the runtime-detected module prefix actually
// prefixes this package's own frames. The assertion is fork-safe: it derives
// the expectation from the runtime rather than a hardcoded import path.
func TestDDPackagePrefix(t *testing.T) {
	prefix := DDPackagePrefix()
	if prefix == "" {
		t.Fatal("DDPackagePrefix() is empty")
	}

	pc, _, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fn := runtime.FuncForPC(pc).Name() // <prefix>/internal.TestDDPackagePrefix
	if !strings.HasPrefix(fn, prefix+"/internal.") {
		t.Errorf("DDPackagePrefix() = %q does not prefix this package's frame %q", prefix, fn)
	}
}

func TestGetJSONFieldNames(t *testing.T) {
	// Test with custom field names
	customNames := &JSONFieldNames{
		Timestamp: "ts",
		Level:     "lvl",
		Caller:    "src",
		Message:   "msg",
		Fields:    "data",
	}
	formatter := NewMessageFormatter(&FormatterConfig{
		Format: LogFormatJSON,
		JSON: &JSONOptions{
			FieldNames: customNames,
		},
	})

	names := formatter.getJSONFieldNames()
	if names.Timestamp != "ts" {
		t.Errorf("Expected custom timestamp field name, got %s", names.Timestamp)
	}

	// Test with nil config
	formatter2 := NewMessageFormatter(&FormatterConfig{
		Format: LogFormatText,
		JSON:   nil,
	})
	names2 := formatter2.getJSONFieldNames()
	if names2.Timestamp != "timestamp" {
		t.Errorf("Expected default timestamp field name, got %s", names2.Timestamp)
	}
}

func TestGetJSONOptions(t *testing.T) {
	// Test with custom options
	customOpts := &JSONOptions{
		PrettyPrint: true,
		Indent:      "    ",
	}
	formatter := NewMessageFormatter(&FormatterConfig{
		Format: LogFormatJSON,
		JSON:   customOpts,
	})

	opts := formatter.getJSONOptions()
	if !opts.PrettyPrint {
		t.Error("Expected PrettyPrint to be true")
	}
	if opts.Indent != "    " {
		t.Errorf("Expected indent '    ', got %q", opts.Indent)
	}
}
