package dd

import (
	"context"
	"fmt"
	"hash/maphash"
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
	return newSensitiveDataFilterWithPatterns(combined, defaultFilterTimeout)
}

// SensitiveDataFilter filters sensitive data from log messages using configurable regex patterns.
// It provides thread-safe filtering with caching, timeout protection, and concurrency limiting.
type SensitiveDataFilter struct {
	// patternsPtr stores an immutable slice of patterns using atomic pointer.
	// This eliminates slice copying during filter operations (hot path).
	// The slice is replaced atomically when patterns are added/removed.
	patternsPtr    atomic.Pointer[[]*regexp.Regexp]
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
func (f *SensitiveDataFilter) hashString(s string) uint64 {
	var h maphash.Hash
	h.SetSeed(f.hashSeed)
	h.WriteString(s)
	return h.Sum64()
}

// newSensitiveDataFilterWithPatterns is the internal constructor for SensitiveDataFilter.
// It creates a filter with the specified patterns and timeout.
func newSensitiveDataFilterWithPatterns(patterns []*regexp.Regexp, timeout time.Duration) *SensitiveDataFilter {
	filter := &SensitiveDataFilter{
		maxInputLength: maxInputLength,
		timeout:        timeout,
		semaphore:      make(chan struct{}, maxConcurrentFilters),
		cache:          make(map[uint64]filterCacheEntry),
		maxCacheSz:     1000, // Maximum cache entries
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

	return filter
}

// NewSensitiveDataFilter creates a sensitive data filter with all built-in patterns.
// This includes patterns for passwords, API keys, credit cards, SSN, emails, and more.
func NewSensitiveDataFilter() *SensitiveDataFilter {
	internal.InitPatterns()
	return newSensitiveDataFilterWithPatterns(internal.CompiledFullPatterns, defaultFilterTimeout)
}

// NewEmptySensitiveDataFilter creates a sensitive data filter with no patterns.
// Use AddPattern() to add custom patterns.
func NewEmptySensitiveDataFilter() *SensitiveDataFilter {
	return newSensitiveDataFilterWithPatterns(nil, emptyFilterTimeout)
}

// NewCustomSensitiveDataFilter creates a sensitive data filter with custom regex patterns.
// Patterns are validated for ReDoS safety before being added.
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
	f.mu.Lock()
	defer f.mu.Unlock()
	emptyPatterns := make([]*regexp.Regexp, 0)
	f.patternsPtr.Store(&emptyPatterns)
	f.patternCount.Store(0)
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
// Returns true if all goroutines completed, false if timeout was reached.
func (f *SensitiveDataFilter) WaitForGoroutines(timeout time.Duration) bool {
	if f == nil {
		return true
	}

	// Fast path: no active goroutines
	if f.activeGoroutines.Load() == 0 {
		return true
	}

	// Use a channel to implement timeout on Cond.Wait
	done := make(chan struct{})

	go func() {
		f.goroutineCond.L.Lock()
		defer f.goroutineCond.L.Unlock()
		for f.activeGoroutines.Load() > 0 && !f.closed.Load() {
			f.goroutineCond.Wait()
		}
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		// Timeout reached: mark closed so the waiting goroutine exits its loop,
		// then broadcast to wake it from Cond.Wait().
		f.closed.Store(true)
		f.goroutineCond.Broadcast()
		return f.activeGoroutines.Load() == 0
	}
}

// Close marks the filter as closed and waits for active goroutines to complete.
// After calling Close, the Filter method will return input unchanged without
// spawning new goroutines. This prevents goroutine leaks during shutdown.
//
// IMPORTANT: Always call Close (or WaitForGoroutines) before program exit to
// ensure all background goroutines complete gracefully.
//
// Returns true if all goroutines completed within the timeout, false otherwise.
func (f *SensitiveDataFilter) Close() bool {
	if f == nil {
		return true
	}

	f.closed.Store(true)
	return f.WaitForGoroutines(defaultFilterTimeout * 2)
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
		hashSeed:       f.hashSeed, // Share the same seed (read-only after initialization)
		goroutineCond:  *sync.NewCond(&sync.Mutex{}),
	}
	clone.enabled.Store(f.enabled.Load())

	// Share the patterns pointer directly (immutable after creation)
	// This avoids allocation when cloning
	clone.patternsPtr.Store(f.patternsPtr.Load())
	clone.patternCount.Store(f.patternCount.Load())

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

	// Pre-compute hash for cache operations (avoid redundant hash calculations)
	// This hash is for the original input; will recompute if input is truncated
	// SECURITY: Only cache inputs <= cacheInputMaxLen (128 bytes) to prevent
	// hash collision attacks. See cacheResult for details.
	var inputHash uint64
	useCache := inputLen <= cacheInputMaxLen

	// Check cache for repeated messages (only for small inputs to avoid memory bloat)
	// Skip cache if not initialized (for filters created without using constructor)
	if useCache {
		inputHash = f.hashString(input)
		f.cacheMu.RLock()
		if f.cache != nil {
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

	// Track if input was truncated for cache decision
	// SECURITY: When input is truncated, the content changes so we must
	// disable caching to prevent cache pollution with stale results
	inputWasTruncated := false

	startTime := time.Now()

	patterns := *patternsPtr
	timeout := f.timeout

	// Handle truncation with boundary-aware sensitive data detection FIRST.
	// This prevents sensitive data patterns that span the truncation boundary
	// from being leaked, regardless of couldContainSensitiveData result.
	// IMPORTANT: Boundary check must happen before early exit to prevent
	// sensitive data leakage at truncation boundaries.
	if f.maxInputLength > 0 && inputLen > f.maxInputLength {
		// Check the boundary region for sensitive data before truncating
		boundaryStart := f.maxInputLength - boundaryCheckSize
		if boundaryStart < 0 {
			boundaryStart = 0
		}
		boundaryRegion := input[boundaryStart:]

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

		// Cache the result for small inputs (use pre-computed hash)
		if useCache && f.cache != nil {
			f.cacheResult(inputHash, input, input)
		}
		return input
	}

	result := input
	redactionCount := int64(0)
	for i := range patterns {
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
	if useCache && f.cache != nil {
		f.cacheResult(inputHash, input, result)
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

// cacheResult stores a filter result in the cache.
// For inputs longer than cacheInputMaxLen, the input string is not stored
// to prevent memory bloat from caching large strings.
//
// SECURITY: For inputs longer than cacheInputMaxLen, we skip caching entirely
// to prevent hash collision attacks that could bypass sensitive data filtering.
func (f *SensitiveDataFilter) cacheResult(hash uint64, input, result string) {
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
		now := time.Now()
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
		created: time.Now(),
	}
}

// Pre-computed lowercase credential keywords for fast case-insensitive matching
// These are the most common credential keywords that appear in sensitive data patterns
var credentialKeywords = [][]byte{
	[]byte("password"),
	[]byte("passwd"),
	[]byte("secret"),
	[]byte("token"),
	[]byte("api_key"),
	[]byte("apikey"),
	[]byte("bearer"),
	[]byte("auth"),
	[]byte("credential"),
	[]byte("private_key"),
	[]byte("session"),
}

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
// SECURITY NOTE: This pre-check is an optimization, not a security boundary.
// The actual regex patterns will still catch sensitive data even if this
// function returns false. Attackers cannot bypass filtering by encoding
// data (e.g., fullwidth digits, HTML entities) because the regex patterns
// operate on the raw input.
//
// Returns true if any sensitive data pattern could potentially match.
func (f *SensitiveDataFilter) couldContainSensitiveData(input string) bool {
	inputLen := len(input)

	// Track what characteristics the input has
	hasDigits := false
	hasAtSign := false
	hasProtocol := false
	hasCredentialKeyword := false
	hasAPIKeyPrefix := false

	// Quick scan for key characteristics
	// Use byte-by-byte scanning for efficiency
	for i := 0; i < inputLen; i++ {
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

		// Early exit if we found all characteristics
		if hasDigits && hasAtSign && hasProtocol {
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

	// Check for API key prefixes (case-sensitive for efficiency)
	// These are the most common API key prefixes
	if !hasAPIKeyPrefix {
		// Check for common prefixes without allocation
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
			hasAPIKeyPrefix = true
		}
	}

	// Fast return: API key prefix found, no need for further checks.
	if hasAPIKeyPrefix {
		return true
	}

	// Check for credential keywords using case-insensitive byte comparison
	// This avoids strings.ToLower allocation
	if !hasCredentialKeyword && inputLen >= 4 {
		hasCredentialKeyword = containsCredentialKeyword(input)
	}

	// Fast return: credential keyword found.
	if hasCredentialKeyword {
		return true
	}

	// Check for base64-like patterns (common for tokens, keys, certificates)
	// Look for sequences of base64 characters (A-Z, a-z, 0-9, +, /, =)
	hasBase64Pattern := false
	if inputLen >= 20 {
		// Only check if we haven't found other indicators
		// Look for a sequence of at least 16 consecutive base64 chars
		base64Run := 0
		for i := 0; i < inputLen; i++ {
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

	// If input has none of the characteristics, it's very unlikely to contain sensitive data
	// Most patterns require at least one of these characteristics
	return hasDigits || hasAtSign || hasProtocol || hasCredentialKeyword || hasAPIKeyPrefix || hasBase64Pattern
}

// containsCredentialKeyword checks if input contains any credential keyword.
// Uses first-byte filter to skip positions that cannot match, reducing comparisons.
func containsCredentialKeyword(input string) bool {
	inputLen := len(input)
	if inputLen < 4 {
		return false
	}

	// For each keyword, use first-byte filtering to quickly skip non-matching positions.
	// This reduces the inner loop iterations from O(N*M) toward O(N) in practice,
	// since most positions are skipped by the first-byte check.
	for _, keyword := range credentialKeywords {
		keywordLen := len(keyword)
		if inputLen < keywordLen {
			continue
		}

		// Get the first byte of keyword for quick filtering
		firstByte := keyword[0]
		firstByteUpper := firstByte - 32 // uppercase equivalent (keyword bytes are lowercase)

		// Search for keyword in input using first-byte filter
		endPos := inputLen - keywordLen
		for i := 0; i <= endPos; i++ {
			c := input[i]
			// Quick first-byte filter: skip if first char can't match
			if c != firstByte && c != firstByteUpper {
				continue
			}

			// First byte matches, check remaining bytes
			match := true
			for j := 1; j < keywordLen; j++ {
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

// filterWithTimeout applies regex filtering with timeout protection for large inputs.
//
// The function uses a tiered approach based on input size:
// - Small inputs (< fastPathThreshold): Direct synchronous processing
// - Medium inputs (< filterMediumInputThreshold): Synchronous chunked processing with context
// - Large inputs: Async processing with timeout and semaphore-based concurrency limiting
//
// For large inputs, a goroutine is spawned for regex processing. The context is passed
// to allow early termination on timeout. The semaphore limits concurrent goroutines
// to prevent resource exhaustion.
func (f *SensitiveDataFilter) filterWithTimeout(input string, pattern *regexp.Regexp, timeout time.Duration) string {
	inputLen := len(input)

	// Fast path for small inputs
	if inputLen < fastPathThreshold {
		return f.replaceWithPattern(input, pattern)
	}

	// For medium inputs, use synchronous chunked processing (no goroutine overhead)
	if inputLen < filterMediumInputThreshold {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		result := f.filterInChunksWithContext(ctx, input, pattern)
		// Check if context timed out
		if ctx.Err() == context.DeadlineExceeded {
			f.totalTimeouts.Add(1)
		}
		return result
	}

	// Try to acquire semaphore with timeout to limit concurrent goroutines
	select {
	case f.semaphore <- struct{}{}:
		defer func() { <-f.semaphore }()
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
		defer func() {
			if f.activeGoroutines.Add(-1) == 0 {
				// Signal any waiting goroutines that count reached zero
				f.goroutineCond.Broadcast()
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

		output := f.filterInChunksWithContext(ctx, input, pattern)
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
		return "[REDACTED]"
	}
}

// filterInChunksWithContext processes input in chunks with context support for early termination.
// Uses overlapping chunks to ensure sensitive data patterns spanning chunk boundaries are detected.
func (f *SensitiveDataFilter) filterInChunksWithContext(ctx context.Context, input string, pattern *regexp.Regexp) string {
	inputLen := len(input)

	// For inputs up to a reasonable size, process directly without chunking
	// This avoids the complexity of reassembling filtered chunks with different lengths
	if inputLen <= filterDirectProcessThreshold {
		return f.replaceWithPatternWithContext(ctx, input, pattern)
	}

	// For very large inputs, use chunked processing with overlap for boundary detection
	overlap := chunkOverlapSize

	var result strings.Builder
	result.Grow(inputLen)

	// Calculate effective step (chunk size minus overlap)
	step := filterChunkSize - overlap
	if step <= 0 {
		step = filterChunkSize / 2
		if step == 0 {
			step = 1
		}
	}

	lastWritten := 0

	for pos := 0; pos < inputLen; pos += step {
		// Check context at the start of each iteration for early termination
		select {
		case <-ctx.Done():
			// Write remaining unprocessed input
			if lastWritten < inputLen {
				result.WriteString(input[lastWritten:])
			}
			return result.String()
		default:
		}

		chunkStart := pos
		chunkEnd := min(pos+filterChunkSize, inputLen)
		chunk := input[chunkStart:chunkEnd]

		// Filter the current chunk with context awareness
		filtered := f.replaceWithPatternWithContext(ctx, chunk, pattern)

		// Check if context was cancelled during filtering
		select {
		case <-ctx.Done():
			// Context cancelled, return what we have
			if lastWritten < inputLen {
				result.WriteString(input[lastWritten:])
			}
			return result.String()
		default:
		}

		if pos == 0 {
			// First chunk: write everything
			result.WriteString(filtered)
			lastWritten = chunkEnd
		} else {
			// Subsequent chunks: skip the overlap region that was already written
			// Only write the new portion (from overlap point to chunk end)
			overlapStart := overlap
			if overlapStart < len(filtered) {
				// Simple approach: write the non-overlap portion
				// The overlap ensures boundary patterns are caught in the previous chunk
				result.WriteString(filtered[overlapStart:])
			}
			lastWritten = chunkEnd
		}
	}

	// The overlap-based chunking already covers boundary patterns between chunks.
	// No additional full-string pass is needed — it would double the work for large inputs.
	return result.String()
}

// replaceWithPatternWithContext applies regex replacement with context awareness.
// It checks for context cancellation to allow early termination.
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

// FilterValueRecursive recursively filters sensitive data from nested structures.
// It processes maps, slices, arrays, and structs to filter sensitive values.
// Circular references are detected and replaced with "[CIRCULAR_REFERENCE]".
// Maximum recursion depth is limited to maxRecursionDepth to prevent stack overflow.
func (f *SensitiveDataFilter) FilterValueRecursive(key string, value any) any {
	// Get pooled visited map to reduce allocations
	visited := visitedMapPool.Get().(map[uintptr]bool)
	defer func() {
		// Clear and return to pool
		clear(visited)
		visitedMapPool.Put(visited)
	}()
	return f.filterValueRecursiveInternal(key, value, visited, 0)
}

// filterValueRecursiveInternal is the internal implementation with circular reference detection.
func (f *SensitiveDataFilter) filterValueRecursiveInternal(key string, value any, visited map[uintptr]bool, depth int) any {
	if f == nil || !f.enabled.Load() {
		return value
	}

	// Check recursion depth to prevent stack overflow on deeply nested structures
	if depth > maxRecursionDepth {
		return "[MAX_DEPTH_EXCEEDED]"
	}

	// Handle nil values
	if value == nil {
		return nil
	}

	// Check if the key itself is sensitive
	if internal.IsSensitiveKey(key) {
		return "[REDACTED]"
	}

	// Handle string values directly
	if str, ok := value.(string); ok {
		return f.Filter(str)
	}

	// Use reflection for complex types
	val := reflect.ValueOf(value)
	if !val.IsValid() {
		return value
	}

	kind := val.Kind()

	// Handle pointers - check for circular references
	if kind == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		ptr := val.Pointer()
		if visited[ptr] {
			return "[CIRCULAR_REFERENCE]"
		}
		visited[ptr] = true
		return f.filterValueRecursiveInternal(key, val.Elem().Interface(), visited, depth+1)
	}

	// Handle interfaces
	if kind == reflect.Interface {
		if val.IsNil() {
			return nil
		}
		return f.filterValueRecursiveInternal(key, val.Elem().Interface(), visited, depth+1)
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
			result[i] = f.filterValueRecursiveInternal("", val.Index(i).Interface(), visited, depth+1)
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
			result[keyStr] = f.filterValueRecursiveInternal(keyStr, mapValue, visited, depth+1)
		}
		return result
	}

	// Handle structs
	if kind == reflect.Struct {
		result := make(map[string]any)
		typ := val.Type()
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			fieldType := typ.Field(i)

			// Skip unexported fields
			if !field.CanInterface() {
				continue
			}

			fieldName := fieldType.Name
			// Check for json tag
			if tag := fieldType.Tag.Get("json"); tag != "" && tag != "-" {
				if commaIdx := strings.Index(tag, ","); commaIdx >= 0 {
					if tagName := tag[:commaIdx]; tagName != "" {
						fieldName = tagName
					}
				} else if tag != "" {
					fieldName = tag
				}
			}

			result[fieldName] = f.filterValueRecursiveInternal(fieldName, field.Interface(), visited, depth+1)
		}
		return result
	}

	// For other types, return as-is
	return value
}

// SecurityConfig configures security features for the logger.
type SecurityConfig struct {
	MaxMessageSize  int
	MaxWriters      int
	SensitiveFilter *SensitiveDataFilter
}

// SecurityLevel defines the security level for the logger.
// Higher levels provide more protection but may impact performance.
type SecurityLevel int

const (
	// SecurityLevelDevelopment provides minimal security for development.
	// - No sensitive data filtering
	// - No rate limiting
	// - No audit logging
	// Use only in local development environments.
	SecurityLevelDevelopment SecurityLevel = iota

	// SecurityLevelBasic provides basic security for non-production environments.
	// - Basic sensitive data filtering (passwords, API keys, credit cards)
	// - No rate limiting
	// - No audit logging
	// Suitable for staging and testing environments.
	SecurityLevelBasic

	// SecurityLevelStandard provides standard security for production.
	// - Full sensitive data filtering
	// - Rate limiting enabled
	// - Basic audit logging
	// Recommended for most production deployments.
	SecurityLevelStandard

	// SecurityLevelStrict provides enhanced security for sensitive environments.
	// - Full sensitive data filtering
	// - Strict rate limiting
	// - Full audit logging
	// - Input sanitization
	// Suitable for environments handling PII or financial data.
	SecurityLevelStrict

	// SecurityLevelParanoid provides maximum security for high-risk environments.
	// - Full sensitive data filtering with all patterns
	// - Very strict rate limiting
	// - Complete audit logging
	// - All input validation
	// - Log integrity verification
	// Use for healthcare (HIPAA), financial (PCI-DSS), or government systems.
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
//   - SensitiveFilter (via SensitiveDataFilter.Clone())
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
	return clone
}

func newBasicSensitiveDataFilter() *SensitiveDataFilter {
	internal.InitPatterns()
	return newSensitiveDataFilterWithPatterns(internal.CompiledBasicPatterns, defaultFilterTimeout)
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
