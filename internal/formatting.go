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

// textBuilderPool pools bytes.Buffer objects for text formatting
// to reduce memory allocations during high-frequency logging.
// Initial capacity of 2048 bytes covers most common log messages:
// base (~80) + timestamp (~35) + caller (~50) + message (~500) + 10 fields (~400) + safety margin
// Increased from 1024 to reduce grow() calls which were 58% of allocations
// SECURITY: Uses bytes.Buffer instead of strings.Builder to allow proper
// zeroing of sensitive data before returning to pool.
var textBuilderPool = sync.Pool{
	New: func() any {
		buf := &bytes.Buffer{}
		buf.Grow(2048) // optimized for typical log entries, reduced grow overhead
		return buf
	},
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

// pcsPool pools []uintptr slices for the single stack capture in
// resolveDynamicCaller. Size 32 comfortably covers the handful of dd-internal
// frames between the capture anchor and user code.
var pcsPool = sync.Pool{
	New: func() any {
		pcs := make([]uintptr, 32) // typical call stack depth
		return &pcs
	},
}

// dynamicOffsetEntry stores the cached index of the first user (non-dd) frame
// in the stack captured by resolveDynamicCaller, relative to the capture anchor.
type dynamicOffsetEntry struct {
	offset int
}

// dynamicOffsetCache memoizes, per capture-anchor PC, the index of the first
// user-code frame. The anchor (e.g. logCoreWithDepth) is shared across all log
// calls, but the offset is constant for a given build, so a single memo per
// anchor is valid. This lets the steady-state path capture the stack ONCE and
// index straight to the user frame instead of walking it — replacing the prior
// adjustCallerDepth + GetCaller pair that each called runtime.Callers separately
// (the dominant cost: ~80% of SimpleLogging CPU per pprof).
var dynamicOffsetCache sync.Map

// maxDynamicOffsetCacheSize bounds the cache. Anchor PCs are few (a handful of
// entry points), so this is a generous safety cap, not a routinely hit limit.
const maxDynamicOffsetCacheSize = 1024

// dynamicOffsetCount tracks entries for size limiting.
var dynamicOffsetCount atomic.Int32

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
}

// newTimeCache creates a new time cache with the given format
func newTimeCache(timeFormat string) *timeCache {
	tc := &timeCache{
		timeFormat: timeFormat,
	}
	// Initialize with zero entry to avoid nil checks
	tc.current.Store(&cachedTimeEntry{sec: -1, formatted: ""})
	return tc
}

// getFormattedTime returns the formatted current time.
// Uses lock-free atomic operations for better concurrency performance.
// Cache hit path is completely lock-free with no mutex contention.
// SECURITY: Uses Compare-And-Swap to ensure atomic updates and prevent
// race conditions that could cause inconsistent timestamp formatting.
func (tc *timeCache) getFormattedTime() string {
	now := time.Now()
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
		return val.Error()
	case fmt.Stringer:
		return val.String()
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
	// Resolve the caller ONCE for the whole entry. The previous implementation
	// called adjustCallerDepth here and GetCaller inside each format function —
	// two runtime.Callers captures (stack unwinds) per log line, which pprof
	// showed was ~80% of SimpleLogging CPU. resolveDynamicCaller does it in a
	// single capture. callerDepth is retained only as the cold-path fallback for
	// GetCaller when the single-capture fast path cannot locate the user frame.
	var caller string
	if f.dynamicCaller {
		caller = f.resolveDynamicCaller(callerDepth)
	}

	switch f.format {
	case LogFormatJSON:
		return f.formatJSON(level, caller, message, fields)
	default:
		return f.formatText(level, caller, message, fields)
	}
}

func (f *MessageFormatter) formatText(level LogLevel, caller string, message string, fields []Field) string {
	// Pre-calculate capacity to reduce memory allocations
	// Base: timestamp (~35) + level (7) + brackets (2) + caller (~30) + message + fields
	estimatedLen := 64 + len(message) + len(fields)*EstimatedFieldSize

	// Get bytes.Buffer from pool
	buf := textBuilderPool.Get().(*bytes.Buffer)
	buf.Reset()
	// Grow if needed
	if buf.Cap() < estimatedLen {
		buf.Grow(estimatedLen - buf.Cap())
	}

	// SECURITY: Zero buffer contents before returning to pool
	defer func() {
		if buf.Cap() > 4096 {
			// Don't return large buffers to pool - let GC collect them
			// This limits sensitive data retention in pooled memory
			return
		}
		// Zero the buffer contents for security
		zeroBuffer(buf)
		textBuilderPool.Put(buf)
	}()

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
		wroteField := false
		for _, field := range fields {
			if field.Key == "" {
				continue
			}
			if !wroteField {
				buf.WriteByte(' ')
				wroteField = true
			} else {
				buf.WriteByte(' ')
			}
			buf.WriteString(field.Key)
			buf.WriteByte('=')
			formatFieldValueBytes(buf, field.Value)
		}
	}

	return buf.String()
}

func (f *MessageFormatter) formatJSON(level LogLevel, caller string, message string, fields []Field) string {
	fieldNames := f.getJSONFieldNames()

	// Fast path: direct buffer writing for compact JSON with simple field types.
	// Avoids map allocation entirely for the common case of primitive field values.
	if !f.jsonOpts.PrettyPrint && allFieldsAreSimple(fields) {
		if result, ok := f.formatJSONDirect(level, caller, message, fields, fieldNames); ok {
			return result
		}
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

	// Format JSON
	result := FormatJSON(entry, f.getJSONOptions())

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

	return result
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

// formatJSONDirect writes JSON directly to a buffer without creating maps.
// Returns (result, true) on success, or ("", false) if fallback is needed.
// SECURITY: Zeroes buffer contents before returning to pool.
func (f *MessageFormatter) formatJSONDirect(level LogLevel, caller string, message string, fields []Field, fieldNames *JSONFieldNames) (string, bool) {
	buf := jsonBuilderPool.Get().(*bytes.Buffer)
	buf.Reset()

	// SECURITY: Clear buffer contents before returning to pool
	defer func() {
		zeroBuffer(buf)
		jsonBuilderPool.Put(buf)
	}()

	buf.WriteByte('{')

	first := true

	// Write timestamp
	if f.includeTime {
		writeJSONString(buf, fieldNames.Timestamp)
		buf.WriteByte(':')
		writeJSONString(buf, f.timeCache.getFormattedTime())
		first = false
	}

	// Write level
	if f.includeLevel {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		writeJSONString(buf, fieldNames.Level)
		buf.WriteByte(':')
		writeJSONString(buf, level.String())
	}

	// Write caller (resolved once in FormatWithMessage)
	if caller != "" {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		writeJSONString(buf, fieldNames.Caller)
		buf.WriteByte(':')
		writeJSONString(buf, caller)
	}

	// Write message
	if !first {
		buf.WriteByte(',')
	}
	first = false
	writeJSONString(buf, fieldNames.Message)
	buf.WriteByte(':')
	writeJSONString(buf, message)

	// Write fields
	if len(fields) > 0 {
		if !first {
			buf.WriteByte(',')
		}
		writeJSONString(buf, fieldNames.Fields)
		buf.WriteString(":{")
		fieldFirst := true
		for _, field := range fields {
			if field.Key == "" {
				continue
			}
			if !fieldFirst {
				buf.WriteByte(',')
			}
			fieldFirst = false
			writeJSONString(buf, field.Key)
			buf.WriteByte(':')
			if !writeJSONValueFast(buf, field.Value) {
				return "", false // Need fallback for complex type
			}
		}
		buf.WriteByte('}')
	}

	buf.WriteByte('}')
	return buf.String(), true
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

// resolveDynamicCaller returns the formatted caller of the current log call using
// a SINGLE runtime.Callers capture. It replaces the previous two-step
// adjustCallerDepth (compute a depth) + GetCaller (re-capture the stack to read
// the frame) sequence, which together issued two runtime.Callers calls per log
// line — the dominant CPU cost (stack unwinding was ~80% of SimpleLogging per
// pprof).
//
// Steady-state path (anchor offset memoized):
//  1. One runtime.Callers capture from a fixed shallow skip.
//  2. Read the memoized frame offset for the capture-anchor PC.
//  3. Index straight to the user-code frame's PC (no frame walking).
//  4. Resolve that PC's file:line through the shared callerCache.
//
// Cold path (first call from an anchor, or a stack too shallow to contain the
// user frame): falls back to GetCaller(baseDepth), preserving the exact previous
// behavior, so correctness is never worse than before.
//
// SECURITY: frame indexing is bounded by maxSafeOffset to prevent out-of-range
// access; the offset cache is size-limited and updated race-free.
func (f *MessageFormatter) resolveDynamicCaller(baseDepth int) string {
	if baseDepth < 0 {
		baseDepth = 0
	}

	pcsPtr := pcsPool.Get().(*[]uintptr)
	pcs := *pcsPtr
	defer pcsPool.Put(pcsPtr)

	// pcs[0] = the caller of FormatWithMessage (skip Callers, resolveDynamicCaller,
	// FormatWithMessage). For the logger hot path this is logCoreWithDepth, which
	// is the same anchor the old depthCache keyed on.
	n := runtime.Callers(3, pcs)
	if n == 0 {
		return GetCaller(baseDepth, f.fullPath)
	}

	firstPC := pcs[0]

	// Hot path: memoized offset → index directly to the user frame's PC.
	if cached, ok := dynamicOffsetCache.Load(firstPC); ok {
		off := cached.(*dynamicOffsetEntry).offset
		if off < n && off <= maxSafeOffset {
			return callerForPC(pcs[off], f.fullPath)
		}
		// Offset not reachable with the captured frames: fall back.
		return GetCaller(baseDepth, f.fullPath)
	}

	// Cold path: walk the captured PCs to find the first user (non-dd) frame.
	// FuncForPC indexes pcs 1:1 (unlike CallersFrames, which expands inlined
	// frames), so the resulting index is safe to reuse as a pcs subscript.
	pkgPrefix := getDDPackagePrefix()
	pkgLen := len(pkgPrefix)
	off := -1
	for i := 0; i < n && i <= maxSafeOffset; i++ {
		fn := runtime.FuncForPC(pcs[i])
		if fn == nil {
			continue
		}
		if !isDDFunction(fn.Name(), pkgPrefix, pkgLen) {
			off = i
			break
		}
	}
	if off < 0 {
		// No user frame in the captured window: fall back to a full GetCaller.
		return GetCaller(baseDepth, f.fullPath)
	}

	storeDynamicOffset(firstPC, off)
	return callerForPC(pcs[off], f.fullPath)
}

// maxSafeOffset bounds frame indexing in resolveDynamicCaller to prevent
// out-of-range access and guard against pathological stacks. The user frame sits
// only a handful of frames above the capture anchor, so the pcs buffer capacity
// (32) is a safe ceiling.
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

// storeDynamicOffset caches the anchor→offset memo with a size limit, mirroring
// the callerCache bounding pattern (CAS counter + LoadOrStore) to stay race-free.
func storeDynamicOffset(anchorPC uintptr, offset int) {
	for {
		current := dynamicOffsetCount.Load()
		if current >= maxDynamicOffsetCacheSize {
			return // cache full; skip caching (correctness unaffected, just slower)
		}
		if dynamicOffsetCount.CompareAndSwap(current, current+1) {
			entry := &dynamicOffsetEntry{offset: offset}
			if _, loaded := dynamicOffsetCache.LoadOrStore(anchorPC, entry); loaded {
				dynamicOffsetCount.Add(-1) // another goroutine won; release our slot
			}
			return
		}
	}
}
