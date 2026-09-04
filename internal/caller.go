package internal

import (
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
)

// callerCache caches caller information to reduce runtime.Caller calls.
// Uses sync.Map for thread-safe concurrent access without locking.
// Key: uintptr (program counter), Value: *callerCacheEntry
var callerCache sync.Map

// callerCacheEntry stores cached caller information
type callerCacheEntry struct {
	file      string
	line      int
	formatted string // pre-formatted "file:line" string
	isFunnel  bool   // frame is dd's own entry funnel (entry-capture validation)
}

// maxCallerCacheSize limits the cache size to prevent unbounded memory growth.
// Each entry is ~100-200 bytes, so 10000 entries ~= 1-2 MB.
const maxCallerCacheSize = 10000

// callerCacheCount tracks the number of entries for size limiting
var callerCacheCount atomic.Int32

// callerPCPool pools []uintptr slices for GetCaller to reduce allocations.
// Each slice is size 1 since we only need a single PC.
var callerPCPool = sync.Pool{
	New: func() any {
		pcs := make([]uintptr, 1)
		return &pcs
	},
}

// GetCaller retrieves the caller information at the specified depth.
// Uses a cache to reduce runtime.Caller calls for repeated call sites.
func GetCaller(callerDepth int, fullPath bool) string {
	if callerDepth < 0 {
		callerDepth = 0
	}

	// Get pooled []uintptr slice (size 1)
	pcsPtr := callerPCPool.Get().(*[]uintptr)
	pcs := *pcsPtr
	defer callerPCPool.Put(pcsPtr)

	// Use runtime.Callers to get the PC for caching
	n := runtime.Callers(callerDepth+1, pcs) // +1 to skip GetCaller itself
	if n == 0 {
		return ""
	}

	return callerForPC(pcs[0], fullPath)
}

// callerForPC returns the formatted caller string for a single return program
// counter, resolving file:line through callerCache. On a cache hit it is a single
// map lookup plus an allocation-free format (line numbers < 100 use smallInts).
// On a miss it resolves the frame via runtime.CallersFrames and caches the result.
//
// Shared by GetCaller and the formatter's caller resolvers so all reuse one
// cache with identical formatting.
func callerForPC(pc uintptr, fullPath bool) string {
	caller, _ := callerForPCLookup(pc, fullPath, false)
	return caller
}

// callerForPCValidated is callerForPC that additionally reports whether the
// frame is acceptable as an entry-captured caller: anything except dd's own
// entry funnel (see isFunnelSourceFile). The flag rides on the same cache
// entry, so validation is free on the (hot) cache-hit path.
func callerForPCValidated(pc uintptr, fullPath bool) (string, bool) {
	return callerForPCLookup(pc, fullPath, true)
}

// callerForPCLookup implements callerForPC and callerForPCValidated.
func callerForPCLookup(pc uintptr, fullPath, validate bool) (string, bool) {
	// Check cache first (fast path - no allocation needed)
	if cached, ok := callerCache.Load(pc); ok {
		entry := cached.(*callerCacheEntry)
		if validate && entry.isFunnel {
			return "", false
		}
		if fullPath {
			// Re-format from cached full path
			return formatCallerDirect(entry.file, entry.line), true
		}
		return entry.formatted, true
	}

	// Cache miss - resolve the single frame for this PC.
	frames := runtime.CallersFrames([]uintptr{pc})
	frame, _ := frames.Next()
	if frame.PC == 0 {
		return "", validate
	}

	// Classify the frame: dd's entry funnel or acceptable caller. Used by the
	// entry-caller capture to reject capture-shape violations (a frame from
	// the funnel itself means the fixed skip landed too shallow); harmless
	// for plain lookups. Module-internal test and example files are NOT
	// funnel files, so in-module callers keep the fast path.
	frameIsFunnel := isFunnelSourceFile(frame.File)

	// Get base name for short path version (always cache short path)
	baseName := getBaseName(frame.File)

	// Create cache entry with pre-formatted short path
	formatted := formatCallerDirect(baseName, frame.Line)
	entry := &callerCacheEntry{
		file:      frame.File, // Store full path
		line:      frame.Line,
		formatted: formatted, // Pre-formatted short path
		isFunnel:  frameIsFunnel,
	}

	// Store in cache with size limit
	// SECURITY: Use CAS loop to ensure precise cache size limiting
	for {
		current := callerCacheCount.Load()
		if current >= maxCallerCacheSize {
			break // Cache full, skip caching
		}
		// Try to reserve a slot
		if callerCacheCount.CompareAndSwap(current, current+1) {
			// Slot reserved, now try to store
			if _, loaded := callerCache.LoadOrStore(pc, entry); loaded {
				// Another goroutine stored first, release our reserved slot
				callerCacheCount.Add(-1)
			}
			break // Exit after successful reservation (whether stored or loaded)
		}
		// CAS failed, retry
	}

	// Return based on fullPath setting
	if validate && frameIsFunnel {
		return "", false
	}
	if fullPath {
		return formatCallerDirect(frame.File, frame.Line), true
	}
	return formatted, true
}

// formatCallerDirect formats file and line without using pool.
// Used for cached results to avoid pool overhead.
func formatCallerDirect(file string, line int) string {
	// Direct string concatenation is faster than builder for short strings
	return file + ":" + FormatInt(line)
}

// getBaseName extracts the base filename from a path without allocating.
// This is faster than filepath.Base for the common case.
func getBaseName(path string) string {
	// Find the last separator
	// Handle both forward and back slashes for cross-platform support
	lastSep := -1
	for i := len(path) - 1; i >= 0; i-- {
		c := path[i]
		if c == '/' || c == '\\' {
			lastSep = i
			break
		}
	}
	if lastSep >= 0 {
		return path[lastSep+1:]
	}
	return path
}

// smallInts caches string representations of small integers (0-99)
// to avoid repeated strconv.FormatInt calls for line numbers.
var smallInts = [100]string{
	"0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
	"10", "11", "12", "13", "14", "15", "16", "17", "18", "19",
	"20", "21", "22", "23", "24", "25", "26", "27", "28", "29",
	"30", "31", "32", "33", "34", "35", "36", "37", "38", "39",
	"40", "41", "42", "43", "44", "45", "46", "47", "48", "49",
	"50", "51", "52", "53", "54", "55", "56", "57", "58", "59",
	"60", "61", "62", "63", "64", "65", "66", "67", "68", "69",
	"70", "71", "72", "73", "74", "75", "76", "77", "78", "79",
	"80", "81", "82", "83", "84", "85", "86", "87", "88", "89",
	"90", "91", "92", "93", "94", "95", "96", "97", "98", "99",
}

// FormatInt formats an integer to string using cached values for small integers.
func FormatInt(n int) string {
	if n >= 0 && n < 100 {
		return smallInts[n]
	}
	return strconv.FormatInt(int64(n), 10)
}
