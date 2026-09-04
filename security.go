package dd

import (
	"context"
	"fmt"
	"hash/maphash"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/dd/internal"
)

// cacheTTLSeconds defines how long cache entries are valid (5 minutes)
const cacheTTLSeconds = 300

// defaultFilterCacheSize is the maximum number of filtered-input results each
// SensitiveDataFilter caches. The cache maps input-hash -> filtered result so
// repeated identical inputs (the common case in logging) skip regex entirely.
// This MUST be initialized in every filter instance, including clones — a
// filter with a nil cache disables caching, forcing every Filter() call to
// re-run all regex patterns. See clone().
const defaultFilterCacheSize = 1000

// visitedMapPool pools visited maps for FilterValueRecursive to reduce allocations
// in the hot path when filtering complex nested structures.
var visitedMapPool = sync.Pool{
	New: func() any {
		return make(map[uintptr]bool, 8) // typical visited capacity
	},
}

// Pre-compiled additional patterns for security levels.
// Compiled at init time to eliminate runtime panics in public API functions.
// These are extra patterns added on top of the base sensitive data filter.
var (
	strictExtraCompiled     []*regexp.Regexp
	paranoidExtraCompiled   []*regexp.Regexp
	healthcareExtraCompiled []*regexp.Regexp
	financialExtraCompiled  []*regexp.Regexp
	governmentExtraCompiled []*regexp.Regexp
)

func init() {
	// Strict mode extra patterns — context-aware confidentiality and internal IDs
	strictExtraCompiled = compileExtraPatterns([]string{
		`(?i)(?:confidential|classified|secret|private)[\s:=]+[^\s]{1,256}\b`,
		`(?i)(?:internal[_-]?id|employee[_-]?id|user[_-]?id)[\s:=]+[A-Za-z0-9]{4,50}\b`,
	})

	// Paranoid mode extra patterns — broad coverage for IDs, financial amounts, UUIDs
	paranoidExtraCompiled = compileExtraPatterns([]string{
		`(?i)(?:confidential|classified|secret|private|restricted)[\s:=]+[^\s]{1,256}\b`,
		`(?i)(?:internal[_-]?id|employee[_-]?id|user[_-]?id|session[_-]?id|transaction[_-]?id|reference[_-]?id|tracking[_-]?id)[\s:=]+[A-Za-z0-9]{4,50}\b`,
		`(?i)(?:amount|balance|deposit|withdrawal|transfer|payment)[\s:=]+[0-9.,]{1,20}\b`,
		`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`,
	})

	// Healthcare extra patterns — HIPAA/PHI identifiers
	healthcareExtraCompiled = compileExtraPatterns([]string{
		`(?i)(?:icd[-_]?10?|diagnosis|diag|dx|diagnostic[_-]?code|clinical[_-]?code)[\s:=]+[A-Z][0-9]{2}(?:\.[0-9A-Z]{1,4})?\b`,
		`(?i)(?:mrn|medical[_-]?record[_-]?number|patient[_-]?id|health[_-]?record)[\s:=]+[A-Za-z0-9]{6,20}\b`,
		`\b[0-9]{9}[A-Z]{1,2}\b`,
		`(?i)(?:patient[_-]?identifier|patient[_-]?code)[\s:=]+[A-Za-z0-9]{6,20}\b`,
	})

	// Financial extra patterns — PCI-DSS identifiers
	financialExtraCompiled = compileExtraPatterns([]string{
		`(?i)(?:swift|bic|bank[_-]?code|iban)[\s:=]+[A-Z]{4}[A-Z]{2}[A-Z0-9]{2}(?:[A-Z0-9]{3})?\b`,
		`\b[A-Z]{2}[0-9]{2}[A-Z0-9]{4}[0-9]{7,30}\b`,
		`(?i)(?:cvv|cvc|cv2|security[_-]?code|card[_-]?verification)[\s:=]+[0-9]{3,4}\b`,
		`(?i)(?:account[_-]?number|bank[_-]?account|acct[_-]?no)[\s:=]+[0-9]{8,17}\b`,
		`(?i)(?:routing[_-]?number|aba|aba[_-]?rn|routing)[\s:=]+[0-9]{9}\b`,
	})

	// Government extra patterns — identity and case identifiers
	governmentExtraCompiled = compileExtraPatterns([]string{
		`(?i)(?:passport[_-]?number|passport[_-]?no|passport[_-]?id)[\s:=]+[0-9]{8,9}\b`,
		`(?i)(?:driver[_-]?license|dl[_-]?number|license[_-]?number|drivers[_-]?license)[\s:=]+[A-Za-z0-9]{5,20}\b`,
		`\b[0-9]{2}-[0-9]{7}\b`,
		`\b[A-CEGHJ-PR-TW-Z][A-CEGHJ-NPR-TW-Z][0-9]{6}[A-D]\b`,
		`\b[0-9]{3}[- ]?[0-9]{3}[- ]?[0-9]{3}\b`,
		`(?i)(?:case[_-]?number|file[_-]?number|docket)[\s:=]+[A-Za-z0-9]{5,20}\b`,
	})
}

// compileExtraPatterns compiles a list of regex patterns for use in security levels.
// Called only at init time — panics here indicate a developer error in hardcoded patterns,
// not a user-triggerable condition. This keeps panics out of public API functions.
func compileExtraPatterns(patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			panic(fmt.Sprintf("dd: internal error: invalid hardcoded pattern %q: %v", p, err))
		}
		compiled = append(compiled, re)
	}
	return compiled
}

// newSensitiveDataFilterWithExtras creates a sensitive data filter with base patterns
// plus additional pre-compiled patterns. This avoids the AddPatterns() error path
// in public API functions, eliminating potential panics.
func newSensitiveDataFilterWithExtras(extras []*regexp.Regexp) *SensitiveDataFilter {
	internal.InitPatterns()
	base := internal.CompiledFullPatterns
	combined := make([]*regexp.Regexp, 0, len(base)+len(extras))
	combined = append(combined, base...)
	combined = append(combined, extras...)
	// Gate slice mirrors the combined pattern slice: built-in gates first,
	// zero gates ("always run") for the caller-supplied extras. The length
	// MUST be len(base)+len(extras) — Filter only applies gating when the two
	// slices are index-aligned, so a shorter slice silently disables gating
	// for every pattern, not just the extras.
	combinedGates := make([]internal.PatternGate, len(base)+len(extras))
	copy(combinedGates, internal.FullPatternGates)
	return newSensitiveDataFilterWithPatterns(combined, combinedGates, defaultFilterTimeout)
}

// SensitiveDataFilter filters sensitive data from log messages using configurable regex patterns.
// It provides thread-safe filtering with caching, timeout protection, and concurrency limiting.
type SensitiveDataFilter struct {
	// patternsPtr stores an immutable slice of patterns using atomic pointer.
	// This eliminates slice copying during filter operations (hot path).
	// The slice is replaced atomically when patterns are added/removed.
	patternsPtr atomic.Pointer[[]*regexp.Regexp]
	// gatesPtr stores a slice of per-pattern gates, index-aligned with the
	// slice in patternsPtr (see internal.ScanMessageFeatures). Custom patterns
	// added via AddPattern carry a zero gate ("always run"), so alignment is
	// maintained by appending one gate per appended pattern.
	gatesPtr       atomic.Pointer[[]internal.PatternGate]
	mu             sync.RWMutex // protects pattern modifications
	maxInputLength int
	timeout        time.Duration
	enabled        atomic.Bool
	closed         atomic.Bool // prevents new goroutines when true
	// semaphore limits concurrent regex filtering goroutines to prevent resource exhaustion
	semaphore chan struct{}
	// activeGoroutines tracks the number of currently running filter goroutines
	activeGoroutines atomic.Int32
	// patternCount caches the number of patterns for O(1) access
	patternCount atomic.Int32

	// Performance monitoring counters
	totalFiltered   atomic.Int64 // Total number of filter operations
	totalRedactions atomic.Int64 // Total number of redactions performed
	totalTimeouts   atomic.Int64 // Total number of timeout events
	totalLatencyNs  atomic.Int64 // Total latency in nanoseconds (for average calculation)

	// Filter result cache for repeated messages
	cacheMu    sync.RWMutex
	cache      map[uint64]filterCacheEntry
	cacheHits  atomic.Int64
	cacheMiss  atomic.Int64
	maxCacheSz int

	// hashSeed is used for maphash-based hashing of cache keys.
	// Initialized during filter creation for better collision resistance.
	hashSeed maphash.Seed

	// goroutineCond is used to signal when activeGoroutines reaches zero,
	// allowing WaitForGoroutines to wait efficiently without busy-waiting.
	goroutineCond sync.Cond
}

// filterCacheEntry stores a cached filter result
type filterCacheEntry struct {
	input   string
	result  string
	created time.Time // creation time for TTL calculation
}

// hashString computes a hash of the input string using maphash.
// This provides better collision resistance than FNV-1a while maintaining
// good performance. Each filter instance uses a unique seed for security.
// maphash.String is the zero-setup form of the Hash/SetSeed/WriteString/Sum64
// sequence, which profiling showed paid measurable setup cost on every
// cacheable filter call.
func (f *SensitiveDataFilter) hashString(s string) uint64 {
	return maphash.String(f.hashSeed, s)
}

// newSensitiveDataFilterWithPatterns is the internal constructor for SensitiveDataFilter.
// It creates a filter with the specified patterns and timeout.
// gates must be index-aligned with patterns (built-in lists) or nil/empty
// (no gating — every pattern always runs).
func newSensitiveDataFilterWithPatterns(patterns []*regexp.Regexp, gates []internal.PatternGate, timeout time.Duration) *SensitiveDataFilter {
	filter := &SensitiveDataFilter{
		maxInputLength: maxInputLength,
		timeout:        timeout,
		semaphore:      make(chan struct{}, maxConcurrentFilters),
		cache:          make(map[uint64]filterCacheEntry),
		maxCacheSz:     defaultFilterCacheSize,
		hashSeed:       maphash.MakeSeed(),
	}
	// Initialize the condition variable with a new mutex
	filter.goroutineCond = *sync.NewCond(&sync.Mutex{})
	filter.enabled.Store(true)

	if patterns != nil {
		copiedPatterns := make([]*regexp.Regexp, len(patterns))
		copy(copiedPatterns, patterns)
		filter.patternsPtr.Store(&copiedPatterns)
		filter.patternCount.Store(int32(len(copiedPatterns)))
	} else {
		emptyPatterns := make([]*regexp.Regexp, 0)
		filter.patternsPtr.Store(&emptyPatterns)
		filter.patternCount.Store(0)
	}

	if gates != nil {
		copiedGates := make([]internal.PatternGate, len(gates))
		copy(copiedGates, gates)
		filter.gatesPtr.Store(&copiedGates)
	} else {
		emptyGates := make([]internal.PatternGate, 0)
		filter.gatesPtr.Store(&emptyGates)
	}

	return filter
}

// NewSensitiveDataFilter creates a sensitive data filter with all built-in patterns.
// This includes patterns for passwords, API keys, credit cards, SSN, emails, and more.
func NewSensitiveDataFilter() *SensitiveDataFilter {
	internal.InitPatterns()
	return newSensitiveDataFilterWithPatterns(internal.CompiledFullPatterns, internal.FullPatternGates, defaultFilterTimeout)
}

// NewEmptySensitiveDataFilter creates a sensitive data filter with no patterns.
// Use AddPattern() to add custom patterns.
func NewEmptySensitiveDataFilter() *SensitiveDataFilter {
	return newSensitiveDataFilterWithPatterns(nil, nil, emptyFilterTimeout)
}

// NewCustomSensitiveDataFilter creates a sensitive data filter with custom regex patterns.
// Patterns are validated for ReDoS safety before being added.
//
// Returns errors:
//   - ErrEmptyPattern: when a pattern is empty
//   - ErrPatternTooLong: when a pattern exceeds 1000 characters
//   - ErrInvalidPattern: when a pattern fails to compile
//   - ErrReDoSPattern: when a pattern contains dangerous nested quantifiers
func NewCustomSensitiveDataFilter(patterns ...string) (*SensitiveDataFilter, error) {
	filter := NewEmptySensitiveDataFilter()

	for _, pattern := range patterns {
		if err := filter.AddPattern(pattern); err != nil {
			return nil, err
		}
	}

	return filter, nil
}

func (f *SensitiveDataFilter) addPattern(pattern string) error {
	if len(pattern) > maxPatternLength {
		return fmt.Errorf("%w: %d exceeds maximum %d", ErrPatternTooLong, len(pattern), maxPatternLength)
	}

	if internal.HasNestedQuantifiers(pattern, maxQuantifierRange) {
		return ErrReDoSPattern
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPattern, err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	currentPatterns := f.patternsPtr.Load()
	newPatterns := make([]*regexp.Regexp, len(*currentPatterns)+1)
	copy(newPatterns, *currentPatterns)
	newPatterns[len(*currentPatterns)] = re
	f.patternsPtr.Store(&newPatterns)
	f.patternCount.Store(int32(len(newPatterns)))

	// Keep the gate slice index-aligned: custom patterns get a zero gate
	// (always run), since gates are only derived for built-in patterns.
	currentGates := f.gatesPtr.Load()
	newGates := make([]internal.PatternGate, len(*currentGates)+1)
	copy(newGates, *currentGates)
	f.gatesPtr.Store(&newGates)

	return nil
}

// AddPattern adds a regex pattern to the filter. Returns ErrNilFilter if receiver is nil,
// ErrEmptyPattern if pattern is empty, or ErrReDoSPattern if the pattern is potentially dangerous.
func (f *SensitiveDataFilter) AddPattern(pattern string) error {
	if f == nil {
		return ErrNilFilter
	}
	if pattern == "" {
		return ErrEmptyPattern
	}
	return f.addPattern(pattern)
}

// AddPatterns adds multiple regex patterns to the filter. Empty patterns are skipped.
func (f *SensitiveDataFilter) AddPatterns(patterns ...string) error {
	if f == nil {
		return ErrNilFilter
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if err := f.addPattern(pattern); err != nil {
			return fmt.Errorf("%w: %q: %w", ErrPatternFailed, pattern, err)
		}
	}
	return nil
}

// ClearPatterns removes all patterns from the filter.
func (f *SensitiveDataFilter) ClearPatterns() {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	emptyPatterns := make([]*regexp.Regexp, 0)
	f.patternsPtr.Store(&emptyPatterns)
	f.patternCount.Store(0)
	emptyGates := make([]internal.PatternGate, 0)
	f.gatesPtr.Store(&emptyGates)
}

// PatternCount returns the number of registered patterns.
func (f *SensitiveDataFilter) PatternCount() int {
	if f == nil {
		return 0
	}
	return int(f.patternCount.Load())
}

// Enable enables the filter.
func (f *SensitiveDataFilter) Enable() {
	if f != nil {
		f.enabled.Store(true)
	}
}

// Disable disables the filter. Filter calls will return input unchanged.
func (f *SensitiveDataFilter) Disable() {
	if f != nil {
		f.enabled.Store(false)
	}
}

// IsEnabled returns whether the filter is currently enabled.
func (f *SensitiveDataFilter) IsEnabled() bool {
	if f == nil {
		return false
	}
	return f.enabled.Load()
}

// ActiveGoroutineCount returns the number of currently active filter goroutines.
// This can be used for monitoring and detecting potential goroutine leaks in
// high-concurrency scenarios. A consistently high count may indicate that
// filter operations are timing out frequently.
func (f *SensitiveDataFilter) ActiveGoroutineCount() int32 {
	if f == nil {
		return 0
	}
	return f.activeGoroutines.Load()
}

// FilterStats holds filter statistics for monitoring and observability.
// This provides a snapshot of the filter's current state for health checks
// and performance monitoring.
type FilterStats struct {
	ActiveGoroutines  int32         // Number of currently running filter goroutines
	PatternCount      int32         // Number of registered sensitive data patterns
	SemaphoreCapacity int           // Maximum concurrent filter operations
	MaxInputLength    int           // Maximum input length before truncation
	Enabled           bool          // Whether filtering is enabled
	TotalFiltered     int64         // Total number of filter operations
	TotalRedactions   int64         // Total number of redactions performed
	TotalTimeouts     int64         // Total number of timeout events
	AverageLatency    time.Duration // Average latency per filter operation
	CacheHits         int64         // Number of cache hits
	CacheMiss         int64         // Number of cache misses
}

// GetFilterStats returns current filter statistics for monitoring.
// This is useful for health checks, metrics collection, and debugging.
//
// Example:
//
//	stats := filter.GetFilterStats()
//	fmt.Printf("Active goroutines: %d\n", stats.ActiveGoroutines)
//	fmt.Printf("Patterns: %d\n", stats.PatternCount)
//	fmt.Printf("Enabled: %v\n", stats.Enabled)
//	fmt.Printf("Total filtered: %d\n", stats.TotalFiltered)
//	fmt.Printf("Average latency: %v\n", stats.AverageLatency)
func (f *SensitiveDataFilter) GetFilterStats() FilterStats {
	if f == nil {
		return FilterStats{
			SemaphoreCapacity: 0,
			MaxInputLength:    0,
			Enabled:           false,
		}
	}

	var avgLatency time.Duration
	totalFiltered := f.totalFiltered.Load()
	if totalFiltered > 0 {
		avgLatency = time.Duration(f.totalLatencyNs.Load() / totalFiltered)
	}

	return FilterStats{
		ActiveGoroutines:  f.activeGoroutines.Load(),
		PatternCount:      f.patternCount.Load(),
		SemaphoreCapacity: cap(f.semaphore),
		MaxInputLength:    f.maxInputLength,
		Enabled:           f.enabled.Load(),
		TotalFiltered:     totalFiltered,
		TotalRedactions:   f.totalRedactions.Load(),
		TotalTimeouts:     f.totalTimeouts.Load(),
		AverageLatency:    avgLatency,
		CacheHits:         f.cacheHits.Load(),
		CacheMiss:         f.cacheMiss.Load(),
	}
}

// WaitForGoroutines waits for all active filter goroutines to complete or until
// the timeout is reached.
//
// IMPORTANT: Call this method before program exit to prevent goroutine leaks.
// In high-concurrency scenarios with large inputs, filter operations may spawn
// background goroutines for regex processing. Failing to wait for these goroutines
// can result in resource leaks and incomplete log filtering.
//
// Recommended usage in shutdown sequence:
//
//	// 1. Stop accepting new log messages
//	// 2. Wait for filter goroutines to complete
//	logger.WaitForFilterGoroutines(5 * time.Second)
//	// 3. Close the logger
//	logger.Close()
//
// A timeout does NOT disable the filter: subsequent Filter calls continue to
// redact as normal.
//
// Returns true if all goroutines completed, false if timeout was reached.
func (f *SensitiveDataFilter) WaitForGoroutines(timeout time.Duration) bool {
	if f == nil {
		return true
	}

	// Fast path: no active goroutines
	if f.activeGoroutines.Load() == 0 {
		return true
	}

	// Per-call abort flag: a timed-out wait must abort only its OWN helper
	// goroutine. A shared flag would let one caller's timeout prematurely wake
	// another concurrent caller's helper, making that caller return true
	// ("all completed") while filter goroutines are still running.
	abort := &atomic.Bool{}

	// Use a channel to implement timeout on Cond.Wait
	done := make(chan struct{})

	go func() {
		f.goroutineCond.L.Lock()
		defer f.goroutineCond.L.Unlock()
		for f.activeGoroutines.Load() > 0 && !f.closed.Load() && !abort.Load() {
			f.goroutineCond.Wait()
		}
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		// Timeout reached: wake the helper goroutine via the per-call abort
		// flag — NOT via closed. Setting closed here would permanently disable
		// filtering for every subsequent Filter call (a transient timeout must
		// never turn into a security off-switch; WaitForGoroutines is
		// documented for use BEFORE Close(), i.e. while logging is still
		// active). Mutate abort under the cond's lock (same discipline as the
		// goroutine-exit path): a helper between its condition check and
		// Cond.Wait would otherwise miss the broadcast and sleep forever —
		// a leaked goroutine, not just a delayed wakeup.
		f.goroutineCond.L.Lock()
		abort.Store(true)
		f.goroutineCond.Broadcast()
		f.goroutineCond.L.Unlock()
		return f.activeGoroutines.Load() == 0
	}
}

// Close marks the filter as closed and waits for active goroutines to complete.
// After calling Close, the Filter method will return input unchanged without
// spawning new goroutines. This prevents goroutine leaks during shutdown.
// Also releases the filter cache to free memory.
//
// IMPORTANT: Always call Close (or WaitForGoroutines) before program exit to
// ensure all background goroutines complete gracefully.
//
// Returns true if all goroutines completed within the timeout, false otherwise.
func (f *SensitiveDataFilter) Close() bool {
	if f == nil {
		return true
	}

	// Mutate closed under the cond's lock and broadcast: a WaitForGoroutines
	// helper between its condition check and Cond.Wait must observe the flag
	// and exit instead of sleeping until the wait timeout below.
	f.goroutineCond.L.Lock()
	f.closed.Store(true)
	f.goroutineCond.Broadcast()
	f.goroutineCond.L.Unlock()

	result := f.WaitForGoroutines(defaultFilterTimeout * 2)

	// Release cache memory
	f.cacheMu.Lock()
	f.cache = nil
	f.cacheMu.Unlock()

	return result
}

// Clone creates a copy of the SensitiveDataFilter.
//
// Shared (immutable):
//   - patterns slice pointer (shared for better performance, patterns are immutable after creation)
//   - hashSeed (shared for consistent hashing)
//
// IMPORTANT: The patterns slice is shared between original and clone.
// This is safe because patterns are immutable after creation.
// DO NOT modify the underlying patterns slice directly.
// Always use AddPattern() method which creates a new slice.
//
// New instances (not shared):
//   - semaphore channel (new channel with same capacity)
//   - cache (new empty cache)
//   - counters (reset to 0)
//
// Returns nil if the receiver is nil.
func (f *SensitiveDataFilter) clone() *SensitiveDataFilter {
	if f == nil {
		return nil
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	clone := &SensitiveDataFilter{
		maxInputLength: f.maxInputLength,
		timeout:        f.timeout,
		semaphore:      make(chan struct{}, maxConcurrentFilters),
		// Initialize a fresh per-instance cache. The cache is intentionally NOT
		// shared with the source filter: each logger owns its own cache, warmed
		// from its own log traffic. Omitting this (leaving cache nil) silently
		// disables caching and forces every Filter() call to re-run all regex
		// patterns — a major hot-path regression.
		cache:         make(map[uint64]filterCacheEntry),
		maxCacheSz:    defaultFilterCacheSize,
		hashSeed:      f.hashSeed, // Share the same seed (read-only after initialization)
		goroutineCond: *sync.NewCond(&sync.Mutex{}),
	}
	clone.enabled.Store(f.enabled.Load())

	// Share the patterns pointer directly (immutable after creation)
	// This avoids allocation when cloning
	clone.patternsPtr.Store(f.patternsPtr.Load())
	clone.patternCount.Store(f.patternCount.Load())
	// Share the gates pointer the same way (immutable, index-aligned)
	clone.gatesPtr.Store(f.gatesPtr.Load())

	return clone
}

// Filter applies all registered patterns to the input string and returns the filtered result.
// Sensitive data is replaced with [REDACTED]. Returns input unchanged if filter is nil or disabled.
func (f *SensitiveDataFilter) Filter(input string) string {
	if f == nil || !f.enabled.Load() || f.closed.Load() {
		return input
	}

	inputLen := len(input)
	if inputLen == 0 {
		return input
	}

	// Fast path: atomic load of patterns pointer (lock-free read)
	patternsPtr := f.patternsPtr.Load()
	if patternsPtr == nil || len(*patternsPtr) == 0 {
		return input
	}

	// Track if input was truncated for cache decision
	// SECURITY: When input is truncated, the content changes so we must
	// disable caching to prevent cache pollution with stale results
	inputWasTruncated := false

	// Pre-compute hash for cache operations (avoid redundant hash calculations)
	// This hash is for the original input; will recompute if input is truncated
	// SECURITY: Only cache inputs <= cacheInputMaxLen (64 bytes) to prevent
	// hash collision attacks. See cacheResult for details.
	var inputHash uint64
	useCache := inputLen <= cacheInputMaxLen

	// Check cache for repeated messages (only for small inputs to avoid memory bloat)
	// Skip cache if not initialized (for filters created without using constructor)
	//
	// The lookup runs BEFORE the quick-rejection scan: repeated identical
	// messages (the common case the cache exists for) return in one hash +
	// RLock + probe, without re-scanning the input at all.
	//
	// cachePresent snapshots (under this read lock) whether a cache map exists.
	// The post-scan cacheResult sites below consult the snapshot instead of
	// re-reading f.cache: an unlocked read there would race with Close()'s
	// f.cache = nil write (Close is the only nil-er and is one-way, so a
	// stale-true snapshot merely routes into cacheResult, which re-checks nil
	// under the write lock).
	cachePresent := false
	if useCache {
		inputHash = f.hashString(input)
		f.cacheMu.RLock()
		if f.cache != nil {
			cachePresent = true
			// SECURITY: Verify both hash AND input length to add collision resistance.
			// This provides defense-in-depth: even if hash collision occurs,
			// different length inputs will be rejected.
			if entry, ok := f.cache[inputHash]; ok && len(entry.input) == inputLen && entry.input == input {
				// SECURITY: Check TTL with 1ms margin to prevent boundary condition issues
				// Entries must be strictly within TTL to be used
				ttlWithMargin := time.Duration(cacheTTLSeconds)*time.Second - time.Millisecond
				if time.Since(entry.created) < ttlWithMargin {
					f.cacheMu.RUnlock()
					f.cacheHits.Add(1)
					f.totalFiltered.Add(1)
					// Record minimal latency for cache hit
					f.totalLatencyNs.Add(1)
					return entry.result
				}
				// Entry expired, will be refreshed below (fall through)
			}
		}
		f.cacheMu.RUnlock()
		f.cacheMiss.Add(1)
	}

	startTime := time.Now()

	patterns := *patternsPtr
	timeout := f.timeout

	// Handle truncation with boundary-aware sensitive data detection FIRST.
	// This prevents sensitive data patterns that span the truncation boundary
	// from being leaked, regardless of couldContainSensitiveData result.
	// IMPORTANT: Boundary check must happen before early exit to prevent
	// sensitive data leakage at truncation boundaries.
	if f.maxInputLength > 0 && inputLen > f.maxInputLength {
		// Check the boundary region for sensitive data before truncating.
		// The region starts boundaryCheckSize before the cut and extends at
		// most boundaryLookahead past it — enough to see any built-in pattern
		// straddling the cut (the longest, a PEM block, matches ~4KB) while
		// keeping the scan bounded. Scanning input[boundaryStart:] to the end
		// (the historical behavior) ran every pattern over the entire
		// discarded tail — unbounded, timeout-free CPU on the logging hot
		// path for oversized messages — and, when the tail matched, appended
		// the whole filtered tail to the "truncated" output, defeating the
		// size limit. Custom patterns able to match farther than
		// boundaryLookahead past the cut are partially retained; keep the
		// lookahead >= the longest registered pattern if that matters.
		boundaryStart := max(f.maxInputLength-boundaryCheckSize, 0)
		boundaryEnd := min(inputLen, f.maxInputLength+boundaryLookahead)
		boundaryRegion := input[boundaryStart:boundaryEnd]

		// Check if boundary region contains any sensitive patterns
		boundaryHasSensitive := false
		for _, pattern := range patterns {
			if pattern.MatchString(boundaryRegion) {
				boundaryHasSensitive = true
				break
			}
		}

		if boundaryHasSensitive {
			// Filter the boundary region separately
			filteredBoundary := boundaryRegion
			for i := range patterns {
				filteredBoundary = f.replaceWithPattern(filteredBoundary, patterns[i])
				if filteredBoundary == "" || filteredBoundary == "[REDACTED]" {
					break
				}
			}
			// Reconstruct: keep the non-boundary part + filtered boundary + truncation marker
			input = input[:boundaryStart] + filteredBoundary + "... [TRUNCATED FOR SECURITY]"
		} else {
			// No sensitive data in boundary, safe to truncate directly
			input = input[:f.maxInputLength] + "... [TRUNCATED FOR SECURITY]"
		}

		inputWasTruncated = true // Track for cache decision
	}

	// SECURITY: Disable caching when input was truncated
	// The content has changed, so caching with the new hash would pollute
	// the cache with results for modified inputs
	// Also explicitly zero the hash to prevent accidental cache pollution
	if inputWasTruncated {
		useCache = false
		inputHash = 0 // SECURITY: Invalidate hash to prevent any cache access
	}

	// Quick rejection: check if input could possibly contain sensitive data
	// This avoids running all regex patterns on obviously safe input
	// Note: Truncation is already handled above
	if !f.couldContainSensitiveData(input) {
		// Still track metrics for monitoring
		// Ensure at least 1ns to avoid zero average latency for very fast operations
		latencyNs := time.Since(startTime).Nanoseconds()
		if latencyNs == 0 {
			latencyNs = 1
		}
		f.totalFiltered.Add(1)
		f.totalLatencyNs.Add(latencyNs)

		// Cache the result for small inputs (use pre-computed hash) so repeated
		// safe messages short-circuit at the cache lookup above.
		if useCache && cachePresent {
			f.cacheResult(inputHash, input, input, startTime)
		}
		return input
	}

	// Pattern gating: extract structural features once and skip patterns whose
	// necessary conditions rule out a match, before running any regex.
	// SECURITY: gates only skip patterns that provably cannot match
	// (see internal.ScanMessageFeatures). The length check guards against a
	// gate slice that is out of alignment with patterns; on mismatch no gate
	// is applied and every pattern runs.
	var gates []internal.PatternGate
	if gatesPtr := f.gatesPtr.Load(); gatesPtr != nil && len(*gatesPtr) == len(patterns) {
		gates = *gatesPtr
	}
	var features internal.MessageFeatures
	if len(gates) > 0 {
		features = internal.ScanMessageFeatures(input)
	}

	result := input
	redactionCount := int64(0)
	for i := range patterns {
		if len(gates) > 0 && !gates[i].Allows(features) {
			continue
		}
		beforeFilter := result
		result = f.filterWithTimeout(result, patterns[i], timeout)
		// Track redactions (result changed by this pattern)
		if result != beforeFilter {
			redactionCount++
		}
		// Early exit if result becomes empty or redacted
		// Note: redactionCount already incremented above when result changed
		if result == "" || result == "[REDACTED]" {
			break
		}
	}

	// Update metrics
	f.totalFiltered.Add(1)
	if redactionCount > 0 {
		f.totalRedactions.Add(redactionCount)
	}
	latencyNs := time.Since(startTime).Nanoseconds()
	f.totalLatencyNs.Add(latencyNs)

	// Cache the result for small inputs (use pre-computed hash)
	if useCache && cachePresent {
		f.cacheResult(inputHash, input, result, startTime)
	}

	return result
}

// cacheInputMaxLen limits the maximum input string length for caching.
// SECURITY: Only inputs <= this length are cached to prevent hash collision attacks.
// Longer inputs bypass the cache entirely, ensuring all sensitive data is filtered.
// This value balances security (collision resistance) with performance (cache hit rate).
// Reduced from 128 to 64 for stronger collision resistance while maintaining
// good cache hit rate for typical short log messages.
const cacheInputMaxLen = 64

// cacheResult stores a filter result in the cache, stamping the entry with
// now (the filter call's start time — close enough for the TTL check and one
// time.Now call cheaper than taking a fresh reading here).
// For inputs longer than cacheInputMaxLen, the input string is not stored
// to prevent memory bloat from caching large strings.
//
// SECURITY: For inputs longer than cacheInputMaxLen, we skip caching entirely
// to prevent hash collision attacks that could bypass sensitive data filtering.
func (f *SensitiveDataFilter) cacheResult(hash uint64, input, result string, now time.Time) {
	f.cacheMu.Lock()
	defer f.cacheMu.Unlock()
	if f.cache == nil {
		return
	}

	// SECURITY: Don't cache long inputs to prevent hash collision attacks.
	// Without storing the full input, we cannot verify collision on cache hit,
	// which could allow an attacker to bypass filtering by crafting collisions.
	if len(input) > cacheInputMaxLen {
		return
	}

	// Check if this is a new entry or an update (handles hash collision case)
	_, exists := f.cache[hash]

	// Evict old entries if cache is full AND this is a new entry
	// Use a batch eviction threshold to reduce O(N) eviction scans:
	// evict when 10% over capacity instead of at exact capacity.
	if !exists && len(f.cache) >= f.maxCacheSz {
		// Simple eviction: clear expired entries first
		ttl := cacheTTLSeconds * time.Second
		for k, entry := range f.cache {
			if now.Sub(entry.created) >= ttl {
				delete(f.cache, k)
			}
		}

		// If still full after removing expired, clear half the cache
		if len(f.cache) >= f.maxCacheSz {
			count := 0
			toDelete := f.maxCacheSz / 2
			for k := range f.cache {
				delete(f.cache, k)
				count++
				if count >= toDelete {
					break
				}
			}
		}
	}

	f.cache[hash] = filterCacheEntry{
		input:   input, // Always store input for collision detection (already checked length)
		result:  result,
		created: now,
	}
}

// Pre-computed lowercase credential keywords for fast case-insensitive matching
// These are the most common credential keywords that appear in sensitive data patterns.
//
// SECURITY: this list must stay a SUPERSET of every keyword that a built-in
// keyword-gated pattern can require when the pattern's value shape contains
// no other pre-gate signal (no digits, '@', "://", '+', API-key prefix, or
// >=16-char base64 run). The password/token patterns match digit-less values
// ("pwd: x"), and so do the swift/mrn/license/connection-string/biometric
// context patterns ("swift: DEUTDEFF", "mrn: ABCDEF12") — for those inputs
// this list is the ONLY way the couldContainSensitiveData hard pre-gate lets
// the regex sweep run at all. A missing keyword here silently disables the
// pattern's redaction (pinned by TestKeywordGatedPatternsReachableThroughPreGate).
//
// Keyword groups deliberately NOT added, because their patterns would then
// false-positive on ordinary digit-free prose (e.g. "host the party",
// "confidential information shared"): kwServerHost (server/host/data source/
// oracle/tns/sid) and the strict/paranoid confidential|classified|private
// extras. See the "known limitations" note on couldContainSensitiveData.
var credentialKeywords = []string{
	"password",
	"passwd",
	"pwd",
	"secret",
	"token",
	"api_key",
	"apikey",
	"api-key",
	"bearer",
	"auth",
	"credential",
	"private_key",
	"session",
	// kwSwift (context SWIFT/BIC/IBAN patterns match letter-only values)
	"swift",
	"bic",
	"iban",
	"bank",
	// kwMrn (medical record / patient identifiers can be letter-only)
	"mrn",
	"medical",
	"patient",
	"health",
	// kwLicense (driver's license values can be letter-only)
	"driver",
	"license",
	"dl",
	// kwConn (azure connection-string values are letter-only secrets)
	"connstr",
	"connection",
	"azure",
	// kwBio (biometric template identifiers can be letter-only)
	"fingerprint",
	"fp",
	"face",
	"biometric",
	"bio",
}

// credentialKeywordIndex indexes credentialKeywords for single-pass scanning
// with two-byte pre-rejection (see internal.SecondByteIndex), so
// containsCredentialKeyword compares only candidates whose first two bytes
// can continue at the current position instead of re-scanning the whole
// input once per keyword.
var credentialKeywordIndex = internal.NewSecondByteIndex(credentialKeywords, true)

// couldContainSensitiveData performs fast pre-checks to determine if input
// could possibly contain sensitive data. This avoids expensive regex matching
// on obviously safe input, providing significant performance improvement.
//
// Checks performed:
//   - Has digits: required for credit cards, SSN, phone numbers, many API keys
//   - Has special prefixes: required for API keys (sk-, ghp_, AKIA, AIza, etc.)
//   - Has credential keywords: required for password/token/secret patterns
//   - Has @ symbol: required for email patterns
//   - Has protocol indicators: required for connection strings
//
// SECURITY NOTE: This pre-check IS a hard gate, not merely an optimization:
// when it returns false, Filter skips all regex patterns and returns the input
// unchanged. Every pattern registered with AddPattern must therefore be
// detectable through at least one characteristic checked here (ASCII or
// fullwidth digits, '@', "://", a known API-key prefix, a credential keyword,
// or a >=16-char base64 run). A custom pattern that can match input containing
// none of these characteristics will NOT be redacted. The fullwidth-digit
// check below exists precisely to keep this invariant from becoming a bypass.
//
// KNOWN LIMITATIONS (built-ins whose only trigger keyword is not in
// credentialKeywords because their arbitrary-token values would false-positive
// on ordinary digit-free prose): server/data source/host (and oracle/tns/sid)
// connection patterns, and the strict/paranoid confidential|classified|private
// extras. Letter-only values for those shapes pass through unredacted; the
// patterns still redact any input that carries another signal (digits,
// scheme, ...).
//
// Returns true if any sensitive data pattern could potentially match.
func (f *SensitiveDataFilter) couldContainSensitiveData(input string) bool {
	inputLen := len(input)

	// Track what characteristics the input has
	hasDigits := false
	hasAtSign := false
	hasProtocol := false

	// Quick scan for key characteristics
	// Use byte-by-byte scanning for efficiency
	for i := range inputLen {
		c := input[i]

		// Check for ASCII digits
		if c >= '0' && c <= '9' {
			hasDigits = true
		}

		// Check for @ (email)
		if c == '@' {
			hasAtSign = true
		}

		// Check for protocol indicators (:)
		if c == ':' && i+2 < inputLen && input[i+1] == '/' && input[i+2] == '/' {
			hasProtocol = true
		}

		// Early exit on the first characteristic found: the result is an OR
		// of the three, so one hit already decides the outcome and the rest
		// of the scan is wasted work (relevant for long messages that start
		// with a digit).
		if hasDigits || hasAtSign || hasProtocol {
			break
		}
	}

	// SECURITY: Also check for encoded digits that might bypass the ASCII check
	// This ensures encoded data doesn't get a free pass through pre-check
	// Fullwidth digits (U+FF10-U+FF19) are encoded as EF BC 90 to EF BC 99
	if !hasDigits && inputLen >= 3 {
		for i := 0; i < inputLen-2; i++ {
			// Check for UTF-8 encoded fullwidth digits: EF BC 9X
			if input[i] == 0xEF && input[i+1] == 0xBC &&
				input[i+2] >= 0x90 && input[i+2] <= 0x99 {
				hasDigits = true
				break
			}
		}
	}

	// Fast return: if byte scan already found a positive indicator, skip expensive checks.
	// The result is OR-ed, so finding any indicator is sufficient.
	if hasDigits || hasAtSign || hasProtocol {
		return true
	}

	// Check for API key prefixes (case-sensitive for efficiency).
	// Fast return on a hit: the result is OR-ed, so no further checks are needed.
	if strings.HasPrefix(input, "sk-") ||
		strings.HasPrefix(input, "ghp_") ||
		strings.HasPrefix(input, "gho_") ||
		strings.HasPrefix(input, "ghu_") ||
		strings.HasPrefix(input, "ghs_") ||
		strings.HasPrefix(input, "ghr_") ||
		strings.HasPrefix(input, "glpat-") ||
		strings.HasPrefix(input, "xox") ||
		strings.Contains(input, "AKIA") ||
		strings.Contains(input, "ASIA") ||
		strings.Contains(input, "AIza") ||
		strings.Contains(input, "ya29.") ||
		strings.Contains(input, "1//") {
		return true
	}

	// Check for credential keywords using case-insensitive byte comparison
	// This avoids strings.ToLower allocation
	if inputLen >= 4 && containsCredentialKeyword(input) {
		return true
	}

	// Check for base64-like patterns (common for tokens, keys, certificates)
	// Look for sequences of base64 characters (A-Z, a-z, 0-9, +, /, =)
	hasBase64Pattern := false
	if inputLen >= 20 {
		// Look for a sequence of at least 16 consecutive base64 chars
		base64Run := 0
		for i := range inputLen {
			c := input[i]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
				(c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=' {
				base64Run++
				if base64Run >= 16 {
					hasBase64Pattern = true
					break
				}
			} else {
				base64Run = 0
			}
		}
	}

	// If input has none of the characteristics, it's very unlikely to contain sensitive data.
	// The early returns above have already ruled out digits, '@', protocols, and API-key
	// prefixes, so at this point only the base64-pattern signal can still be true.
	return hasBase64Pattern
}

// containsCredentialKeyword checks if input contains any credential keyword.
// Single pass over the input; at each position only keywords whose first two
// bytes can continue there (via credentialKeywordIndex) are compared,
// instead of scanning the whole input once per keyword.
func containsCredentialKeyword(input string) bool {
	inputLen := len(input)
	if inputLen < 4 {
		return false
	}

	// Every keyword is at least two bytes, so the last byte can never start one.
	for i := 0; i+1 < inputLen; i++ {
		cands := credentialKeywordIndex.Candidates(input[i], input[i+1])
		for _, ci := range cands {
			keyword := credentialKeywords[ci]
			end := i + len(keyword)
			if end > inputLen {
				continue
			}
			// First two bytes already matched via the index; check the rest
			// case-insensitively (keyword bytes are lowercase).
			match := true
			for j := 2; j < len(keyword); j++ {
				c := input[i+j]
				if c >= 'A' && c <= 'Z' {
					c += 32
				}
				if c != keyword[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

// Filter deadline scaling for large inputs.
//
// Rationale: Go's regexp engine is linear in input size, so scan time grows
// with the message. A FIXED wall-clock budget disproportionately punishes
// large-but-legitimate inputs: measured, one built-in pattern (the 10-keyword
// database-URL alternation) takes ~13ms on a 68KB input in production — i.e.
// ~50ms at the 256KB truncation cap, exactly the old base timeout — and ~250ms
// under the race detector. A false-positive timeout silently replaces the
// ENTIRE message with [REDACTED], destroying both the content and every prior
// redaction, and makes Filter's output timing-dependent. Scaling the deadline
// with input length keeps a hard ceiling for pathological inputs while
// removing that cliff.
const (
	// filterTimeoutScaleBytes is the input-size increment that adds
	// filterTimeoutScaleFactor base timeouts to the effective deadline.
	filterTimeoutScaleBytes = filterDirectProcessThreshold // 32KB
	// filterTimeoutScaleFactor multiplies the per-increment allowance. The 4x
	// headroom absorbs detector/compiler/machine variance (race detector alone
	// measured ~19x slowdown on the slowest pattern) while still bounding a
	// single pattern's runtime.
	filterTimeoutScaleFactor = 4
)

// scaledFilterTimeout returns the effective timeout for an input of the given
// length: the base timeout for inputs within the direct-process threshold,
// then filterTimeoutScaleFactor additional base timeouts per
// filterTimeoutScaleBytes of input.
func scaledFilterTimeout(base time.Duration, inputLen int) time.Duration {
	if inputLen <= filterTimeoutScaleBytes {
		return base
	}
	extra := base * filterTimeoutScaleFactor * time.Duration(inputLen/filterTimeoutScaleBytes)
	return base + extra
}

// filterWithTimeout applies regex filtering with timeout protection for large inputs.
//
// The function uses a tiered approach based on input size:
//   - Small inputs (< fastPathThreshold): Direct synchronous processing
//   - Medium inputs (< filterMediumInputThreshold): Synchronous processing with
//     a cancellation-aware match walk
//   - Large inputs: Async processing with timeout and semaphore-based concurrency limiting
//
// For large inputs, a goroutine is spawned for regex processing. The context
// bounds how long the CALLER waits: on deadline the caller abandons the scan
// and returns "[REDACTED]", but the goroutine itself runs to completion — Go's
// regexp engine cannot be interrupted mid-scan. Patterns are validated against
// catastrophic backtracking (ReDoS) at AddPattern time, so an abandoned scan
// is wasted CPU, not an unbounded one. The semaphore limits concurrent scan
// goroutines to prevent resource exhaustion.
func (f *SensitiveDataFilter) filterWithTimeout(input string, pattern *regexp.Regexp, timeout time.Duration) string {
	inputLen := len(input)

	// Fast path for small inputs
	if inputLen < fastPathThreshold {
		return f.replaceWithPattern(input, pattern)
	}

	// For medium inputs, run synchronously with a timeout (no goroutine overhead)
	if inputLen < filterMediumInputThreshold {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		result := f.filterWithContext(ctx, input, pattern)
		// Check if context timed out
		if ctx.Err() == context.DeadlineExceeded {
			f.totalTimeouts.Add(1)
		}
		return result
	}

	// Large inputs get a deadline scaled to their length (see
	// scaledFilterTimeout): the regex scan is linear in input size, so the
	// base timeout alone would false-positive on big legitimate messages.
	timeout = scaledFilterTimeout(timeout, inputLen)

	// Try to acquire semaphore with timeout to limit concurrent goroutines
	select {
	case f.semaphore <- struct{}{}:
	case <-time.After(timeout / 2):
		// Could not acquire semaphore within half the timeout, return [REDACTED] for safety
		f.totalTimeouts.Add(1)
		return "[REDACTED]"
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	type result struct {
		output string
	}
	done := make(chan result, 1)

	f.activeGoroutines.Add(1)
	go func() {
		// SECURITY: the semaphore slot is released HERE, when the regex work
		// actually finishes — not when filterWithTimeout returns. On timeout
		// the caller abandons the wait, but this goroutine keeps scanning to
		// completion (Go regexp cannot be interrupted); a caller-side release
		// would hand the slot to a new worker while this one still burns CPU,
		// letting the worker population grow unboundedly past
		// maxConcurrentFilters under sustained slow inputs — exactly the
		// resource exhaustion the semaphore exists to prevent.
		defer func() { <-f.semaphore }()
		defer func() {
			if f.activeGoroutines.Add(-1) == 0 {
				// Signal any waiting goroutines that count reached zero.
				// Broadcast under the cond's lock: without it, a waiter in
				// WaitForGoroutines that has checked activeGoroutines but not
				// yet entered Cond.Wait would miss this signal and sleep until
				// the wait timeout (correct result, needlessly delayed).
				f.goroutineCond.L.Lock()
				f.goroutineCond.Broadcast()
				f.goroutineCond.L.Unlock()
			}
		}()
		defer func() {
			if r := recover(); r != nil {
				select {
				case done <- result{output: "[REDACTED]"}:
				default:
				}
			}
		}()

		output := f.filterWithContext(ctx, input, pattern)
		select {
		case done <- result{output: output}:
		case <-ctx.Done():
		}
	}()

	select {
	case res := <-done:
		return res.output
	case <-ctx.Done():
		f.totalTimeouts.Add(1)
		if os.Getenv("DD_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "dd: filter timeout after %v on %d-byte input, pattern %q\n", timeout, inputLen, pattern.String())
		}
		return "[REDACTED]"
	}
}

// filterWithContext redacts all matches of pattern in input while remaining
// responsive to ctx cancellation.
//
// This used to be implemented as fixed-size overlapping chunks reassembled by
// byte offset. That reassembly is incorrect whenever redaction changes the
// output length ([REDACTED] differs in length from the matched text): slicing a
// redacted chunk at a fixed byte offset no longer aligns to the original byte
// boundaries, dropping or duplicating bytes — and truncating [REDACTED] tokens —
// around chunk edges for inputs beyond the direct-process threshold.
//
// Instead we locate every match in a single scan and rebuild the string match
// by match. The result is identical to pattern.ReplaceAllString (correct by
// construction), with a periodic ctx check so very large inputs still respect
// the filter timeout. A single bounded scan is acceptable because patterns are
// validated against catastrophic backtracking (ReDoS) at AddPattern time, the
// same assumption the direct-process path relies on for smaller inputs.
func (f *SensitiveDataFilter) filterWithContext(ctx context.Context, input string, pattern *regexp.Regexp) string {
	// Inputs at or below the threshold skip the match-walk setup.
	if len(input) <= filterDirectProcessThreshold {
		return f.replaceWithPatternWithContext(ctx, input, pattern)
	}

	matches := pattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input
	}

	hasSubexp := pattern.NumSubexp() > 0
	var b strings.Builder
	b.Grow(len(input))

	prev := 0
	for i, m := range matches {
		// FindAllStringSubmatchIndex returns ordered, non-overlapping spans, so
		// m[0] >= prev and this copy is always valid. m[0],m[1] is the full
		// match; m[2],m[3] is capture group 1 (when the pattern has one).
		b.WriteString(input[prev:m[0]])
		// "$1[REDACTED]" semantics: keep group 1 when the pattern has one and it
		// participated in this match; otherwise emit just "[REDACTED]".
		if hasSubexp && len(m) >= 4 && m[2] >= 0 {
			b.WriteString(input[m[2]:m[3]])
		}
		b.WriteString("[REDACTED]")
		prev = m[1]

		// Yield to cancellation periodically so huge inputs respect the filter
		// timeout instead of running unbounded to completion.
		if i&0x3FF == 0x3FF {
			select {
			case <-ctx.Done():
				b.WriteString(input[prev:])
				return b.String()
			default:
			}
		}
	}
	b.WriteString(input[prev:])
	return b.String()
}

// replaceWithPatternWithContext applies regex replacement with context awareness.
// It checks for context cancellation to allow early termination.
//
// Note: on cancellation this returns the input UNFILTERED (the pattern is
// skipped), whereas the large-input path in filterWithTimeout returns
// "[REDACTED]" for the whole message on timeout. The two failure modes are
// intentionally different: the medium path is only reachable for inputs under
// filterMediumInputThreshold (10KB), where a full scan stays well inside the
// base timeout, so cancellation here is pathological rather than expected.
func (f *SensitiveDataFilter) replaceWithPatternWithContext(ctx context.Context, input string, pattern *regexp.Regexp) string {
	// Quick context check before expensive regex operation
	select {
	case <-ctx.Done():
		return input // Return unchanged on cancellation
	default:
	}
	return f.replaceWithPattern(input, pattern)
}

func (f *SensitiveDataFilter) replaceWithPattern(input string, pattern *regexp.Regexp) string {
	// Match gate: skip the allocation-heavy ReplaceAllString when the pattern
	// cannot match. ReplaceAllString allocates a result buffer on every call,
	// even when no match is found; MatchString performs the same single regex
	// scan without that allocation. When MatchString reports no match the
	// replacement would return input unchanged anyway, so the output is
	// identical — this is purely an allocation/CPU win on the common
	// non-matching path (most log traffic). Verified via pprof: this was the
	// source of ~97% of allocations on messages that merely "could contain"
	// sensitive data (e.g. any message containing digits triggers all 67
	// patterns, each previously allocating even when nothing matched).
	if !pattern.MatchString(input) {
		return input
	}
	if pattern.NumSubexp() > 0 {
		return pattern.ReplaceAllString(input, "$1[REDACTED]")
	}
	return pattern.ReplaceAllString(input, "[REDACTED]")
}

// FilterFieldValue filters a single field value. If the key is sensitive, returns [REDACTED].
// Non-string values are returned unchanged.
func (f *SensitiveDataFilter) FilterFieldValue(key string, value any) any {
	if f == nil || !f.enabled.Load() {
		return value
	}

	str, ok := value.(string)
	if !ok {
		return value
	}

	if internal.IsSensitiveKey(key) {
		return "[REDACTED]"
	}

	return f.Filter(str)
}

// FilterString filters a single string field value: it redacts the value
// entirely when the key is sensitive, otherwise it applies the message-level
// pattern filter. It is the exact behavior of FilterValueRecursive's string
// branch, exposed separately so the per-field hot path can work with plain
// strings — returning the filtered value inside an `any` would box it into a
// fresh interface allocation per field per log call even when nothing was
// redacted (pprof: ~22% of allocation objects on the structured-logging path).
// The returned string is identical to value when no redaction occurred, so a
// cheap != comparison detects the no-op case.
func (f *SensitiveDataFilter) FilterString(key, value string) string {
	if f == nil || !f.enabled.Load() {
		return value
	}
	if internal.IsSensitiveKey(key) {
		return "[REDACTED]"
	}
	return f.Filter(value)
}

// FilterValueRecursive recursively filters sensitive data from nested structures.
// It processes maps, slices, arrays, and structs to filter sensitive values.
// Circular references are detected and replaced with "[CIRCULAR_REFERENCE]".
// Maximum recursion depth is limited to maxRecursionDepth to prevent stack overflow.
// Total elements are bounded to prevent resource exhaustion from large structures.
func (f *SensitiveDataFilter) FilterValueRecursive(key string, value any) (result any) {
	// Scalar fast path: numeric and boolean values can never match a value
	// pattern (patterns match text shapes), so only the key check applies.
	// Returning them directly skips the visited-map pool round-trip and the
	// reflection dispatch below, which pprof showed was pure overhead for
	// scalar fields on the structured-logging hot path. Behavior is identical
	// to the slow path, which also returns these types unchanged unless the
	// key itself is sensitive.
	switch value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, bool:
		if f == nil || !f.enabled.Load() {
			return value
		}
		if internal.IsSensitiveKey(key) {
			return "[REDACTED]"
		}
		return value
	}

	// SEC-003: Recover from panics in reflection-based recursive filtering.
	// Return the original value on panic so logging continues without disruption.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "dd: recovered from panic in FilterValueRecursive: %v\n", r)
			result = value
		}
	}()

	// Get pooled visited map to reduce allocations
	visited := visitedMapPool.Get().(map[uintptr]bool)
	defer func() {
		// Clear and return to pool
		clear(visited)
		visitedMapPool.Put(visited)
	}()
	remaining := maxFilterElements
	return f.filterValueRecursiveInternal(key, value, visited, 0, &remaining)
}

// filterValueRecursiveInternal is the internal implementation with circular reference detection.
func (f *SensitiveDataFilter) filterValueRecursiveInternal(key string, value any, visited map[uintptr]bool, depth int, remaining *int) any {
	if f == nil || !f.enabled.Load() {
		return value
	}

	// Check recursion depth to prevent stack overflow on deeply nested structures
	if depth > maxRecursionDepth {
		return "[MAX_DEPTH_EXCEEDED]"
	}

	// Check total element count to prevent resource exhaustion
	*remaining--
	if *remaining <= 0 {
		return "[FILTER_LIMIT_EXCEEDED]"
	}

	// Handle nil values
	if value == nil {
		return nil
	}

	// Check if the key itself is sensitive
	if internal.IsSensitiveKey(key) {
		return "[REDACTED]"
	}

	// Handle string values directly via the shared string semantics (the key
	// was already checked above, so FilterString's key check is redundant here
	// but keeps one implementation of the string-branch behavior).
	if str, ok := value.(string); ok {
		return f.FilterString(key, str)
	}

	// Use reflection for complex types
	val := reflect.ValueOf(value)
	if !val.IsValid() {
		return value
	}

	kind := val.Kind()

	// Handle pointers - check for circular references
	if kind == reflect.Pointer {
		if val.IsNil() {
			return nil
		}
		ptr := val.Pointer()
		if visited[ptr] {
			return "[CIRCULAR_REFERENCE]"
		}
		visited[ptr] = true
		return f.filterValueRecursiveInternal(key, val.Elem().Interface(), visited, depth+1, remaining)
	}

	// Handle interfaces
	if kind == reflect.Interface {
		if val.IsNil() {
			return nil
		}
		return f.filterValueRecursiveInternal(key, val.Elem().Interface(), visited, depth+1, remaining)
	}

	// Handle slices and arrays
	if kind == reflect.Slice || kind == reflect.Array {
		if val.Len() == 0 {
			if kind == reflect.Slice {
				return []any{}
			}
			return value
		}
		// Check for circular reference in slice pointer
		if kind == reflect.Slice {
			ptr := val.Pointer()
			if visited[ptr] {
				return "[CIRCULAR_REFERENCE]"
			}
			visited[ptr] = true
		}
		result := make([]any, val.Len())
		for i := 0; i < val.Len(); i++ {
			result[i] = f.filterValueRecursiveInternal("", val.Index(i).Interface(), visited, depth+1, remaining)
		}
		return result
	}

	// Handle maps
	if kind == reflect.Map {
		if val.IsNil() {
			return nil
		}
		// Check for circular reference in map pointer
		ptr := val.Pointer()
		if visited[ptr] {
			return "[CIRCULAR_REFERENCE]"
		}
		visited[ptr] = true
		result := make(map[string]any, val.Len())
		for _, mapKey := range val.MapKeys() {
			keyStr := internal.MapKeyToString(mapKey)
			mapValue := val.MapIndex(mapKey).Interface()
			result[keyStr] = f.filterValueRecursiveInternal(keyStr, mapValue, visited, depth+1, remaining)
		}
		return result
	}

	// Handle structs
	if kind == reflect.Struct {
		// Fast path: return well-known types as-is before decomposing into fields.
		// Without this, time.Time (all unexported fields) becomes map[string]any{},
		// and types implementing fmt.Stringer/error get decomposed instead of using
		// their canonical string representation.
		if val.CanInterface() {
			iface := val.Interface()
			switch v := iface.(type) {
			case time.Time:
				return v
			case time.Duration:
				return v
			case fmt.Stringer:
				// SEC-003: user String() method — called through the
				// panic-safe helper. A bare call that panicked would unwind
				// to FilterValueRecursive's coarse recover, whose fallback
				// returns the ENTIRE structure unfiltered (fail-open: even
				// password-keyed fields would bypass redaction). The helper
				// degrades to a placeholder instead.
				return internal.SafeStringerString(v)
			case error:
				if v != nil {
					return internal.SafeErrorString(v)
				}
				return nil
			}
		}

		result := make(map[string]any)
		typ := val.Type()
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			fieldType := typ.Field(i)

			// Skip unexported fields
			if !field.CanInterface() {
				continue
			}

			fieldName := internal.JSONFieldName(fieldType)

			result[fieldName] = f.filterValueRecursiveInternal(fieldName, field.Interface(), visited, depth+1, remaining)
		}
		return result
	}

	// For other types, return as-is
	return value
}

// ============================================================================
// Rate Limiting (public configuration)
// ============================================================================

// RateLimitConfig configures rate limiting for the logger (flood protection):
// a fixed per-second budget for messages and bytes, plus a burst allowance,
// refilled once per wall-clock second. Zero values disable the corresponding
// limit. This is an alias of the internal configuration type so callers can
// construct it directly (matching the JSONOptions alias pattern in config.go).
//
// Example:
//
//	cfg := dd.DefaultConfig()
//	cfg.Security.RateLimitConfig = dd.DefaultRateLimitConfig()
//	cfg.Security.RateLimitConfig.MaxMessagesPerSecond = 1000
type RateLimitConfig = internal.RateLimitConfig

// RateLimitStrategy defines how over-limit log messages are handled.
type RateLimitStrategy = internal.RateLimitStrategy

const (
	// RateLimitStrategyDrop drops messages when the rate limit is exceeded (default).
	RateLimitStrategyDrop = internal.RateLimitStrategyDrop
	// RateLimitStrategySample keeps 1 in SamplingRate messages when rate limited.
	RateLimitStrategySample = internal.RateLimitStrategySample
	// RateLimitStrategyThrottle is accepted for API completeness; the logger
	// never blocks, so it currently behaves like RateLimitStrategyDrop.
	RateLimitStrategyThrottle = internal.RateLimitStrategyThrottle
)

// DefaultRateLimitConfig returns a RateLimitConfig with sensible defaults:
// 10,000 messages/second, 10MB/second, a burst allowance of 1,000 messages,
// drop strategy, and a 1-in-100 sampling rate (used by the Sample strategy).
func DefaultRateLimitConfig() *RateLimitConfig {
	return internal.DefaultRateLimitConfig()
}

// SecurityConfig configures security features for the logger.
type SecurityConfig struct {
	// MaxMessageSize is the maximum allowed log message size in bytes.
	// Messages exceeding this limit are truncated. Zero means no limit.
	MaxMessageSize int
	// MaxWriters is the maximum number of output writers allowed.
	// NOTE: this field is currently informational — writer limits are enforced
	// against the fixed package cap (100, see ErrMaxWritersExceeded on
	// Logger.AddWriter), not against this value. Configuring a lower
	// MaxWriters does NOT cause AddWriter to fail earlier.
	MaxWriters int
	// SensitiveFilter is the filter used to redact sensitive data from log output.
	// A nil filter disables sensitive data filtering.
	SensitiveFilter *SensitiveDataFilter
	// RateLimitConfig configures rate limiting to prevent log flooding.
	// A nil value disables rate limiting. Start from DefaultRateLimitConfig()
	// and adjust the fields:
	//
	//	cfg.Security.RateLimitConfig = dd.DefaultRateLimitConfig()
	//	cfg.Security.RateLimitConfig.MaxMessagesPerSecond = 1000
	//
	// When set, over-limit messages are dropped (or sampled) on the log path,
	// and a RATE_LIMIT_EXCEEDED audit event is emitted if audit logging is
	// configured (Config.Audit).
	RateLimitConfig *RateLimitConfig
}

// SecurityLevel defines the security level for the logger.
// Higher levels provide more protection but may impact performance.
type SecurityLevel int

const (
	// SecurityLevelDevelopment provides minimal security for development.
	// SecurityConfigForLevel returns a config with no sensitive data filtering.
	// Use only in local development environments.
	SecurityLevelDevelopment SecurityLevel = iota

	// SecurityLevelBasic provides basic security for non-production environments.
	// SecurityConfigForLevel returns basic sensitive data filtering (passwords,
	// API keys, credit cards). Suitable for staging and testing environments.
	SecurityLevelBasic

	// SecurityLevelStandard provides standard security for production.
	// SecurityConfigForLevel returns the full built-in sensitive data filter
	// (all standard patterns: credentials, keys, cards, emails, IPs, ...).
	//
	// NOTE: the level only selects the sensitive data filter. Rate limiting
	// (SecurityConfig.RateLimitConfig), audit logging (Config.Audit), and log
	// integrity (Config.Audit.IntegritySigner) are separate settings that this
	// preset does not enable — configure them explicitly when needed.
	SecurityLevelStandard

	// SecurityLevelStrict provides enhanced security for sensitive environments.
	// SecurityConfigForLevel returns the full filter plus extra patterns for
	// confidential/internal identifiers. Rate limiting, audit logging, and
	// integrity signing are separate settings (see SecurityLevelStandard NOTE).
	SecurityLevelStrict

	// SecurityLevelParanoid provides maximum security for high-risk environments.
	// SecurityConfigForLevel returns the full filter plus broad extra patterns
	// (IDs, financial amounts, UUIDs). Rate limiting, audit logging, and
	// integrity signing are separate settings (see SecurityLevelStandard NOTE).
	// For healthcare (HIPAA) / financial (PCI-DSS) / government systems, also
	// consider the HealthcareConfig/FinancialConfig/GovernmentConfig presets.
	SecurityLevelParanoid
)

// String returns the string representation of the security level.
func (l SecurityLevel) String() string {
	switch l {
	case SecurityLevelDevelopment:
		return "Development"
	case SecurityLevelBasic:
		return "Basic"
	case SecurityLevelStandard:
		return "Standard"
	case SecurityLevelStrict:
		return "Strict"
	case SecurityLevelParanoid:
		return "Paranoid"
	default:
		return "Unknown"
	}
}

// SecurityConfigForLevel returns a SecurityConfig configured for the specified security level.
// This provides a convenient way to configure security based on deployment environment.
func SecurityConfigForLevel(level SecurityLevel) *SecurityConfig {
	switch level {
	case SecurityLevelDevelopment:
		return &SecurityConfig{
			MaxMessageSize:  maxMessageSize,
			MaxWriters:      maxWriterCount,
			SensitiveFilter: nil, // No filtering
		}

	case SecurityLevelBasic:
		return &SecurityConfig{
			MaxMessageSize:  maxMessageSize,
			MaxWriters:      maxWriterCount,
			SensitiveFilter: newBasicSensitiveDataFilter(),
		}

	case SecurityLevelStandard:
		return &SecurityConfig{
			MaxMessageSize:  maxMessageSize,
			MaxWriters:      maxWriterCount,
			SensitiveFilter: NewSensitiveDataFilter(),
		}

	case SecurityLevelStrict:
		filter := newSensitiveDataFilterWithExtras(strictExtraCompiled)
		return &SecurityConfig{
			MaxMessageSize:  maxMessageSize,
			MaxWriters:      maxWriterCount,
			SensitiveFilter: filter,
		}

	case SecurityLevelParanoid:
		filter := newSensitiveDataFilterWithExtras(paranoidExtraCompiled)
		return &SecurityConfig{
			MaxMessageSize:  maxMessageSize,
			MaxWriters:      maxWriterCount,
			SensitiveFilter: filter,
		}

	default:
		return DefaultSecurityConfig()
	}
}

// Clone creates a copy of the SecurityConfig.
//
// Deep copy:
//   - SensitiveFilter (via SensitiveDataFilter.clone(), unexported)
//
// Returns nil if the receiver is nil.
func (sc *SecurityConfig) Clone() *SecurityConfig {
	if sc == nil {
		return nil
	}

	clone := &SecurityConfig{
		MaxMessageSize: sc.MaxMessageSize,
		MaxWriters:     sc.MaxWriters,
	}
	if sc.SensitiveFilter != nil {
		clone.SensitiveFilter = sc.SensitiveFilter.clone()
	}
	if sc.RateLimitConfig != nil {
		clone.RateLimitConfig = sc.RateLimitConfig.Clone()
	}
	return clone
}

func newBasicSensitiveDataFilter() *SensitiveDataFilter {
	internal.InitPatterns()
	return newSensitiveDataFilterWithPatterns(internal.CompiledBasicPatterns, internal.BasicPatternGates, defaultFilterTimeout)
}

// DefaultSecurityConfig returns a security config with basic sensitive data filtering enabled.
// This provides out-of-the-box protection for common sensitive data like passwords,
// API keys, credit cards, and phone numbers.
//
// This is the recommended default for production use. For development environments
// where performance is critical and data sensitivity is low, consider using
// SecurityConfigForLevel(SecurityLevelDevelopment) instead.
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		MaxMessageSize:  maxMessageSize,
		MaxWriters:      maxWriterCount,
		SensitiveFilter: newBasicSensitiveDataFilter(),
	}
}

// DefaultSecureConfig returns a security config with full sensitive data filtering enabled.
// This includes all patterns from basic filtering plus additional patterns for
// emails, IP addresses, JWT tokens, and database connection strings.
// Use this for maximum security in production environments.
func DefaultSecureConfig() *SecurityConfig {
	return &SecurityConfig{
		MaxMessageSize:  maxMessageSize,
		MaxWriters:      maxWriterCount,
		SensitiveFilter: NewSensitiveDataFilter(),
	}
}

// HealthcareConfig returns a security config optimized for HIPAA compliance.
// This includes all patterns from DefaultSecureConfig plus healthcare-specific patterns:
//   - ICD-10 diagnosis codes (with medical context)
//   - US National Provider Identifier (NPI)
//   - Medical Record Numbers (MRN)
//   - Health Insurance Claim Numbers (HICN)
//
// Use this configuration for applications handling Protected Health Information (PHI)
// in healthcare, medical, and insurance environments.
func HealthcareConfig() *SecurityConfig {
	filter := newSensitiveDataFilterWithExtras(healthcareExtraCompiled)
	return &SecurityConfig{
		MaxMessageSize:  maxMessageSize,
		MaxWriters:      maxWriterCount,
		SensitiveFilter: filter,
	}
}

// FinancialConfig returns a security config optimized for PCI-DSS compliance.
// This includes all patterns from DefaultSecureConfig plus financial-specific patterns:
//   - SWIFT/BIC codes
//   - IBAN (International Bank Account Numbers)
//   - CVV/CVC security codes
//   - Additional card number formats
//
// Use this configuration for applications in banking, payment processing,
// fintech, and other financial services environments.
func FinancialConfig() *SecurityConfig {
	filter := newSensitiveDataFilterWithExtras(financialExtraCompiled)
	return &SecurityConfig{
		MaxMessageSize:  maxMessageSize,
		MaxWriters:      maxWriterCount,
		SensitiveFilter: filter,
	}
}

// GovernmentConfig returns a security config optimized for government and public sector.
// This includes all patterns from DefaultSecureConfig plus government-specific patterns:
//   - US Passport numbers
//   - US Driver's License numbers
//   - US Tax ID / EIN
//   - UK National Insurance Numbers
//   - Canadian Social Insurance Numbers
//
// Use this configuration for applications in government, public sector,
// defense, and regulated identity management environments.
func GovernmentConfig() *SecurityConfig {
	filter := newSensitiveDataFilterWithExtras(governmentExtraCompiled)
	return &SecurityConfig{
		MaxMessageSize:  maxMessageSize,
		MaxWriters:      maxWriterCount,
		SensitiveFilter: filter,
	}
}
