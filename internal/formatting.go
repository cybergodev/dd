package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Cached default JSON options to avoid repeated allocations
var defaultJSONOptions = &JSONOptions{
	PrettyPrint: false,
	Indent:      DefaultJSONIndent,
}

// ddPackagePrefix stores the dynamically detected package path prefix.
// This allows the logger to work correctly when the package is forked
// or the module path is changed.
var (
	ddPackagePrefix     string
	ddPackagePrefixOnce sync.Once
)

// getDDPackagePrefix returns the package path prefix for the dd package.
// It uses runtime.Caller(0) to get the fully qualified function name,
// then extracts the module path by finding the "/internal." boundary.
// This is more robust than file-path-based detection which could match
// unrelated directories named "dd".
func getDDPackagePrefix() string {
	ddPackagePrefixOnce.Do(func() {
		// runtime.Caller(0) returns the function name of the caller.
		// Example: github.com/cybergodev/dd/internal.getDDPackagePrefix
		// We want: github.com/cybergodev/dd
		if pc, _, _, ok := runtime.Caller(0); ok {
			fn := runtime.FuncForPC(pc).Name()
			// Find the "/internal." boundary to extract the module root.
			// The function name contains the full import path before the ".func" part.
			if idx := strings.LastIndex(fn, "/internal."); idx > 0 {
				ddPackagePrefix = fn[:idx]
				return
			}
			// Fallback: try to strip ".func" suffix for the dd package itself
			if idx := strings.LastIndex(fn, "/dd."); idx > 0 {
				ddPackagePrefix = fn[:idx+3]
				return
			}
		}
		// Fallback to known prefix if detection fails
		ddPackagePrefix = "github.com/cybergodev/dd"
	})
	return ddPackagePrefix
}

// DDPackagePrefix returns this module's package path prefix (for example
// "github.com/cybergodev/dd"), detected at runtime so callers that must skip
// their own stack frames or files keep working when the module is forked or
// the module path changes. Prefer this over hardcoding the import path.
func DDPackagePrefix() string {
	return getDDPackagePrefix()
}

// lineBufferPool pools the bytes.Buffer objects that carry one fully
// formatted log line from the formatter to the writers. The formatter fills a
// pooled buffer and hands ownership to the caller (FormatWithMessageBytes),
// so the write path emits the bytes directly — the previous design returned a
// string per line (bytes.Buffer.String) and writeMessage then copied that
// string into a second pooled buffer for the trailing newline; pprof showed
// the String copy alone was ~65% of allocation volume on the logging hot path.
// Initial capacity of 2048 bytes covers most common log messages:
// base (~80) + timestamp (~35) + caller (~50) + message (~500) + 10 fields (~400) + safety margin
// SECURITY: Uses bytes.Buffer instead of strings.Builder to allow proper
// zeroing of sensitive data before returning to pool (see PutLineBuffer).
var lineBufferPool = sync.Pool{
	New: func() any {
		buf := &bytes.Buffer{}
		buf.Grow(2048) // optimized for typical log entries, reduced grow overhead
		return buf
	},
}

// maxLineBufferCap bounds the capacity of buffers kept in lineBufferPool.
// Buffers that had to grow beyond this (very large log lines) are dropped on
// return and left for GC, limiting sensitive-data retention in pooled memory
// (same policy the former per-format pools used).
const maxLineBufferCap = 4 * 1024

// GetLineBuffer returns an empty buffer from the line pool for formatting one
// log line. Pair every Get with exactly one PutLineBuffer.
func GetLineBuffer() *bytes.Buffer {
	buf := lineBufferPool.Get().(*bytes.Buffer)
	buf.Reset() // defensive: PutLineBuffer already resets, this guards misuse
	return buf
}

// PutLineBuffer zeroes a line buffer's contents and returns it to the pool.
// Ownership of a buffer returned by FormatWithMessageBytes ends here; callers
// must pass every such buffer to PutLineBuffer exactly once, on every path.
func PutLineBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	if buf.Cap() > maxLineBufferCap {
		return // oversized: let GC reclaim it rather than retain pooled memory
	}
	// SECURITY: Zero the buffer contents before returning to pool
	zeroBuffer(buf)
	lineBufferPool.Put(buf)
}

// argsBuilderPool pools bytes.Buffer objects for argument concatenation
// to reduce memory allocations when formatting multiple arguments.
// Increased from 256 to 512 to reduce grow() overhead in hot path
// SECURITY: Uses bytes.Buffer instead of strings.Builder to allow proper
// zeroing of sensitive data before returning to pool.
var argsBuilderPool = sync.Pool{
	New: func() any {
		buf := &bytes.Buffer{}
		buf.Grow(512) // optimized for typical args concatenation
		return buf
	},
}

// paddedLevelStrings caches formatted level strings with leading spaces for alignment.
// Pre-computed to avoid repeated string formatting in the hot path.
// Format: " DEBUG", "  INFO", "  WARN", " ERROR", " FATAL" (6 chars each)
var paddedLevelStrings = [5]string{
	" DEBUG", // LevelDebug = 0
	"  INFO", // LevelInfo = 1
	"  WARN", // LevelWarn = 2
	" ERROR", // LevelError = 3
	" FATAL", // LevelFatal = 4
}

// CallerHint memoizes, per capture anchor, the index of the first user
// (non-dd) frame in a captured stack. The anchor (the first recorded frame
// below user code) and the offset are constant for a given build and entry
// site, so a hint stays valid for the logger's lifetime.
//
// Knowing the offset up front lets the steady-state path capture only
// offset+1 stack frames — runtime.Callers stops when the buffer is full, and
// unwinding frames scales the pcvalue/findfunc work linearly with frames
// walked, which pprof showed was the dominant CPU cost of every log call
// (76% of SimpleLogging when the capture sat deep in the formatter).
type CallerHint struct {
	AnchorPC uintptr
	Offset   int
}

// jsonEntryMapPool pools map[string]any objects for JSON formatting
// to reduce memory allocations during high-frequency JSON logging.
var jsonEntryMapPool = sync.Pool{
	New: func() any {
		m := make(map[string]any, 10) // increased from 8 for typical log entries
		return &m
	},
}

// jsonFieldsMapPool pools map[string]any objects for JSON fields
// to reduce memory allocations when logging with structured fields.
var jsonFieldsMapPool = sync.Pool{
	New: func() any {
		m := make(map[string]any, 6) // increased from 4 for typical field counts
		return &m
	},
}

// cachedTimeEntry stores a single cached timestamp entry
type cachedTimeEntry struct {
	sec       int64  // Unix timestamp in seconds
	formatted string // Cached formatted timestamp
}

// timeCache stores cached formatted timestamp for high-frequency logging.
// Uses atomic pointer for lock-free reads with better cache locality.
// Caches the formatted string within the same second to reduce time formatting overhead.
type timeCache struct {
	current    atomic.Pointer[cachedTimeEntry] // Atomic pointer to current cache entry
	timeFormat string                          // Time format string (immutable after creation)
	// subSecond is true when the format renders sub-second precision (e.g.
	// "15:04:05.000"). The cache is keyed by whole seconds, so it must be
	// bypassed for such formats — otherwise every entry logged within one
	// second reused the first call's fractional digits (stale timestamps).
	subSecond bool
}

// newTimeCache creates a new time cache with the given format
func newTimeCache(timeFormat string) *timeCache {
	tc := &timeCache{
		timeFormat: timeFormat,
		subSecond:  formatHasSubSecond(timeFormat),
	}
	// Initialize with zero entry to avoid nil checks
	tc.current.Store(&cachedTimeEntry{sec: -1, formatted: ""})
	return tc
}

// formatHasSubSecond reports whether layout renders sub-second precision, by
// comparing the rendering of two times that differ only below the second mark.
// Behavior-based detection stays correct for any fractional-second layout
// (".000", ".999999", ",000", ...) without parsing the layout grammar.
func formatHasSubSecond(layout string) bool {
	base := time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC)
	frac := time.Date(2006, 1, 2, 15, 4, 5, 500000000, time.UTC)
	return base.Format(layout) != frac.Format(layout)
}

// getFormattedTime returns the formatted current time.
// Uses lock-free atomic operations for better concurrency performance.
// Cache hit path is completely lock-free with no mutex contention.
// SECURITY: Uses Compare-And-Swap to ensure atomic updates and prevent
// race conditions that could cause inconsistent timestamp formatting.
func (tc *timeCache) getFormattedTime() string {
	now := time.Now()

	// Sub-second formats must not use the per-second cache: the cached string
	// would freeze the fractional digits for the whole second.
	if tc.subSecond {
		return now.Format(tc.timeFormat)
	}

	currentSec := now.Unix()

	// Fast path: atomic load to check cache (completely lock-free)
	cached := tc.current.Load()
	if cached != nil && cached.sec == currentSec {
		return cached.formatted
	}

	// Slow path: format time and atomically swap
	// SECURITY: Use CAS loop to ensure only one goroutine updates the cache
	// This prevents race conditions where multiple goroutines format the same second
	// with slightly different nanosecond offsets
	formatted := now.Format(tc.timeFormat)
	newEntry := &cachedTimeEntry{
		sec:       currentSec,
		formatted: formatted,
	}

	// CAS loop: only update if cache is still stale
	// This ensures atomic updates without mutex contention
	// SECURITY: Limit retries to prevent theoretical infinite loop
	const maxCASRetries = 100
	for i := 0; i < maxCASRetries; i++ {
		oldEntry := tc.current.Load()
		if oldEntry != nil && oldEntry.sec == currentSec {
			// Another goroutine already updated it with the same second
			return oldEntry.formatted
		}
		if tc.current.CompareAndSwap(oldEntry, newEntry) {
			return formatted
		}
		// CAS failed, retry
	}

	// Fallback: After CAS consistently fails, check one more time for consistency
	// SECURITY: This ensures we return a cached value if available for the same second
	finalEntry := tc.current.Load()
	if finalEntry != nil && finalEntry.sec == currentSec {
		return finalEntry.formatted
	}
	// Return the locally formatted time as last resort
	// This is extremely unlikely but provides safety guarantee
	return formatted
}

// FormatterConfig holds the configuration for creating a MessageFormatter.
// This is used to pass configuration from the root package without importing it.
type FormatterConfig struct {
	Format        LogFormat
	TimeFormat    string
	IncludeTime   bool
	IncludeLevel  bool
	FullPath      bool
	DynamicCaller bool
	JSON          *JSONOptions
}

// MessageFormatter handles formatting of log messages.
// It supports both text and JSON formats and caches resources for performance.
type MessageFormatter struct {
	format        LogFormat
	timeFormat    string
	includeTime   bool
	includeLevel  bool
	fullPath      bool
	dynamicCaller bool
	// Cached JSON options to avoid repeated allocations
	jsonOpts *JSONOptions
	// Cached merged field names to avoid allocations during logging
	cachedFieldNames *JSONFieldNames
	// Time cache for reducing time formatting overhead
	timeCache *timeCache
	// Learned anchor→offset hint for resolveDynamicCaller's short capture
	// (the formatter-position resolver; entry methods use the fixed-skip
	// CaptureEntryCaller instead)
	callerHint atomic.Pointer[CallerHint]
}

// NewMessageFormatter creates a new MessageFormatter with the given configuration.
func NewMessageFormatter(config *FormatterConfig) *MessageFormatter {
	mf := &MessageFormatter{
		format:        config.Format,
		timeFormat:    config.TimeFormat,
		includeTime:   config.IncludeTime,
		includeLevel:  config.IncludeLevel,
		fullPath:      config.FullPath,
		dynamicCaller: config.DynamicCaller,
		timeCache:     newTimeCache(config.TimeFormat),
	}

	// Pre-compute JSON options to avoid allocations during logging
	if config.JSON != nil {
		mf.jsonOpts = &JSONOptions{
			PrettyPrint: config.JSON.PrettyPrint,
			Indent:      config.JSON.Indent,
			FieldNames:  config.JSON.FieldNames,
		}
		// Pre-merge field names at creation time
		mf.cachedFieldNames = MergeWithDefaults(config.JSON.FieldNames)
	} else {
		mf.jsonOpts = defaultJSONOptions
		// Use default field names when no JSON config provided
		mf.cachedFieldNames = DefaultJSONFieldNames()
	}

	return mf
}

// FormatArgsToString converts arguments to a single string for filtering.
// Complex types (slices, maps, structs) are formatted as JSON for better readability.
// Uses pooled bytes.Buffer to reduce allocations.
// SECURITY: Zeroes buffer contents before returning to pool.
func (f *MessageFormatter) FormatArgsToString(args ...any) string {
	if len(args) == 0 {
		return ""
	}
	if len(args) == 1 {
		return f.formatArgToString(args[0])
	}

	// Use pooled buffer for multiple arguments
	buf := argsBuilderPool.Get().(*bytes.Buffer)
	buf.Reset()

	// SECURITY: Zero buffer contents before returning to pool
	defer func() {
		if buf.Cap() > 1024 {
			// Don't return large buffers to pool - let GC collect them
			// This prevents sensitive data retention in pooled memory
			return
		}
		// Zero the buffer contents for security
		zeroBuffer(buf)
		argsBuilderPool.Put(buf)
	}()

	for i, arg := range args {
		if i > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(f.formatArgToString(arg))
	}
	return buf.String()
}

// formatArgToString converts a single argument to string.
// Uses type switch for common types to avoid fmt.Sprint reflection overhead.
func (f *MessageFormatter) formatArgToString(arg any) string {
	switch val := arg.(type) {
	case string:
		return val
	case int:
		return strconv.FormatInt(int64(val), 10)
	case int64:
		return strconv.FormatInt(val, 10)
	case int32:
		return strconv.FormatInt(int64(val), 10)
	case int16:
		return strconv.FormatInt(int64(val), 10)
	case int8:
		return strconv.FormatInt(int64(val), 10)
	case uint:
		return strconv.FormatUint(uint64(val), 10)
	case uint64:
		return strconv.FormatUint(val, 10)
	case uint32:
		return strconv.FormatUint(uint64(val), 10)
	case uint16:
		return strconv.FormatUint(uint64(val), 10)
	case uint8:
		return strconv.FormatUint(uint64(val), 10)
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(val), 'g', -1, 32)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case time.Duration:
		return val.String()
	case time.Time:
		return val.Format(time.RFC3339)
	case error:
		// SEC-003: user-controlled Error method; FormatArgsToString is called
		// before logCoreWithDepth's recover, so the call must be panic-safe.
		return SafeErrorString(val)
	case fmt.Stringer:
		// SEC-003: user-controlled String method, same rationale as error above.
		return SafeStringerString(val)
	case nil:
		return "<nil>"
	default:
		if IsComplexValue(arg) {
			if jsonData, err := json.Marshal(ConvertValue(arg)); err == nil {
				return string(jsonData)
			}
		}
		return fmt.Sprint(arg)
	}
}

// FormatWithMessage formats a complete log message with level, caller, and fields.
func (f *MessageFormatter) FormatWithMessage(level LogLevel, callerDepth int, message string, fields []Field) string {
	// Resolve the caller from this method's own stack position (legacy
	// formatter-position capture; the logger's hot path resolves at its entry
	// sites instead and calls FormatWithMessageBytes directly).
	var caller string
	if f.dynamicCaller {
		caller = f.resolveDynamicCaller(callerDepth)
	}
	buf := f.FormatWithMessageBytes(level, caller, message, fields)
	s := buf.String()
	PutLineBuffer(buf)
	return s
}

// FormatWithMessageBytes is FormatWithMessage returning the formatted line in
// a pooled buffer instead of a string, so the write path can emit the bytes
// directly without materializing a one-off string copy per log line.
// The caller must already have resolved the caller string (empty when caller
// reporting is disabled); use EntryCaller at the logging entry site.
// Ownership of the returned buffer passes to the caller, which must release
// it with PutLineBuffer exactly once (on every path).
func (f *MessageFormatter) FormatWithMessageBytes(level LogLevel, caller string, message string, fields []Field) *bytes.Buffer {
	buf := GetLineBuffer()
	switch f.format {
	case LogFormatJSON:
		f.formatJSONInto(buf, level, caller, message, fields)
	default:
		f.formatTextInto(buf, level, caller, message, fields)
	}
	return buf
}

// formatTextInto writes the text-format log line for one entry into buf,
// which must be empty (fresh from GetLineBuffer).
func (f *MessageFormatter) formatTextInto(buf *bytes.Buffer, level LogLevel, caller string, message string, fields []Field) {
	// Pre-calculate capacity to reduce memory allocations
	// Base: timestamp (~35) + level (7) + brackets (2) + caller (~30) + message + fields
	estimatedLen := 64 + len(message) + len(fields)*EstimatedFieldSize
	// Grow if needed. Grow(n) only guarantees capacity for n more bytes beyond
	// the current length (0 here), so the argument is the full estimate — the
	// former `estimatedLen - buf.Cap()` arithmetic under-grew and left the
	// buffer at its pool capacity (2048), defeating the single-allocation intent.
	if buf.Cap() < estimatedLen {
		buf.Grow(estimatedLen)
	}

	// Add timestamp and level with brackets
	if f.includeTime || f.includeLevel {
		buf.WriteByte('[')

		// Add timestamp (using cached time for performance)
		if f.includeTime {
			buf.WriteString(f.timeCache.getFormattedTime())
		}

		// Add level with alignment (5 character width, left-padded with spaces)
		if f.includeLevel {
			if f.includeTime {
				buf.WriteString(" ") // Space before level for alignment
			}
			// Use pre-computed padded level string to avoid repeated formatting
			if int(level) >= 0 && int(level) < len(paddedLevelStrings) {
				buf.WriteString(paddedLevelStrings[level])
			} else {
				buf.WriteString(level.String())
			}
		}

		buf.WriteByte(']')
	}

	// Add caller (resolved once in FormatWithMessage)
	if caller != "" {
		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(caller)
	}

	// Add message
	if buf.Len() > 0 {
		buf.WriteByte(' ')
	}
	buf.WriteString(message)

	// Add fields directly to buffer to avoid intermediate string allocation
	if len(fields) > 0 {
		for _, field := range fields {
			if field.Key == "" {
				continue
			}
			buf.WriteByte(' ')
			buf.WriteString(field.Key)
			buf.WriteByte('=')
			formatFieldValueBytes(buf, field.Value)
		}
	}
}

// formatJSONInto writes the JSON-format log line for one entry into buf,
// which must be empty (fresh from GetLineBuffer).
func (f *MessageFormatter) formatJSONInto(buf *bytes.Buffer, level LogLevel, caller string, message string, fields []Field) {
	fieldNames := f.getJSONFieldNames()

	// Fast path: direct buffer writing, avoiding the map allocation entirely.
	// Compact mode accepts every "simple" value the direct writer renders;
	// pretty mode additionally requires scalar values, because nested
	// arrays/objects would need pretty indentation, which only the
	// encoding/json fallback below produces.
	if (!f.jsonOpts.PrettyPrint && allFieldsAreSimple(fields)) ||
		(f.jsonOpts.PrettyPrint && allFieldsAreScalars(fields)) {
		if f.formatJSONDirectInto(buf, level, caller, message, fields, fieldNames) {
			return
		}
		buf.Reset() // fast path bailed out mid-line; rebuild via the map path
	}

	// Slow path: use map-based approach for complex types or pretty print.
	// Use pooled entry map for better performance.
	entryPtr := jsonEntryMapPool.Get().(*map[string]any)
	entry := *entryPtr

	// Clear the map for reuse - clear is more efficient than delete loop
	clear(entry)

	// Add timestamp if enabled (using cached time for performance)
	if f.includeTime {
		entry[fieldNames.Timestamp] = f.timeCache.getFormattedTime()
	}

	// Add level if enabled
	if f.includeLevel {
		entry[fieldNames.Level] = level.String()
	}

	// Add caller (resolved once in FormatWithMessage)
	if caller != "" {
		entry[fieldNames.Caller] = caller
	}

	// Add message
	entry[fieldNames.Message] = message

	// Add structured fields if present
	var fieldsMapPtr *map[string]any
	fieldsCount := len(fields)
	if fieldsCount > 0 {
		// Use pooled fields map
		fieldsMapPtr = jsonFieldsMapPool.Get().(*map[string]any)
		fieldsMap := *fieldsMapPtr
		clear(fieldsMap)
		for _, field := range fields {
			fieldsMap[field.Key] = field.Value
		}
		entry[fieldNames.Fields] = fieldsMap
	}

	// Format JSON directly into the line buffer (no intermediate string copy)
	FormatJSONInto(buf, entry, f.getJSONOptions())

	// SECURITY: Clean up and return maps to pool
	// For large maps, clear and discard to prevent sensitive data retention
	if fieldsMapPtr != nil {
		fieldsMap := *fieldsMapPtr
		clear(fieldsMap) // SECURITY: Zero all values before deciding
		// Use pre-clear count to decide whether to return to pool
		if fieldsCount <= 20 {
			// Only return small maps to pool
			jsonFieldsMapPool.Put(fieldsMapPtr)
		}
		// Large maps are discarded (already cleared, GC will collect)
	}

	// Return entry map to pool (only if small)
	// Count entry fields: timestamp(1) + level(1) + caller(1) + message(1) + fields(0-1) = max 5
	clear(entry)
	// Entry maps are always small (max 5 fields), always return to pool
	jsonEntryMapPool.Put(entryPtr)
}

// allFieldsAreScalars reports whether every field value is a scalar the
// pretty direct writer can render (see isScalarJSONValue).
func allFieldsAreScalars(fields []Field) bool {
	for _, field := range fields {
		if !isScalarJSONValue(field.Value) {
			return false
		}
	}
	return true
}

// isScalarJSONValue returns true for values that render as a single JSON
// token (string/number/bool/null), so a pretty printer can emit them inline
// without recursion. time.Time and time.Duration render as strings.
func isScalarJSONValue(v any) bool {
	switch v.(type) {
	case string, int, int64, int32, int16, int8,
		uint, uint64, uint32, uint16, uint8,
		float64, float32, bool, nil,
		time.Time, time.Duration:
		return true
	}
	return false
}

// allFieldsAreSimple checks if all field values can be written by the fast JSON path.
// Complex types (structs, maps, slices of interfaces) require the map-based approach.
func allFieldsAreSimple(fields []Field) bool {
	for _, field := range fields {
		if !isSimpleJSONValue(field.Value) {
			return false
		}
	}
	return true
}

// isSimpleJSONValue returns true if the value can be written directly to JSON
// without going through the map-based encoding/json path.
func isSimpleJSONValue(v any) bool {
	switch v.(type) {
	case string, int, int64, int32, int16, int8,
		uint, uint64, uint32, uint16, uint8,
		float64, float32, bool, nil,
		time.Time, time.Duration:
		return true
	case []string, []int, []int64, []float64, []bool:
		return true
	default:
		return false
	}
}

// formatJSONDirectInto writes JSON directly into buf without creating maps,
// in compact form or — when PrettyPrint is enabled — with encoding/json-style
// indentation. Returns true on success, or false if the map-based fallback is
// needed (buf then holds a partial line the caller must Reset before the
// fallback).
//
// Pretty output matches the shape encoding/json produces for the same entry
// (newline + indent nesting, ": " after keys) but with a deterministic key
// order (timestamp, level, caller, message, fields) instead of the map path's
// randomized iteration order. Scalar values are formatted by the same writers
// as the compact fast path, so the two paths stay byte-consistent.
//
// Implemented with plain helper calls rather than closures on purpose: the
// compact path is the JSON logging hot path, and calls through closure values
// cannot be inlined (measured as a ~30% JSON-path regression).
func (f *MessageFormatter) formatJSONDirectInto(buf *bytes.Buffer, level LogLevel, caller string, message string, fields []Field, fieldNames *JSONFieldNames) bool {
	pretty := f.jsonOpts.PrettyPrint
	indent := f.jsonOpts.Indent

	buf.WriteByte('{')

	first := true
	// Write timestamp
	if f.includeTime {
		first = jsonMemberSep(buf, pretty, indent, first, 1)
		writeJSONMemberKey(buf, fieldNames.Timestamp, pretty)
		writeJSONString(buf, f.timeCache.getFormattedTime())
	}

	// Write level
	if f.includeLevel {
		first = jsonMemberSep(buf, pretty, indent, first, 1)
		writeJSONMemberKey(buf, fieldNames.Level, pretty)
		writeJSONString(buf, level.String())
	}

	// Write caller
	if caller != "" {
		first = jsonMemberSep(buf, pretty, indent, first, 1)
		writeJSONMemberKey(buf, fieldNames.Caller, pretty)
		writeJSONString(buf, caller)
	}

	// Write message
	first = jsonMemberSep(buf, pretty, indent, first, 1)
	writeJSONMemberKey(buf, fieldNames.Message, pretty)
	writeJSONString(buf, message)

	// Write fields
	if len(fields) > 0 {
		// fields is the last entry member, so the separator state it returns
		// is not needed — only the separator bytes it writes matter.
		jsonMemberSep(buf, pretty, indent, first, 1)
		writeJSONString(buf, fieldNames.Fields)
		if pretty {
			buf.WriteString(": {")
		} else {
			buf.WriteString(":{")
		}
		fieldFirst := true
		for _, field := range fields {
			if field.Key == "" {
				continue
			}
			fieldFirst = jsonMemberSep(buf, pretty, indent, fieldFirst, 2)
			writeJSONMemberKey(buf, field.Key, pretty)
			if !writeJSONValueFast(buf, field.Value) {
				return false // Need fallback for complex type
			}
		}
		if !fieldFirst {
			jsonNl(buf, pretty, indent, 1) // non-empty object: closing brace on its own line
		}
		buf.WriteByte('}')
	}

	jsonNl(buf, pretty, indent, 0)
	buf.WriteByte('}')
	return true
}

// jsonNl writes the pretty-mode newline+indent separator at the given nesting
// depth (0 = entry object's closing brace, 1 = entry members, 2 = field
// members); a no-op in compact mode.
func jsonNl(buf *bytes.Buffer, pretty bool, indent string, depth int) {
	if !pretty {
		return
	}
	buf.WriteByte('\n')
	for i := 0; i < depth; i++ {
		buf.WriteString(indent)
	}
}

// jsonMemberSep writes the separator before an object member: nothing before
// the first member in compact mode; in pretty mode every member (including
// the first) starts on its own indented line, later ones after a comma.
// Returns the updated first-member state.
func jsonMemberSep(buf *bytes.Buffer, pretty bool, indent string, first bool, depth int) bool {
	if first {
		jsonNl(buf, pretty, indent, depth)
		return false
	}
	buf.WriteByte(',')
	jsonNl(buf, pretty, indent, depth)
	return false
}

// writeJSONMemberKey writes a member key and the colon that follows it —
// ": " in pretty mode (matching encoding/json), ":" in compact mode.
func writeJSONMemberKey(buf *bytes.Buffer, key string, pretty bool) {
	writeJSONString(buf, key)
	if pretty {
		buf.WriteString(": ")
	} else {
		buf.WriteByte(':')
	}
}

// getJSONFieldNames returns the cached JSON field names configuration.
// Field names are pre-merged at formatter creation time to avoid allocations.
func (f *MessageFormatter) getJSONFieldNames() *JSONFieldNames {
	// Return the pre-cached merged field names
	if f.cachedFieldNames != nil {
		return f.cachedFieldNames
	}
	// Fallback (should never happen if properly initialized)
	return DefaultJSONFieldNames()
}

// getJSONOptions returns the JSON formatting options (cached at initialization)
func (f *MessageFormatter) getJSONOptions() *JSONOptions {
	return f.jsonOpts
}

// FullPath reports whether caller strings use the full file path. Consumed
// by the entry-dispatch caller capture (EntryCaller).
func (f *MessageFormatter) FullPath() bool {
	return f.fullPath
}

// resolveDynamicCaller returns the formatted caller resolved from the
// formatter's own stack position. It is retained for callers that format a
// message without going through the logger's entry sites (FormatWithMessage,
// and as the fallback for log-entry paths that did not capture a caller);
// the logger's hot path uses EntryCaller instead, which captures the
// stack from the entry dispatch functions — several frames closer to user
// code, so the runtime.Callers unwind (the dominant per-entry CPU cost)
// resolves the minimum number of frames.
//
// Steady-state path (anchor offset learned):
//  1. One runtime.Callers capture of exactly offset+1 frames — the unwind cost
//     scales with frames walked, and the user frame sits only a handful of
//     frames above the capture anchor, so capturing a full 32-frame window
//     wastes most of the work.
//  2. Verify pcs[0] still matches the learned anchor (guards against a future
//     second call site of FormatWithMessage sharing this formatter).
//  3. Index straight to the user-code frame's PC (no frame walking).
//  4. Resolve that PC's file:line through the shared callerCache.
//
// Cold path (first call from an anchor, anchor changed, or a stack too shallow
// to contain the user frame): full capture + walk, learn the hint, or fall
// back to GetCaller(baseDepth) — preserving the exact previous behavior, so
// correctness is never worse than before.
//
// SECURITY: frame indexing is bounded by maxSafeOffset to prevent out-of-range
// access; the hint is published race-free via atomic pointer.
func (f *MessageFormatter) resolveDynamicCaller(baseDepth int) string {
	if baseDepth < 0 {
		baseDepth = 0
	}

	// On-stack capture buffer: maxSafeOffset+1 slots cover every frame index
	// the resolver can address (no pool traffic, no bounds risk).
	var pcs [maxSafeOffset + 1]uintptr

	// Hot path: capture only the frames the hint needs (offset+1, typically
	// 4-5) instead of unwinding the full 32-frame window.
	if hint := f.callerHint.Load(); hint != nil && hint.Offset <= maxSafeOffset {
		need := hint.Offset + 1
		// pcs[0] = the caller of FormatWithMessage (skip Callers,
		// resolveDynamicCaller, FormatWithMessage). For the logger hot path
		// this is logCoreWithDepth.
		if n := runtime.Callers(3, pcs[:need]); n == need && pcs[0] == hint.AnchorPC {
			return callerForPC(pcs[hint.Offset], f.fullPath)
		}
		// Anchor changed or shallow stack: re-learn below.
	}

	// Cold path: full capture, then find the first user (non-dd) frame.
	n := runtime.Callers(3, pcs[:])
	off, found := findUserFrame(pcs[:], n)
	if !found {
		// No user frame in the captured window: fall back to a full GetCaller.
		return GetCaller(baseDepth, f.fullPath)
	}

	f.callerHint.Store(&CallerHint{AnchorPC: pcs[0], Offset: off})
	return callerForPC(pcs[off], f.fullPath)
}

// entryCallerSkip is the fixed runtime.Callers skip that lands directly on
// the first frame outside the dd module from the entry-dispatch capture
// point. Logical stack at the capture:
//
//	Callers(0) EntryCaller(1) entry dispatch(2) entry method(3) user code(4)
//
// The count is stable across the compiler's inlining decisions: an inlined
// function still reports as its own logical frame, and there is exactly one
// library function (EntryCaller itself) between the dispatch and the
// capture. A second wrapper layer is NOT stable — a tail-positioned wrapper
// call that gets inlined can lose its logical frame entirely (observed
// empirically) — so the dispatch functions must call EntryCaller DIRECTLY.
// The shape is pinned by TestEntrySiteCallerAccuracy, which asserts exact
// caller lines for every entry method.
const entryCallerSkip = 4

// ddModuleDir is this module's source root (…/dd), derived once from this
// file's own path. It scopes the entry-funnel file check below to THIS
// module's logger.go/entry.go — a user module's own logger.go must not be
// mistaken for the funnel.
var ddModuleDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	if i := strings.LastIndex(file, "/internal/"); i > 0 {
		return file[:i]
	}
	if i := strings.LastIndex(file, "\\internal\\"); i > 0 {
		return file[:i]
	}
	return ""
}()

// isFunnelSourceFile reports whether a source file is one of dd's entry
// funnel files — the files containing the entry dispatchers and entry
// methods (logger.go, entry.go in THIS module). The entry-caller capture
// uses it to reject frames the fixed skip can only land on when its frame
// shape is violated.
func isFunnelSourceFile(file string) bool {
	if ddModuleDir == "" || !strings.HasPrefix(file, ddModuleDir) {
		return false
	}
	return strings.HasSuffix(file, "logger.go") || strings.HasSuffix(file, "entry.go")
}

// degenerateEntryCallerDepth is the depth-based fallback used when the
// fixed-skip capture does not land on user code (see EntryCaller). From
// GetCaller's position the user frame sits four frames up: GetCaller,
// EntryCaller, the entry dispatch, the entry method.
const degenerateEntryCallerDepth = 4

// EntryCaller resolves the caller for a log entry from the shared
// entry-dispatch functions (logDispatch/logfDispatch/logWithDispatch on both
// Logger and LoggerEntry), which must call it DIRECTLY: the fixed skip above
// assumes exactly one dispatch frame and one entry-method frame below the
// capture, a shape that holds for every public entry method.
//
// Resolving here — one frame below the entry method — walks the minimum
// possible number of stack frames. The previous design resolved from the
// formatter, 7-9 frames deeper, and that stack unwind was 76% of
// SimpleLogging's CPU (pprof).
//
// A learned offset cannot replace the fixed skip: wrapper entry methods
// (Info→Log) and direct calls (Log) reach a shared funnel at DIFFERENT stack
// depths, so any memoized anchor→offset hint is poisoned by whichever path
// warms it first — resolveDynamicCaller's hint has exactly that latent bug,
// first exposed by the line-exact entry-site test.
//
// The captured frame is validated against dd's entry-funnel files (see
// isFunnelSourceFile) via a flag on the caller cache — free on a cache hit.
// A funnel frame means the fixed skip landed too shallow (a future internal
// path breaking the frame shape); resolution then degrades to the
// depth-based fallback instead of reporting a dd-internal frame. Callers
// elsewhere in this module (tests, examples) pass the check and keep the
// fast path.
func EntryCaller(fullPath bool) string {
	var pcs [1]uintptr
	if runtime.Callers(entryCallerSkip, pcs[:]) == 0 {
		return GetCaller(degenerateEntryCallerDepth, fullPath)
	}
	if caller, ok := callerForPCValidated(pcs[0], fullPath); ok {
		return caller
	}
	// Shape violation (a future internal path breaking the frame shape) or an
	// in-module caller: resolve by depth instead of reporting a dd-internal frame.
	return GetCaller(degenerateEntryCallerDepth, fullPath)
}

// findUserFrame walks pcs[0:n] (an already-captured PC window) and returns the
// index of the first user (non-dd) frame, or found=false when the window
// contains none. It contains no runtime.Callers call on purpose: a capture
// issued from a different function sits one frame lower and shifts the anchor
// out from under the hot path's hint validation (measured as a permanent
// cold-path regression: a fresh CallerHint allocation per log call).
// FuncForPC indexes pcs 1:1 (unlike CallersFrames, which expands inlined
// frames), so the returned index is safe to reuse as a pcs subscript.
// Shared by both dynamic-caller resolvers.
func findUserFrame(pcs []uintptr, n int) (off int, found bool) {
	if n == 0 {
		return 0, false
	}

	pkgPrefix := getDDPackagePrefix()
	pkgLen := len(pkgPrefix)
	for i := 0; i < n && i <= maxSafeOffset; i++ {
		fn := runtime.FuncForPC(pcs[i])
		if fn == nil {
			continue
		}
		if !isDDFunction(fn.Name(), pkgPrefix, pkgLen) {
			return i, true
		}
	}
	return 0, false
}

// ResolveCaller resolves the caller from the formatter's own stack position.
// Exported fallback for log-entry paths that reach the formatter without an
// entry-site capture, so no path can silently lose caller reporting.
func (f *MessageFormatter) ResolveCaller(callerDepth int) string {
	if !f.dynamicCaller {
		return ""
	}
	return f.resolveDynamicCaller(callerDepth)
}

// maxSafeOffset bounds frame indexing in resolveDynamicCaller to prevent
// out-of-range access and guard against pathological stacks. The user frame sits
// only a handful of frames above the capture anchor, so the pcs buffer capacity
// (33) is a safe ceiling.
const maxSafeOffset = 32

// isDDFunction reports whether a fully-qualified function name belongs to the dd
// module's own packages (the root dd package or any subpackage such as /internal).
// Such frames are skipped during dynamic caller detection. Extracted from the old
// adjustCallerDepth inline check so it can be unit-tested.
func isDDFunction(name, prefix string, pkgLen int) bool {
	if len(name) <= pkgLen || name[:pkgLen] != prefix {
		return false
	}
	rest := name[pkgLen:]
	return len(rest) > 0 && (rest[0] == '.' || rest[0] == '/')
}
