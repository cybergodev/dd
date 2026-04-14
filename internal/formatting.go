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
// It uses runtime.Caller to dynamically detect the prefix at initialization.
func getDDPackagePrefix() string {
	ddPackagePrefixOnce.Do(func() {
		// Get the function name of this file to extract the package prefix
		// Example: github.com/cybergodev/dd/internal.getDDPackagePrefix
		// We want: github.com/cybergodev/dd
		if _, file, _, ok := runtime.Caller(0); ok {
			// file is like: /path/to/github.com/cybergodev/dd/internal/formatting.go
			// Find the package path in the file path
			// Look for the pattern: module/path/package/file.go

			// Extract the package prefix by finding the dd package in the path
			// Common patterns:
			// - github.com/user/dd/...
			// - golang.org/x/...
			parts := strings.Split(file, "/")
			for i, part := range parts {
				if part == "dd" && i > 0 {
					// Found "dd" package, construct prefix from parts before it
					ddPackagePrefix = strings.Join(parts[:i+1], "/")
					return
				}
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

// pcsPool pools []uintptr slices for runtime.Callers
// to reduce memory allocations in adjustCallerDepth.
var pcsPool = sync.Pool{
	New: func() any {
		pcs := make([]uintptr, 32) // typical call stack depth
		return &pcs
	},
}

// depthCacheEntry stores cached adjusted caller depth
type depthCacheEntry struct {
	pc     uintptr // program counter used as key
	depth  int     // adjusted depth value
}

// depthCache caches adjusted caller depth to avoid repeated stack walking.
// Key: the first non-dd PC in the call stack, Value: adjusted depth.
// This dramatically reduces allocations in the hot path.
var depthCache sync.Map

// maxDepthCacheSize limits the cache size to prevent unbounded memory growth.
const maxDepthCacheSize = 5000

// depthCacheCount tracks the number of entries for size limiting
var depthCacheCount atomic.Int32

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
	// Adjust caller depth if dynamic detection is enabled
	if f.dynamicCaller {
		callerDepth = f.adjustCallerDepth(callerDepth)
	}

	switch f.format {
	case LogFormatJSON:
		return f.formatJSON(level, callerDepth, message, fields)
	default:
		return f.formatText(level, callerDepth, message, fields)
	}
}

func (f *MessageFormatter) formatText(level LogLevel, callerDepth int, message string, fields []Field) string {
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

	// Add caller
	if f.dynamicCaller {
		if callerInfo := GetCaller(callerDepth, f.fullPath); callerInfo != "" {
			if buf.Len() > 0 {
				buf.WriteByte(' ')
			}
			buf.WriteString(callerInfo)
		}
	}

	// Add message
	if buf.Len() > 0 {
		buf.WriteByte(' ')
	}
	buf.WriteString(message)

	// Add fields
	if len(fields) > 0 {
		if fieldsStr := FormatFields(fields); fieldsStr != "" {
			buf.WriteByte(' ')
			buf.WriteString(fieldsStr)
		}
	}

	return buf.String()
}

func (f *MessageFormatter) formatJSON(level LogLevel, callerDepth int, message string, fields []Field) string {
	fieldNames := f.getJSONFieldNames()

	// Use pooled entry map for better performance
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

	// Add caller if enabled
	if f.dynamicCaller {
		if callerInfo := GetCaller(callerDepth, f.fullPath); callerInfo != "" {
			entry[fieldNames.Caller] = callerInfo
		}
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

// adjustCallerDepth adjusts the caller depth based on dynamic caller detection.
// This method looks for the first non-dd package in the call stack.
// Returns the depth relative to GetCaller in formatText.
//
// Performance note: Uses depthCache to avoid repeated stack walking for the same call sites.
// This dramatically reduces allocations and CPU usage in the hot path.
//
// SECURITY: Includes integer overflow protection for depth calculations.
func (f *MessageFormatter) adjustCallerDepth(baseDepth int) int {
	// Validate base depth
	if baseDepth < 0 {
		baseDepth = 0
	}

	// SECURITY: Maximum safe depth to prevent integer overflow and stack issues
	const maxSafeDepth = 1000
	if baseDepth > maxSafeDepth {
		// Return a safe default to prevent incorrect caller info
		return maxSafeDepth
	}

	// Fast path: get the first PC to check cache
	// We cache based on the first frame in the Log method
	pcsPtr := pcsPool.Get().(*[]uintptr)
	pcs := *pcsPtr
	defer pcsPool.Put(pcsPtr)

	// Get frames for both cache lookup and stack walking
	// Skip: runtime.Callers (0), adjustCallerDepth (1), FormatWithMessage (2)
	n := runtime.Callers(3, pcs)
	if n == 0 {
		return baseDepth
	}

	firstPC := pcs[0]

	// Check cache for this call site
	if cached, ok := depthCache.Load(firstPC); ok {
		return cached.(*depthCacheEntry).depth
	}

	// Cache miss - walk the stack to find user code
	// Get the dynamically detected package prefix
	pkgPrefix := getDDPackagePrefix()
	pkgPrefixLen := len(pkgPrefix)

	// Iterate through frames
	frames := runtime.CallersFrames(pcs[:n])

	for depth := 0; depth <= maxSafeDepth; depth++ {
		frame, more := frames.Next()
		if !more {
			break
		}

		// Check if function belongs to dd package using dynamic prefix
		fn := frame.Function
		if len(fn) > pkgPrefixLen && fn[:pkgPrefixLen] == pkgPrefix {
			// It's in the dd module, check if it's the dd package
			rest := fn[pkgPrefixLen:]
			// rest could be: ".pkg.func" or "/subpkg.func" or ".func"
			if len(rest) >= 1 {
				// Skip if still in dd package or its subpackages
				// Pattern: /dd.pkg or /dd/pkg or /dd)
				if rest[0] == '.' || rest[0] == '/' {
					continue // Still in dd package
				}
			}
		}

		// Found user code - calculate adjusted depth
		// From adjustCallerDepth's perspective (skip=3):
		//   depth=0 = Log method, depth=1 = Print method, depth=2 = user code
		// From GetCaller's perspective (called from formatText):
		//   Caller(0) = GetCaller, Caller(1) = formatText, Caller(2) = FormatWithMessage
		//   Caller(3) = Log, Caller(4) = Print, Caller(5) = user code
		// So GetCaller needs depth + 3 to reach the same frame
		// SECURITY: Clamp to prevent any potential overflow
		adjustedDepth := min(depth+3, maxSafeDepth)

		// Cache the result for future calls
		// SECURITY: Use CAS loop to ensure precise cache size limiting
		for {
			current := depthCacheCount.Load()
			if current >= maxDepthCacheSize {
				break // Cache full, skip caching
			}
			// Try to reserve a slot
			if depthCacheCount.CompareAndSwap(current, current+1) {
				// Slot reserved, now try to store
				entry := &depthCacheEntry{pc: firstPC, depth: adjustedDepth}
				if _, loaded := depthCache.LoadOrStore(firstPC, entry); loaded {
					// Another goroutine stored first, release our slot
					depthCacheCount.Add(-1)
				}
				break // Exit after successful reservation (whether stored or loaded)
			}
			// CAS failed, retry
		}

		return adjustedDepth
	}

	return baseDepth
}
