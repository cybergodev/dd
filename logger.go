// Package dd provides a high-performance, thread-safe logging library.
package dd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/cybergodev/dd/internal"
)

// Compile-time interface verification.
// LogProvider covers the full method set; the narrower documented interfaces
// (doc.go) are asserted explicitly so they cannot silently drift apart.
var _ LogProvider = (*Logger)(nil)

var (
	_ CoreLogger         = (*Logger)(nil)
	_ LevelLogger        = (*Logger)(nil)
	_ ConfigurableLogger = (*Logger)(nil)
)

// LogLevel represents the verbosity level for log messages.
// Higher values indicate higher severity. Use LevelDebug through LevelFatal constants.
type LogLevel = internal.LogLevel

const (
	// LevelDebug logs detailed information for debugging during development.
	LevelDebug = internal.LevelDebug
	// LevelInfo logs general operational information about application behavior.
	LevelInfo = internal.LevelInfo
	// LevelWarn logs warning conditions that may indicate potential problems.
	LevelWarn = internal.LevelWarn
	// LevelError logs error conditions that should be investigated.
	LevelError = internal.LevelError
	// LevelFatal logs severe errors followed by program termination via os.Exit(1).
	// WARNING: defer statements will NOT execute after a fatal log.
	LevelFatal = internal.LevelFatal
)

// FatalHandler is called after a fatal-level log message is written.
// Use this to perform custom cleanup before program termination.
// If nil, the default handler calls os.Exit(1).
// If the handler panics, the panic is recovered, logged to stderr, and the
// process exits with status 1 — a Fatal log always terminates the program.
type FatalHandler func()

// WriteErrorHandler is called when a write operation to an io.Writer fails.
// The handler receives the writer that failed and the error that occurred.
// If no handler is set, write errors are silently ignored.
type WriteErrorHandler func(writer io.Writer, err error)

// LevelResolver is a function that determines the effective log level at runtime.
// This allows dynamic log level adjustment based on runtime conditions such as
// system load, error rate, or time of day. The function is called for each log
// entry and for each IsLevelEnabled check, so it should be efficient.
//
// Example:
//
//	// Adjust level based on system load
//	resolver := func(ctx context.Context) LogLevel {
//	    if getCPULoad() > 0.8 {
//	        return LevelWarn  // Only log warnings and above under high load
//	    }
//	    return LevelDebug
//	}
//	logger.SetLevelResolver(resolver)
type LevelResolver func(ctx context.Context) LogLevel

// Flusher is an interface for writers that can flush buffered data.
// Writers implementing this interface will have their Flush method called
// during Logger.Flush() to ensure all buffered data is written.
type Flusher interface {
	Flush() error
}

// Logger is the core logging type that provides thread-safe, structured logging
// with sensitive data filtering, context extraction, hooks, and sampling support.
// Create instances using New() with a Config struct.
//
// All methods are safe for concurrent use across goroutines.
type Logger struct {
	level  atomic.Int32
	closed atomic.Bool

	callerDepth       int
	fatalHandler      FatalHandler
	writeErrorHandler atomic.Pointer[WriteErrorHandler]
	formatter         *internal.MessageFormatter

	// dynamicCaller mirrors Config.DynamicCaller: when false, entry methods
	// skip caller resolution entirely (no stack capture at all).
	dynamicCaller bool

	// levelResolver stores an optional dynamic level resolver function.
	// When set, it is called to determine the effective log level for each entry.
	// If nil or returns LevelDebug, the static level is used.
	levelResolver atomic.Pointer[LevelResolver]

	// fieldValidation stores the field validation configuration.
	// When set, field keys are validated against the configured naming convention.
	fieldValidation atomic.Pointer[FieldValidationConfig]

	// writersPtr stores an immutable slice of writers using atomic pointer.
	// This eliminates slice copying during write operations.
	// The slice is replaced atomically when writers are added/removed.
	writersPtr     atomic.Pointer[[]io.Writer]
	writersMu      sync.Mutex // protects AddWriter/RemoveWriter operations
	securityConfig atomic.Pointer[SecurityConfig]

	// contextExtractors stores the contextExtractorRegistry for extracting
	// fields from context. If nil, default extractors are used.
	contextExtractors atomic.Pointer[contextExtractorRegistry]
	// contextExtractorsMu protects the Clone-Modify-Store sequence in AddContextExtractor
	contextExtractorsMu sync.Mutex

	// hooks stores the HookRegistry for lifecycle hooks.
	hooks atomic.Pointer[HookRegistry]
	// hooksMu protects the Clone-Modify-Store sequence in AddHook to prevent race conditions
	hooksMu sync.Mutex

	// sampling stores the sampling configuration and state.
	sampling atomic.Pointer[samplingState]

	// rateLimiter provides rate limiting to prevent log flooding.
	// Initialized from SecurityConfig.RateLimitConfig when set.
	// Stored in an atomic pointer because SetSecurityConfig can replace it at
	// runtime concurrently with the hot-path reads in shouldLog/logCoreWithDepth;
	// a bare pointer field would be a data race under those concurrent accesses.
	rateLimiter atomic.Pointer[internal.RateLimiter]

	// auditLogger records security events for audit trail.
	// Initialized from Config.Audit when set.
	auditLogger *AuditLogger

	// ctx and cancel provide graceful shutdown for background operations.
	// When Close() is called, cancel() signals all background goroutines
	// (compression, cleanup) to stop. This ensures clean shutdown without
	// leaking goroutines. The context is also used by filter timeout goroutines.
	ctx    context.Context
	cancel context.CancelFunc
}

// samplingState holds the runtime state for log sampling.
type samplingState struct {
	config  *SamplingConfig
	counter atomic.Int64 // Atomic counter for thread-safe increment
	start   time.Time
	startMu sync.Mutex // Only protects start time reset during tick
}

var (
	defaultOutput                    = os.Stdout
	defaultFatalHandler FatalHandler = func() {
		os.Exit(1)
	}
)

// New creates a new Logger with the provided configuration.
// If no configuration is provided, default settings are used.
//
// Example:
//
//	// Simple usage with defaults
//	logger, _ := dd.New()
//	logger.Info("hello")
//
//	// With custom configuration
//	cfg := dd.DefaultConfig()
//	cfg.Level = dd.LevelDebug
//	cfg.Format = dd.FormatJSON
//	logger, _ := dd.New(cfg)
func New(cfg ...Config) (*Logger, error) {
	// Return error if multiple configs provided - this is likely a developer mistake
	if len(cfg) > 1 {
		return nil, fmt.Errorf("%w: %d configs provided, expected 0 or 1", ErrMultipleConfigs, len(cfg))
	}
	if len(cfg) == 0 {
		return defaultConfig().build()
	}
	return cfg[0].build()
}

// newFromInternalConfig creates a Logger from the internal configuration.
func newFromInternalConfig(config *internalConfig) (*Logger, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Pre-allocate writers slice with expected capacity
	initialWriters := make([]io.Writer, 0, len(config.writers))

	// Create formatter config from logger config
	formatterConfig := &internal.FormatterConfig{
		Format:        internal.LogFormat(config.format),
		TimeFormat:    config.timeFormat,
		IncludeTime:   config.includeTime,
		IncludeLevel:  config.includeLevel,
		FullPath:      config.fullPath,
		DynamicCaller: config.dynamicCaller,
		JSON:          config.json,
	}

	l := &Logger{
		callerDepth:   defaultCallerDepth,
		fatalHandler:  config.fatalHandler,
		formatter:     internal.NewMessageFormatter(formatterConfig),
		dynamicCaller: config.dynamicCaller,
		ctx:           ctx,
		cancel:        cancel,
	}

	// Initialize writers pointer with empty slice
	l.writersPtr.Store(&initialWriters)

	if config.writeErrorHandler != nil {
		l.writeErrorHandler.Store(&config.writeErrorHandler)
	}

	l.level.Store(int32(config.level))
	if config.securityConfig != nil {
		l.securityConfig.Store(config.securityConfig.Clone())
		// Initialize rate limiter from security config
		if config.securityConfig.RateLimitConfig != nil {
			l.rateLimiter.Store(internal.NewRateLimiter(config.securityConfig.RateLimitConfig))
		}
	} else {
		l.securityConfig.Store(DefaultSecurityConfig())
	}

	// Initialize field validation (copied so the caller's Config field cannot
	// alias logger state; FieldValidationConfig holds only value fields)
	if config.fieldValidation != nil && config.fieldValidation.Mode != FieldValidationNone {
		fv := *config.fieldValidation
		l.fieldValidation.Store(&fv)
	}

	// Initialize context extractors
	if len(config.contextExtractors) > 0 {
		registry := newContextExtractorRegistry()
		for _, extractor := range config.contextExtractors {
			registry.Add(extractor)
		}
		l.contextExtractors.Store(registry)
	}

	// Initialize hooks
	if config.hooks != nil && config.hooks.count() > 0 {
		l.hooks.Store(config.hooks.clone())
	}

	// Initialize sampling
	if config.sampling != nil && config.sampling.Enabled {
		l.SetSampling(config.sampling)
	}

	// Initialize audit logger.
	// Config.Validate() already rejects invalid audit configs, so this error
	// path is unreachable from New() — it exists as a correctness backstop for
	// direct newFromInternalConfig callers rather than silently disabling audit.
	if config.auditConfig != nil && config.auditConfig.Enabled {
		al, err := newAuditLoggerWithConfig(*config.auditConfig)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to initialize audit logger: %w", err)
		}
		l.auditLogger = al
	}

	if config.writers != nil {
		for _, writer := range config.writers {
			if err := l.AddWriter(writer); err != nil {
				cancel()
				// Close writers already added before failing: they own real
				// resources (file handles) that would otherwise leak for the
				// lifetime of the process. Best-effort — teardown cannot act
				// on individual close errors.
				if current := l.writersPtr.Load(); current != nil {
					for _, w := range *current {
						_ = closeWriter(w) // best-effort cleanup
					}
				}
				return nil, fmt.Errorf("failed to add writer: %w", err)
			}
		}
	}

	return l, nil
}

// SetWriteErrorHandler sets a callback for handling write errors (thread-safe).
// When a write operation fails, the handler is called with the writer and error.
// If no handler is set, write errors are silently ignored.
func (l *Logger) SetWriteErrorHandler(handler WriteErrorHandler) {
	if handler != nil {
		l.writeErrorHandler.Store(&handler)
	} else {
		l.writeErrorHandler.Store(nil)
	}
}

// getWriteErrorHandler returns the current write error handler (thread-safe).
func (l *Logger) getWriteErrorHandler() WriteErrorHandler {
	if p := l.writeErrorHandler.Load(); p != nil {
		return *p
	}
	return nil
}

// loadHooks returns the current hook registry or nil if no hooks are set.
func (l *Logger) loadHooks() *HookRegistry {
	return l.hooks.Load()
}

// loadContextExtractors returns the current context extractor registry or nil.
func (l *Logger) loadContextExtractors() *contextExtractorRegistry {
	return l.contextExtractors.Load()
}

// loadSamplingState returns the current sampling state or nil.
func (l *Logger) loadSamplingState() *samplingState {
	return l.sampling.Load()
}

// shouldLog checks if a message should be logged based on level and logger state.
// When a LevelResolver is set, it determines the effective level dynamically.
// Otherwise, the static level is used.
func (l *Logger) shouldLog(level LogLevel) bool {
	if level < l.effectiveLevel() || level > LevelFatal {
		return false
	}
	if l.closed.Load() {
		return false
	}
	// Message-count rate gate (pre-format). Fatal bypasses rate limiting so a
	// fatal message is never silently dropped (the program must still exit).
	// Byte limiting is applied post-format in logCoreWithDepth, where the
	// message size is known — applying it here with size 0 would leave
	// MaxBytesPerSecond inert.
	if level != LevelFatal {
		// Load the rate limiter once; it may be replaced concurrently by
		// SetSecurityConfig, so the bare field must not be read directly.
		if rl := l.rateLimiter.Load(); rl != nil && !rl.AllowMessage() {
			// Emit audit event for rate limit
			if l.auditLogger != nil {
				l.auditLogger.LogRateLimitExceeded("log message rate limited", nil)
			}
			return false
		}
	}
	return l.shouldSample()
}

// effectiveLevel returns the level gate currently in effect: the dynamic
// resolver's value when one is installed (panic-safe; falls back to the static
// level if the resolver panics), otherwise the static level. Shared by
// shouldLog and IsLevelEnabled so the actual gate and the public predicate can
// never disagree about whether a level passes.
func (l *Logger) effectiveLevel() LogLevel {
	if resolver := l.getLevelResolver(); resolver != nil {
		return l.safeResolveLevel(resolver)
	}
	return LogLevel(l.level.Load())
}

// safeResolveLevel calls a user LevelResolver with panic recovery, falling back
// to the static level when the resolver panics.
//
// SEC-003: shouldLog and IsLevelEnabled run BEFORE logCoreWithDepth's deferred
// recover, so without this backstop a panicking resolver would escape the
// public logging methods and crash the caller.
func (l *Logger) safeResolveLevel(resolver LevelResolver) (level LogLevel) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "dd: level resolver panic (using static level): %v\n", r)
			level = LogLevel(l.level.Load())
		}
	}()
	// Use context.TODO() as public log methods don't accept context.
	// The resolver should handle nil-appropriate context gracefully.
	return resolver(context.TODO())
}

// ============================================================================
// Level Methods
// ============================================================================

// GetLevel returns the current log level (thread-safe).
func (l *Logger) GetLevel() LogLevel {
	return LogLevel(l.level.Load())
}

// SetLevel atomically sets the log level (thread-safe).
func (l *Logger) SetLevel(level LogLevel) error {
	if level < LevelDebug || level > LevelFatal {
		return ErrInvalidLevel
	}
	l.level.Store(int32(level))
	return nil
}

// IsLevelEnabled checks if logging is enabled for the given level (thread-safe).
// Returns true if a log call at this level would pass the level gate.
//
// When a LevelResolver is set, it determines the effective level here too — not
// only on actual log calls — so IsLevelEnabled stays consistent with the
// logger's real gating. A resolver must therefore expect more frequent calls,
// and its output may make the result change between calls. Levels above
// LevelFatal are reported as disabled, matching the gate.
//
// Example:
//
//	if logger.IsLevelEnabled(dd.LevelDebug) {
//	    // Expensive debug computation only when debug is enabled
//	    logger.DebugWith("Details", dd.Any("data", computeExpensiveDebugInfo()))
//	}
func (l *Logger) IsLevelEnabled(level LogLevel) bool {
	return level >= l.effectiveLevel() && level <= LevelFatal
}

// IsDebugEnabled returns true if debug level logging is enabled.
func (l *Logger) IsDebugEnabled() bool {
	return l.IsLevelEnabled(LevelDebug)
}

// IsInfoEnabled returns true if info level logging is enabled.
func (l *Logger) IsInfoEnabled() bool {
	return l.IsLevelEnabled(LevelInfo)
}

// IsWarnEnabled returns true if warn level logging is enabled.
func (l *Logger) IsWarnEnabled() bool {
	return l.IsLevelEnabled(LevelWarn)
}

// IsErrorEnabled returns true if error level logging is enabled.
func (l *Logger) IsErrorEnabled() bool {
	return l.IsLevelEnabled(LevelError)
}

// IsFatalEnabled returns true if fatal level logging is enabled.
func (l *Logger) IsFatalEnabled() bool {
	return l.IsLevelEnabled(LevelFatal)
}

// SetLevelResolver sets a dynamic level resolver function (thread-safe).
// The resolver is called for each log entry to determine the effective log level.
// This allows runtime adjustment of log levels based on conditions like system load,
// error rates, or request context. Set to nil to disable dynamic resolution.
//
// Example:
//
//	// Adaptive logging based on error rate
//	var errorCount atomic.Int64
//	logger.SetLevelResolver(func(ctx context.Context) LogLevel {
//	    if errorCount.Load() > 100 {
//	        return LevelWarn  // Reduce logging under high error rate
//	    }
//	    return LevelDebug
//	})
func (l *Logger) SetLevelResolver(resolver LevelResolver) {
	if resolver == nil {
		l.levelResolver.Store(nil)
	} else {
		l.levelResolver.Store(&resolver)
	}
}

// GetLevelResolver returns the current level resolver function.
// Returns nil if no resolver is set.
func (l *Logger) GetLevelResolver() LevelResolver {
	return l.getLevelResolver()
}

// getLevelResolver safely returns the level resolver function.
func (l *Logger) getLevelResolver() LevelResolver {
	if ptr := l.levelResolver.Load(); ptr != nil {
		return *ptr
	}
	return nil
}

// ============================================================================
// Context Extractor Methods
// ============================================================================

// AddContextExtractor adds a context extractor to the logger (thread-safe).
// Extractors are called in order to extract fields from context during logging.
// If the logger has no extractors, the provided extractor becomes the first one.
// Returns ErrNilExtractor if the extractor is nil, or ErrLoggerClosed if the logger is closed.
func (l *Logger) AddContextExtractor(extractor ContextExtractor) error {
	if extractor == nil {
		return ErrNilExtractor
	}
	if l.closed.Load() {
		return ErrLoggerClosed
	}

	l.contextExtractorsMu.Lock()
	defer l.contextExtractorsMu.Unlock()

	// Load existing registry or create new one
	var registry *contextExtractorRegistry
	if existing := l.loadContextExtractors(); existing != nil {
		registry = existing.clone()
	} else {
		registry = newContextExtractorRegistry()
	}

	registry.Add(extractor)
	l.contextExtractors.Store(registry)
	return nil
}

// SetContextExtractors replaces all context extractors with the provided list (thread-safe).
// Pass no arguments to clear all extractors (which will use default behavior).
// Returns ErrLoggerClosed if the logger is closed.
func (l *Logger) SetContextExtractors(extractors ...ContextExtractor) error {
	if l.closed.Load() {
		return ErrLoggerClosed
	}

	if len(extractors) == 0 {
		// atomic.Value cannot store nil, store an empty registry instead
		l.contextExtractors.Store(newContextExtractorRegistry())
		return nil
	}

	registry := newContextExtractorRegistry()
	for _, extractor := range extractors {
		registry.Add(extractor)
	}
	l.contextExtractors.Store(registry)
	return nil
}

// GetContextExtractors returns a copy of the current context extractors (thread-safe).
// Returns nil if no custom extractors are registered.
func (l *Logger) GetContextExtractors() []ContextExtractor {
	registry := l.loadContextExtractors()
	if registry == nil {
		return nil
	}
	extractorsPtr := registry.extractorsPtr.Load()
	if extractorsPtr == nil {
		return nil
	}
	extractors := make([]ContextExtractor, len(*extractorsPtr))
	copy(extractors, *extractorsPtr)
	return extractors
}

// ============================================================================
// Hook Methods
// ============================================================================

// AddHook registers a hook for a specific event type (thread-safe).
// Hooks are called in order during the logging lifecycle.
// Returns ErrNilHook if the hook is nil, or ErrLoggerClosed if the logger is closed.
func (l *Logger) AddHook(event HookEvent, hook Hook) error {
	if hook == nil {
		return ErrNilHook
	}
	if l.closed.Load() {
		return ErrLoggerClosed
	}

	l.hooksMu.Lock()
	defer l.hooksMu.Unlock()

	// Load existing registry or create new one
	var registry *HookRegistry
	if existing := l.loadHooks(); existing != nil {
		registry = existing.clone()
	} else {
		registry = NewHookRegistry()
	}

	registry.Add(event, hook)
	l.hooks.Store(registry)
	return nil
}

// SetHooks replaces the hook registry with the provided one (thread-safe).
// Pass nil to clear all hooks.
// Returns ErrLoggerClosed if the logger is closed.
func (l *Logger) SetHooks(registry *HookRegistry) error {
	if l.closed.Load() {
		return ErrLoggerClosed
	}

	l.hooksMu.Lock()
	defer l.hooksMu.Unlock()

	if registry == nil {
		// atomic.Value cannot store nil, store an empty registry instead
		l.hooks.Store(NewHookRegistry())
		return nil
	}

	l.hooks.Store(registry.clone())
	return nil
}

// GetHooks returns a copy of the current hook registry (thread-safe).
// Returns nil if no hooks are registered.
func (l *Logger) GetHooks() *HookRegistry {
	if registry := l.loadHooks(); registry != nil {
		return registry.clone()
	}
	return nil
}

// triggerHooks triggers hooks for the given event and context.
// Returns an error if any hook returns an error.
func (l *Logger) triggerHooks(ctx context.Context, hookCtx *HookContext) error {
	if registry := l.loadHooks(); registry != nil {
		return registry.Trigger(ctx, hookCtx.Event, hookCtx)
	}
	return nil
}

// ============================================================================
// Sampling Methods
// ============================================================================

// shouldSample determines if a log message should be recorded based on sampling configuration.
// Returns true if:
//   - Sampling is disabled (default)
//   - The counter is less than Initial
//   - The counter modulo Thereafter equals 0
//
// Thread-safe using atomic operations for counter and mutex only for tick reset.
func (l *Logger) shouldSample() bool {
	state := l.loadSamplingState()
	if state == nil {
		return true // No sampling configured
	}

	if state.config == nil || !state.config.Enabled {
		return true
	}

	var count int64

	// Check if tick interval has passed and reset if needed, then increment atomically.
	// Both operations are inside the lock to prevent a race where one goroutine resets
	// the counter while another has already read a stale value.
	if state.config.Tick > 0 {
		state.startMu.Lock()
		elapsed := time.Since(state.start)
		if elapsed >= state.config.Tick {
			state.counter.Store(0)
			state.start = time.Now()
		}
		count = state.counter.Add(1)
		state.startMu.Unlock()
	} else {
		// No tick reset needed — pure atomic increment
		count = state.counter.Add(1)
	}

	// Always log the first Initial messages
	if count <= int64(state.config.Initial) {
		return true
	}

	// Log 1 out of every Thereafter messages after Initial
	if state.config.Thereafter > 0 {
		return (count-int64(state.config.Initial))%int64(state.config.Thereafter) == 0
	}

	// If Thereafter is 0 after Initial, don't log anymore
	return false
}

// SetSampling enables or disables log sampling at runtime (thread-safe).
// Pass nil to disable sampling.
// Note: This method creates a copy of the config to avoid mutating the caller's data.
func (l *Logger) SetSampling(config *SamplingConfig) {
	if l.closed.Load() {
		return
	}

	if config == nil || !config.Enabled {
		// Don't store nil in atomic.Value - use a disabled state instead
		disabledState := &samplingState{
			config: &SamplingConfig{Enabled: false},
		}
		disabledState.counter.Store(0)
		l.sampling.Store(disabledState)
		return
	}

	// Create a copy to avoid mutating caller's config
	cfg := &SamplingConfig{
		Enabled:    config.Enabled,
		Initial:    config.Initial,
		Thereafter: config.Thereafter,
		Tick:       config.Tick,
	}

	// Apply defaults to the copy
	if cfg.Initial < 0 {
		cfg.Initial = 0
	}
	// Thereafter=0 is valid and means "log nothing after Initial"
	// Thereafter<0 is treated as "log everything" (set to 1)
	if cfg.Thereafter < 0 {
		cfg.Thereafter = 1
	}
	if cfg.Tick <= 0 {
		cfg.Tick = 0 // No tick reset
	}

	newState := &samplingState{
		config: cfg,
		start:  time.Now(),
	}
	newState.counter.Store(0)
	l.sampling.Store(newState)
}

// GetSampling returns a copy of the current sampling configuration (thread-safe).
// Returns nil if sampling is not enabled. Mutating the returned value does not
// affect the logger; use SetSampling to change the configuration.
func (l *Logger) GetSampling() *SamplingConfig {
	state := l.loadSamplingState()
	if state == nil {
		return nil
	}
	if state.config == nil || !state.config.Enabled {
		return nil
	}
	copied := *state.config
	return &copied
}

// ============================================================================
// Security Methods
// ============================================================================

// SetSecurityConfig atomically sets the security configuration (thread-safe).
// The configuration is cloned before being stored, so later mutations of the
// caller's config (or of the SensitiveFilter reachable through it) do not
// affect the logger — matching New(), which also clones Config.Security.
func (l *Logger) SetSecurityConfig(config *SecurityConfig) {
	if config == nil {
		config = DefaultSecurityConfig()
	}
	config = config.Clone()
	// Update rate limiter when security config changes.
	// Store atomically: the hot path reads rateLimiter without a lock.
	if config.RateLimitConfig != nil {
		l.rateLimiter.Store(internal.NewRateLimiter(config.RateLimitConfig))
	} else {
		l.rateLimiter.Store(nil)
	}
	l.securityConfig.Store(config)
}

// GetSecurityConfig returns a copy of the current security configuration (thread-safe).
// Returns DefaultSecurityConfig() if no security config has been set.
// The returned config is a clone, so modifications do not affect the logger's config.
// For internal use within the logger, use getSecurityConfig() which returns the original.
func (l *Logger) GetSecurityConfig() *SecurityConfig {
	secConfig := l.securityConfig.Load()
	if secConfig == nil {
		return DefaultSecurityConfig()
	}
	return secConfig.Clone()
}

// getSecurityConfig returns the current security configuration (internal use).
// This returns the original config pointer, not a clone, for efficiency.
// For external access, use GetSecurityConfig() which returns a safe clone.
func (l *Logger) getSecurityConfig() *SecurityConfig {
	if secConfig := l.securityConfig.Load(); secConfig != nil {
		return secConfig
	}
	return DefaultSecurityConfig()
}

// valueUnchanged reports whether the filtered value is the same as the
// original. A direct == comparison panics when both sides hold an
// uncomparable dynamic type (map, slice, func), so those are reported as
// changed — the recursive filter rebuilds containers, so an uncomparable
// result never compares equal to its input anyway.
func valueUnchanged(filtered, original any) bool {
	if filtered == nil || original == nil {
		return filtered == nil && original == nil
	}
	t := reflect.TypeOf(original)
	if t != reflect.TypeOf(filtered) {
		return false
	}
	if !t.Comparable() {
		return false
	}
	return filtered == original
}

// processFields processes and filters structured fields
func (l *Logger) processFields(fields []Field) []Field {
	if len(fields) == 0 {
		return fields
	}

	// Validate field keys if validation is enabled
	l.validateFields(fields)

	secConfig := l.getSecurityConfig()
	if secConfig == nil || secConfig.SensitiveFilter == nil || !secConfig.SensitiveFilter.IsEnabled() {
		return fields // Early return - no allocation
	}

	// First pass: check if any field actually needs filtering
	// This avoids allocation when all values are non-sensitive
	needsFiltering := false
	hasPatterns := secConfig.SensitiveFilter.PatternCount() > 0

	for _, field := range fields {
		// Check if value is a string that might contain sensitive data patterns.
		// Cheapest discriminating check first: with patterns registered, any string
		// field forces a scan, and the other two conditions set the same flag, so
		// ordering only affects which check short-circuits — never the outcome.
		if _, ok := field.Value.(string); ok && hasPatterns {
			needsFiltering = true
			break
		}
		// Check for complex types that might need recursive filtering
		if internal.IsComplexValue(field.Value) {
			needsFiltering = true
			break
		}
		// Check if key is sensitive (requires redaction regardless of patterns)
		if internal.IsSensitiveKey(field.Key) {
			needsFiltering = true
			break
		}
	}

	// If no field needs filtering, return original slice
	if !needsFiltering {
		return fields
	}

	// Filter each field using copy-on-write: allocate the result slice only when a
	// value is actually redacted. The common case (no sensitive data present)
	// returns the original slice with zero allocations.
	var result []Field
	for i, field := range fields {
		var filtered any
		if str, ok := field.Value.(string); ok {
			// String fast path: same semantics as FilterValueRecursive's string
			// branch, but the usually-unchanged result stays a plain string —
			// returning it inside an `any` would allocate a boxed copy per
			// string field per log call.
			if fs := secConfig.SensitiveFilter.FilterString(field.Key, str); fs != str {
				filtered = fs
			} else {
				continue // unchanged — defer allocation until a real redaction occurs
			}
		} else {
			filtered = secConfig.SensitiveFilter.FilterValueRecursive(field.Key, field.Value)
			if valueUnchanged(filtered, field.Value) {
				continue // unchanged — defer allocation until a real redaction occurs
			}
		}
		// First redaction: lazily allocate result and seed it with the originals.
		if result == nil {
			result = make([]Field, len(fields))
			copy(result, fields)
		}
		result[i].Value = filtered
		// Notify HookOnFilter subscribers (key only — never the value).
		l.fireFilterHook(field.Key)
		// Emit audit event for sensitive data redaction
		if l.auditLogger != nil {
			l.auditLogger.LogSensitiveDataRedaction("", field.Key, "field value redacted during processing")
		}
	}

	if result == nil {
		return fields
	}
	return result
}

// fireFilterHook notifies HookOnFilter subscribers that sensitive data was
// redacted. key is the redacted field's key, or empty for message-level
// redaction. It carries only the key (never the redacted value, to avoid
// re-leaking the very data the filter removed). It is a no-op — and
// allocation-free — when no hooks are registered.
func (l *Logger) fireFilterHook(key string) {
	if l.loadHooks() == nil {
		return
	}
	hookCtx := &HookContext{Event: HookOnFilter}
	if key != "" {
		hookCtx.Metadata = map[string]any{"field": key}
	}
	_ = l.triggerHooks(context.Background(), hookCtx)
}

// applyMessageSecurity applies sensitive data filtering to the raw message (before formatting)
func (l *Logger) applyMessageSecurity(message string) string {
	secConfig := l.getSecurityConfig()
	if secConfig == nil {
		return internal.SanitizeControlChars(message)
	}

	if secConfig.SensitiveFilter != nil && secConfig.SensitiveFilter.IsEnabled() {
		filtered := secConfig.SensitiveFilter.Filter(message)
		if filtered != message {
			l.fireFilterHook("")
			message = filtered
		}
	}

	return internal.SanitizeControlChars(message)
}

// applySizeLimitBuffer applies the message size limit to a formatted log
// line (after formatting, before writing), truncating the buffer in place.
// Byte-for-byte equivalent to the former string-based applySizeLimit.
func (l *Logger) applySizeLimitBuffer(buf *bytes.Buffer) {
	secConfig := l.getSecurityConfig()
	if secConfig == nil {
		return
	}

	if secConfig.MaxMessageSize > 0 && buf.Len() > secConfig.MaxMessageSize {
		const ellipsis = "..."
		// Reserve room for the ellipsis so the result never exceeds MaxMessageSize.
		// When the limit is too small to hold the ellipsis, hard-truncate instead.
		limit := secConfig.MaxMessageSize
		useEllipsis := limit >= len(ellipsis)
		if useEllipsis {
			limit -= len(ellipsis)
		}
		// Truncate at a valid UTF-8 rune boundary to avoid corrupting multi-byte characters
		message := buf.Bytes()
		truncIdx := limit
		for truncIdx > 0 && !utf8.RuneStart(message[truncIdx]) {
			truncIdx--
		}
		buf.Truncate(truncIdx)
		if useEllipsis {
			buf.WriteString(ellipsis)
		}
	}
}

// validateFields validates field keys against the configured naming convention.
// In warn mode, validation errors are logged as warnings.
// In strict mode, validation errors are logged as errors.
func (l *Logger) validateFields(fields []Field) {
	fv := l.getFieldValidation()
	if fv == nil || fv.Mode == FieldValidationNone {
		return
	}

	for _, field := range fields {
		if err := fv.ValidateFieldKey(field.Key); err != nil {
			switch fv.Mode {
			case FieldValidationWarn:
				// Log warning without affecting the log output
				fmt.Fprintf(os.Stderr, "dd: field validation warning: %v\n", err)
			case FieldValidationStrict:
				// Log error without affecting the log output
				fmt.Fprintf(os.Stderr, "dd: field validation error: %v\n", err)
			}
		}
	}
}

// getFieldValidation safely returns the field validation configuration.
func (l *Logger) getFieldValidation() *FieldValidationConfig {
	if ptr := l.fieldValidation.Load(); ptr != nil {
		return ptr
	}
	return nil
}

// SetFieldValidation sets the field validation configuration (thread-safe).
// This allows runtime adjustment of field key validation.
// The configuration is copied, so later mutations of the caller's value do
// not affect the logger.
//
// Example:
//
//	// Enable strict snake_case validation
//	logger.SetFieldValidation(dd.StrictSnakeCaseConfig())
func (l *Logger) SetFieldValidation(config *FieldValidationConfig) {
	if config == nil || config.Mode == FieldValidationNone {
		l.fieldValidation.Store(nil)
	} else {
		fv := *config // copy: FieldValidationConfig holds only value fields
		l.fieldValidation.Store(&fv)
	}
}

// GetFieldValidation returns a copy of the current field validation configuration.
// Returns nil if no validation is configured. Mutating the returned value does
// not affect the logger; use SetFieldValidation to change it.
func (l *Logger) GetFieldValidation() *FieldValidationConfig {
	if fv := l.getFieldValidation(); fv != nil {
		copied := *fv
		return &copied
	}
	return nil
}

// ============================================================================
// Writer Methods
// ============================================================================

// AddWriter adds a writer to the logger in a thread-safe manner.
func (l *Logger) AddWriter(writer io.Writer) error {
	if writer == nil {
		return ErrNilWriter
	}

	if l.closed.Load() {
		return ErrLoggerClosed
	}

	// Wire FileWriter rotation callback to trigger HookOnRotate
	if fw, ok := writer.(*FileWriter); ok {
		fw.SetOnRotateCallback(func(path string) {
			if registry := l.loadHooks(); registry != nil {
				hookCtx := &HookContext{
					Event:     HookOnRotate,
					Timestamp: time.Now(),
					Metadata:  map[string]any{"path": path},
				}
				_ = registry.Trigger(context.Background(), HookOnRotate, hookCtx)
			}
		})
	}

	l.writersMu.Lock()
	defer l.writersMu.Unlock()

	if err := addToWriterList(&l.writersPtr, writer, false); err != nil {
		if errors.Is(err, errWriterListClosed) {
			return ErrLoggerClosed
		}
		return err
	}
	return nil
}

// RemoveWriter removes a writer from the logger in a thread-safe manner.
func (l *Logger) RemoveWriter(writer io.Writer) error {
	if writer == nil {
		return ErrNilWriter
	}

	if l.closed.Load() {
		return ErrLoggerClosed
	}

	l.writersMu.Lock()
	defer l.writersMu.Unlock()

	if err := removeFromWriterList(&l.writersPtr, writer); err != nil {
		if errors.Is(err, errWriterListClosed) {
			return ErrLoggerClosed
		}
		return err // ErrWriterNotFound
	}
	return nil
}

// WriterCount returns the number of registered writers (thread-safe).
func (l *Logger) WriterCount() int {
	writersPtr := l.writersPtr.Load()
	if writersPtr == nil {
		return 0
	}
	return len(*writersPtr)
}

// Flush flushes all buffered writers (thread-safe).
// Writers that implement Flusher interface will be flushed.
// A panic raised by a writer's Flush method is recovered and returned as an error.
func (l *Logger) Flush() error {
	writersPtr := l.writersPtr.Load()
	if writersPtr == nil {
		return nil
	}

	var firstErr error
	for _, w := range *writersPtr {
		if flusher, ok := w.(Flusher); ok {
			if err := safeFlush(flusher); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// safeFlush calls a writer's Flush with panic recovery.
// SEC-003: Flush is a public API entry point with no surrounding recover, so a
// panicking writer must not crash the caller (mirrors safeWrite/safeClose).
func safeFlush(f Flusher) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flush panic: %v", r)
		}
	}()
	return f.Flush()
}

// writeMessageBuffer writes one formatted log line to all configured writers
// and releases the line buffer back to the pool. It takes ownership of buf:
// callers must not touch buf after this returns.
// The trailing newline is appended to the assembled line so each writer
// receives the complete entry in a SINGLE Write call — a line is never split
// across two writes, which two goroutines logging through the same writer
// could otherwise interleave into corrupted output ("msgAmsgB\n\n").
// Writers are still called through safeWrite so a panicking writer is
// recovered here rather than unwinding into logCoreWithDepth's deferred
// recover — which would skip the AfterLog hook and, for Fatal, skip
// handleFatal() (the process would not exit on a Fatal log).
func (l *Logger) writeMessageBuffer(buf *bytes.Buffer) {
	if l.closed.Load() || buf.Len() == 0 {
		internal.PutLineBuffer(buf)
		return
	}

	writersPtr := l.writersPtr.Load()
	if writersPtr == nil || len(*writersPtr) == 0 {
		internal.PutLineBuffer(buf)
		return
	}
	writers := *writersPtr

	buf.WriteByte('\n')
	line := buf.Bytes()
	for _, writer := range writers {
		if _, err := l.safeWrite(writer, line); err != nil {
			l.handleWriteError(writer, err)
		}
	}

	internal.PutLineBuffer(buf)
}

// handleWriteError handles write errors by calling both legacy handler and hooks.
func (l *Logger) handleWriteError(writer io.Writer, err error) {
	// Call legacy write error handler
	if handler := l.getWriteErrorHandler(); handler != nil {
		l.safeCallWriteErrorHandler(handler, writer, err)
	}

	// Trigger OnError hook. Fast path when no hooks are registered: skip the
	// HookContext allocation and time.Now() call entirely (mirrors the hasHooks
	// fast path in logCoreWithDepth).
	if l.loadHooks() == nil {
		return
	}
	hookCtx := &HookContext{
		Event:     HookOnError,
		Error:     err,
		Writer:    writer,
		Timestamp: time.Now(),
	}
	_ = l.triggerHooks(l.ctx, hookCtx)
}

// safeCallWriteErrorHandler calls the user WriteErrorHandler with panic recovery.
// SEC-003: recovered here rather than by logCoreWithDepth's deferred recover for
// the same reason as safeWrite — a panic unwinding that far would skip the
// AfterLog hook and, for Fatal logs, handleFatal() (the process would not exit).
func (l *Logger) safeCallWriteErrorHandler(handler WriteErrorHandler, writer io.Writer, err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "dd: write error handler panic: %v (original write error: %v)\n", r, err)
		}
	}()
	handler(writer, err)
}

// safeWrite writes to a writer with panic recovery.
// Prevents a panicking writer from crashing the entire application
// or blocking other writers in a multi-writer setup.
func (l *Logger) safeWrite(writer io.Writer, buf []byte) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("writer panic: %v", r)
		}
	}()
	return writer.Write(buf)
}

// ============================================================================
// Lifecycle Methods
// ============================================================================

// closeWritersLocked flushes and closes all writers. Must be called with
// writersMu held. Teardown is unconditional: once this runs, the writer list
// is swapped out and its writers are unreachable to every other code path,
// so the loop must run to completion — aborting on the caller's deadline (or
// bailing out before the swap after the logger is already marked closed)
// would leak the writers and their file handles permanently. Deadline
// semantics are enforced by the callers instead: Shutdown returns early on
// ctx expiry while this loop finishes in the background, and Shutdown's
// entry check handles an already-expired context before anything is taken.
func (l *Logger) closeWritersLocked() error {
	currentWriters := l.writersPtr.Swap(nil)
	if currentWriters == nil {
		return nil
	}

	errs := make([]error, 0, len(*currentWriters))
	for _, writer := range *currentWriters {
		// Flush BEFORE closing: a writer that implements Flusher but not
		// io.Closer (e.g. a bare *bufio.Writer registered via AddWriter) has
		// no Close of its own to flush through, and its buffered data would
		// otherwise be silently lost at teardown. Redundant (and harmless)
		// for the built-in writers, whose Close already flushes.
		if flusher, ok := writer.(Flusher); ok {
			if err := safeFlush(flusher); err != nil {
				errs = append(errs, fmt.Errorf("failed to flush writer: %w", err))
			}
		}

		if err := closeWriter(writer); err != nil {
			errs = append(errs, fmt.Errorf("failed to close writer: %w", err))
		}
	}

	return errors.Join(errs...)
}

// Close closes the logger and all associated resources (thread-safe).
// If multiple writers fail to close, all errors are collected and returned.
// Triggers OnClose hooks before closing writers.
// Waits for in-flight security filter goroutines to complete before closing writers.
// Writers implementing the Flusher interface are flushed before being closed,
// so buffered data is not lost even for custom writers whose Close does not
// flush (or that implement no Close at all).
func (l *Logger) Close() error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil
	}

	// Trigger OnClose hook
	hookCtx := &HookContext{
		Event:     HookOnClose,
		Timestamp: time.Now(),
	}
	_ = l.triggerHooks(context.Background(), hookCtx)

	// Close audit logger. Its Close has no failure mode the caller can act on
	// during logger teardown (audit events are best-effort by design), so the
	// error is intentionally discarded.
	if l.auditLogger != nil {
		_ = l.auditLogger.Close()
	}

	l.cancel()

	// Wait for in-flight security filter goroutines before closing writers.
	// This prevents filter goroutines from attempting to use closed writers.
	// Filter goroutines are bounded by defaultFilterTimeout (50ms), so
	// waiting 200ms (4x) provides generous headroom.
	if !l.WaitForFilterGoroutines(defaultFilterTimeout * 4) {
		fmt.Fprintln(os.Stderr, "[dd] Warning: some filter goroutines did not complete during Close()")
	}

	l.writersMu.Lock()
	defer l.writersMu.Unlock()

	return l.closeWritersLocked()
}

// Shutdown gracefully closes the logger with a timeout.
// This is the recommended way to close a logger in production environments.
//
// The method performs the following steps:
//  1. Marks the logger as closed to prevent new log entries
//  2. Triggers OnClose hooks with the provided context
//  3. Waits for any in-flight operations to complete
//  4. Flushes and closes all writers with the specified timeout
//
// If the timeout is exceeded, Shutdown returns a context.DeadlineExceeded error
// along with any other errors that occurred during shutdown.
//
// An already-expired context fails fast: its error is returned and the logger
// is left open, so shutdown can be retried with a fresh context (or Close).
//
// On a timeout DURING teardown, Shutdown returns the context error while the
// writer teardown continues in the background: the writers have already been
// taken out of the logger, and the flush/close loop always runs to completion,
// so no writer is leaked even though the caller stopped waiting. A subsequent
// Close or Shutdown returns nil immediately (the logger is already marked
// closed), not as a guarantee that the background teardown has finished.
//
// Recommended usage:
//
//	logger, _ := dd.New(dd.DefaultConfig())
//	defer func() {
//	    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	    defer cancel()
//	    if err := logger.Shutdown(ctx); err != nil {
//	        fmt.Fprintf(os.Stderr, "Logger shutdown error: %v\n", err)
//	    }
//	}()
func (l *Logger) Shutdown(ctx context.Context) error {
	// Fail fast on an already-expired context BEFORE marking the logger closed.
	// After the CAS below, a canceled context aborts writer teardown with the
	// logger permanently unusable and its writers unclosed. Returning here
	// leaves the logger open, so the caller can retry with a fresh context.
	if err := ctx.Err(); err != nil {
		return err
	}

	if !l.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	// Create a done channel to signal completion
	done := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "dd: recovered from panic during Shutdown: %v\n", r)
				select {
				case done <- fmt.Errorf("shutdown panic: %v", r):
				default:
				}
			}
		}()

		// Trigger OnClose hook
		hookCtx := &HookContext{
			Event:     HookOnClose,
			Timestamp: time.Now(),
		}
		_ = l.triggerHooks(ctx, hookCtx)

		// Close audit logger. Best-effort during shutdown teardown: audit
		// events are fire-and-forget, so a close error is not actionable here.
		if l.auditLogger != nil {
			_ = l.auditLogger.Close()
		}

		// Close writers BEFORE canceling internal context, so background
		// teardown does not immediately see the canceled internal context.
		// closeWritersLocked runs to completion regardless of the caller's
		// deadline: if ctx expires first, Shutdown returns the context error
		// while this loop finishes flushing/closing in the background.
		l.writersMu.Lock()
		closeErr := l.closeWritersLocked()
		l.writersMu.Unlock()

		// Now cancel internal context for background goroutines.
		l.cancel()

		done <- closeErr
	}()

	// Wait for completion or timeout
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsClosed returns true if the logger has been closed (thread-safe).
func (l *Logger) IsClosed() bool {
	return l.closed.Load()
}

// handleFatal handles fatal log messages with timeout protection.
// If Close() takes longer than defaultFatalFlushTimeout, a warning is printed
// and the program exits anyway to prevent indefinite hanging.
func (l *Logger) handleFatal() {
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "dd: recovered from panic during fatal close: %v\n", r)
			}
			close(done)
		}()
		_ = l.Close()
	}()

	select {
	case <-done:
		// Close completed successfully
	case <-time.After(defaultFatalFlushTimeout):
		fmt.Fprintf(os.Stderr, "[dd] Warning: logger close timed out after %s\n", defaultFatalFlushTimeout)
	}

	if l.fatalHandler != nil {
		safeCallFatalHandler(l.fatalHandler)
	} else {
		os.Exit(1)
	}
}

// safeCallFatalHandler invokes the user FatalHandler with panic recovery.
// SEC-003: handleFatal runs inside logCoreWithDepth, whose deferred recover
// would swallow a handler panic and silently leave the process running after
// a Fatal log (the same no-exit bug class safeCallWriteErrorHandler guards
// against). A panicking handler falls back to the documented default —
// os.Exit(1) — because a Fatal log must terminate the process.
func safeCallFatalHandler(handler FatalHandler) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "dd: fatal handler panic (falling back to exit 1): %v\n", r)
			os.Exit(1)
		}
	}()
	handler()
}

// ActiveFilterGoroutines returns the number of currently active filter goroutines
// in the security filter. This can be used for monitoring and detecting potential
// goroutine leaks in high-concurrency scenarios. A consistently high count may
// indicate that filter operations are timing out frequently.
func (l *Logger) ActiveFilterGoroutines() int32 {
	secConfig := l.getSecurityConfig()
	if secConfig == nil || secConfig.SensitiveFilter == nil {
		return 0
	}
	return secConfig.SensitiveFilter.ActiveGoroutineCount()
}

// WaitForFilterGoroutines waits for all active filter goroutines to complete
// or until the timeout is reached.
//
// IMPORTANT: Call this method before Close() in graceful shutdown scenarios
// to prevent goroutine leaks. The security filter may spawn background goroutines
// for processing large inputs with regex patterns. Failing to wait for these
// goroutines can result in resource leaks.
//
// Example graceful shutdown:
//
//	// 1. Stop accepting new requests/logs
//	// 2. Wait for filter goroutines (use 2-5 seconds typically)
//	if !logger.WaitForFilterGoroutines(3 * time.Second) {
//	    log.Println("Warning: some filter goroutines did not complete in time")
//	}
//	// 3. Close the logger
//	logger.Close()
//
// Returns true if all goroutines completed, false if timeout was reached.
func (l *Logger) WaitForFilterGoroutines(timeout time.Duration) bool {
	secConfig := l.getSecurityConfig()
	if secConfig == nil || secConfig.SensitiveFilter == nil {
		return true
	}
	return secConfig.SensitiveFilter.WaitForGoroutines(timeout)
}

// ============================================================================
// Log Methods
// ============================================================================

// logEntry contains the data needed to write a log entry
type logEntry struct {
	msg            string
	caller         string // pre-resolved caller (entry-site capture); empty if not resolved
	fields         []Field
	originalFields []Field // fields before processing (for hooks)
}

// logCore is the internal implementation for all log methods.
// It handles security filtering, hooks, formatting, writing, and fatal handling.
func (l *Logger) logCore(level LogLevel, entry logEntry) {
	l.logCoreWithDepth(level, entry, 0)
}

// logCoreWithDepth is like logCore but accepts an additional caller depth offset.
// This is used by LoggerEntry to skip the extra stack frames introduced by the entry wrapper.
func (l *Logger) logCoreWithDepth(level LogLevel, entry logEntry, extraDepth int) {
	// SEC-003: Recover from panics in formatting, field processing, hooks, and writing.
	// A logging library must never crash the application.
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "dd: recovered from panic in logCore: %v\n", r)
		}
	}()

	// Extract context fields from configured extractors.
	// When extractors are registered, they are called with context.Background()
	// since log methods do not accept a context parameter. Fields are merged
	// with the entry fields so they appear in the final output.
	var contextFields []Field
	if registry := l.loadContextExtractors(); registry != nil {
		contextFields = registry.Extract(context.Background())
	}

	// Merge context fields with entry fields (entry fields take precedence)
	allFields := entry.fields
	if len(contextFields) > 0 {
		if len(allFields) == 0 {
			// Context fields have not been filtered yet — apply security filtering
			allFields = l.processFields(contextFields)
		} else {
			// entry.fields are already filtered; contextFields need filtering
			filteredContextFields := l.processFields(contextFields)
			allFields = mergeFieldSlices(filteredContextFields, allFields)
		}
	}

	// Fast path: check if hooks exist before allocating HookContext
	hasHooks := l.loadHooks() != nil

	var hookCtx *HookContext
	if hasHooks {
		// Only allocate HookContext and call time.Now() when hooks are registered
		hookCtx = &HookContext{
			Event:          HookBeforeLog,
			Level:          level,
			Message:        entry.msg,
			Fields:         allFields,
			OriginalFields: entry.originalFields,
			Timestamp:      time.Now(),
		}
		if err := l.triggerHooks(context.Background(), hookCtx); err != nil {
			return // Hook aborted the log
		}
	}

	callerDepth := l.callerDepth + extraDepth
	// Entry methods resolve the caller at their own (shallow) stack position
	// and pass it in; resolving here would force the stack capture to unwind
	// every intervening library frame (the dominant CPU cost per pprof). The
	// formatter-position resolution remains as a fallback so a path without an
	// entry-site capture can never silently lose caller reporting.
	caller := entry.caller
	if l.dynamicCaller && caller == "" {
		caller = l.formatter.ResolveCaller(callerDepth)
	}
	line := l.formatter.FormatWithMessageBytes(level, caller, entry.msg, allFields)

	// Post-format byte rate gate (Fatal is exempt: it must always be written
	// before the process exits). This is the only point where the formatted
	// message size is known, so byte limiting (MaxBytesPerSecond) is enforced
	// here rather than in the pre-format shouldLog gate.
	if level != LevelFatal {
		// Load once: rateLimiter may be swapped concurrently by SetSecurityConfig.
		if rl := l.rateLimiter.Load(); rl != nil && !rl.AllowBytes(line.Len()) {
			internal.PutLineBuffer(line)
			if l.auditLogger != nil {
				l.auditLogger.LogRateLimitExceeded("log message byte-limited", nil)
			}
			return
		}
	}

	l.applySizeLimitBuffer(line)
	l.writeMessageBuffer(line)

	// Trigger AfterLog hook (only if hooks exist)
	if hasHooks {
		hookCtx.Event = HookAfterLog
		_ = l.triggerHooks(context.Background(), hookCtx)
	}

	if level == LevelFatal {
		l.handleFatal()
	}
}

// logDispatch is the single funnel for the non-structured entry methods (Log,
// the level wrappers, and the Print family). Every entry method calls it
// DIRECTLY — CaptureEntryCaller relies on this fixed frame shape (see
// internal.CaptureEntryCaller); routing one entry method through another
// would shift the caller capture onto the wrong frame.
func (l *Logger) logDispatch(level LogLevel, args ...any) {
	if l == nil {
		return
	}
	if !l.shouldLog(level) {
		return
	}

	// Entry-dispatch caller capture (see internal.EntryCaller).
	var caller string
	if l.dynamicCaller {
		caller = internal.EntryCaller(l.formatter.FullPath())
	}
	msg := l.applyMessageSecurity(l.formatter.FormatArgsToString(args...))
	l.logCore(level, logEntry{msg: msg, caller: caller})
}

// logfDispatch is logDispatch for the formatted-message entry family (Logf,
// the level wrappers, and Printf). Same frame-shape requirement.
func (l *Logger) logfDispatch(level LogLevel, format string, args ...any) {
	if l == nil {
		return
	}
	if !l.shouldLog(level) {
		return
	}

	// Entry-dispatch caller capture (see internal.EntryCaller).
	var caller string
	if l.dynamicCaller {
		caller = internal.EntryCaller(l.formatter.FullPath())
	}
	msg := l.applyMessageSecurity(fmt.Sprintf(format, args...))
	l.logCore(level, logEntry{msg: msg, caller: caller})
}

// logWithDispatch is logDispatch for the structured entry family (LogWith and
// the level wrappers). Same frame-shape requirement.
func (l *Logger) logWithDispatch(level LogLevel, msg string, fields ...Field) {
	if l == nil {
		return
	}
	if !l.shouldLog(level) {
		return
	}

	// Entry-dispatch caller capture (see internal.EntryCaller).
	var caller string
	if l.dynamicCaller {
		caller = internal.EntryCaller(l.formatter.FullPath())
	}
	l.logFiltered(level, msg, fields, caller, 0)
}

// logWithLazyMessage is the lazy-message twin of logFiltered for entry-layer
// methods whose message is derived from user arguments (LoggerEntry dispatch).
// msg is invoked only after the level gate passes — mirroring the (*Logger)
// entry families — so argument formatting and user String()/Error() methods
// never run for entries the gate rejects. caller is the entry-dispatch caller
// capture. extraDepth is entryCallerDepth, to skip the entry wrapper's stack
// frames in the fallback paths.
func (l *Logger) logWithLazyMessage(level LogLevel, msg func() string, fields []Field, caller string, extraDepth int) {
	if l == nil || msg == nil {
		return
	}
	if !l.shouldLog(level) {
		return
	}
	l.logFiltered(level, msg(), fields, caller, extraDepth)
}

// logFiltered runs the post-gate pipeline shared by both structured-logging
// layers ((*Logger).logWithDispatch, extraDepth 0, and the (*LoggerEntry)
// dispatchers, extraDepth entryCallerDepth): the hook copy of original
// fields, message security filtering, and field processing. caller is the
// entry-dispatch caller capture.
func (l *Logger) logFiltered(level LogLevel, msg string, fields []Field, caller string, extraDepth int) {
	// Only copy original fields if hooks are registered (they may need them)
	var originalFields []Field
	if l.loadHooks() != nil && len(fields) > 0 {
		originalFields = make([]Field, len(fields))
		copy(originalFields, fields)
	}

	msg = l.applyMessageSecurity(msg)
	processedFields := l.processFields(fields)

	l.logCoreWithDepth(level, logEntry{
		msg:            msg,
		caller:         caller,
		fields:         processedFields,
		originalFields: originalFields,
	}, extraDepth)
}

// Log logs a message at the specified level
func (l *Logger) Log(level LogLevel, args ...any) { l.logDispatch(level, args...) }

// Logf logs a formatted message at the specified level
func (l *Logger) Logf(level LogLevel, format string, args ...any) {
	l.logfDispatch(level, format, args...)
}

// LogWith logs a structured message with fields at the specified level
func (l *Logger) LogWith(level LogLevel, msg string, fields ...Field) {
	l.logWithDispatch(level, msg, fields...)
}

// Debug logs a message at DEBUG level.
func (l *Logger) Debug(args ...any) { l.logDispatch(LevelDebug, args...) }

// Info logs a message at INFO level.
func (l *Logger) Info(args ...any) { l.logDispatch(LevelInfo, args...) }

// Warn logs a message at WARN level.
func (l *Logger) Warn(args ...any) { l.logDispatch(LevelWarn, args...) }

// Error logs a message at ERROR level.
func (l *Logger) Error(args ...any) { l.logDispatch(LevelError, args...) }

// Fatal logs a message at FATAL level and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute. For graceful shutdown, use Error() with custom logic.
func (l *Logger) Fatal(args ...any) { l.logDispatch(LevelFatal, args...) }

// Debugf logs a formatted message at DEBUG level.
func (l *Logger) Debugf(format string, args ...any) { l.logfDispatch(LevelDebug, format, args...) }

// Infof logs a formatted message at INFO level.
func (l *Logger) Infof(format string, args ...any) { l.logfDispatch(LevelInfo, format, args...) }

// Warnf logs a formatted message at WARN level.
func (l *Logger) Warnf(format string, args ...any) { l.logfDispatch(LevelWarn, format, args...) }

// Errorf logs a formatted message at ERROR level.
func (l *Logger) Errorf(format string, args ...any) { l.logfDispatch(LevelError, format, args...) }

// Fatalf logs a formatted message at FATAL level and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute. For graceful shutdown, use Errorf() with custom logic.
func (l *Logger) Fatalf(format string, args ...any) { l.logfDispatch(LevelFatal, format, args...) }

// DebugWith logs a structured message with fields at DEBUG level.
func (l *Logger) DebugWith(msg string, fields ...Field) {
	l.logWithDispatch(LevelDebug, msg, fields...)
}

// InfoWith logs a structured message with fields at INFO level.
func (l *Logger) InfoWith(msg string, fields ...Field) {
	l.logWithDispatch(LevelInfo, msg, fields...)
}

// WarnWith logs a structured message with fields at WARN level.
func (l *Logger) WarnWith(msg string, fields ...Field) {
	l.logWithDispatch(LevelWarn, msg, fields...)
}

// ErrorWith logs a structured message with fields at ERROR level.
func (l *Logger) ErrorWith(msg string, fields ...Field) {
	l.logWithDispatch(LevelError, msg, fields...)
}

// FatalWith logs a structured message at FATAL level and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute. For graceful shutdown, use ErrorWith() with custom logic.
func (l *Logger) FatalWith(msg string, fields ...Field) {
	l.logWithDispatch(LevelFatal, msg, fields...)
}

// fmt package replacement methods — output via the logger's configured writers
// with caller info, using LevelInfo for filtering and applying sensitive-data
// filtering based on SecurityConfig.
//
// The package-level dd.Print / dd.Println / dd.Printf functions delegate to these
// methods (Default().Print() etc.), so the two layers share identical behavior:
// both write through the configured writers and apply security filtering.
//
// NOTE: These methods are NOT the debug-visualization helpers logger.Text /
// logger.JSON (and dd.Text / dd.JSON), which write directly to stdout WITHOUT
// security filtering and must never be used with sensitive data.
//
// DESIGN NOTE: Print() and Println() behave identically in this library.
// Unlike the standard fmt package where Println adds spaces and newline while Print does not,
// both methods here produce identical output because the underlying Log() method
// always adds a trailing newline. This is intentional for consistency with the
// structured logging pattern where all log entries end with a newline.

// Print writes to configured writers with caller info and newline.
// Uses LevelInfo for filtering. Arguments are joined with spaces.
// Applies sensitive data filtering based on SecurityConfig.
func (l *Logger) Print(args ...any) { l.logDispatch(LevelInfo, args...) }

// Println writes to configured writers with caller info and newline.
// Uses LevelInfo for filtering. Applies sensitive data filtering based on SecurityConfig.
//
// Note: Behaves identically to Print() because the underlying Log() method
// always adds a trailing newline. This differs from fmt.Println behavior.
func (l *Logger) Println(args ...any) { l.logDispatch(LevelInfo, args...) }

// Printf formats according to a format specifier and writes to configured writers with caller info.
// Uses LevelInfo for filtering.
func (l *Logger) Printf(format string, args ...any) {
	l.logfDispatch(LevelInfo, format, args...)
}

// Debug utilities - Text and JSON output for debugging
//
// The JSON helpers below are deliberately duplicated (not delegated) from the
// package-level dd.JSON/dd.JSONF in debug_visual.go: they resolve the caller at a
// FIXED depth (debugVisualizationDepth = 2), so each layer must call internal.Output*
// directly. Delegating would add a stack frame and report the wrapper, not the
// caller. See debug_visual.go for the full rationale.
//
// Text/Textf do not resolve the caller; the package-level dd.Text/dd.Textf
// delegate to these methods (see debug_visual.go), so they are not duplicated.

// Text outputs data as pretty-printed format to stdout for debugging.
//
// SECURITY WARNING: This method does NOT apply sensitive data filtering.
// Do not use with sensitive data in production environments. For secure logging,
// use logger.Info(), logger.Debug(), etc. which apply sensitive data filtering.
func (l *Logger) Text(data ...any) {
	internal.OutputTextData(os.Stdout, data...)
}

// Textf outputs formatted text to stdout for debugging.
func (l *Logger) Textf(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stdout, formatted)
}

// JSON outputs data as JSON to stdout for debugging.
func (l *Logger) JSON(data ...any) {
	caller := internal.GetCaller(debugVisualizationDepth, false)
	internal.OutputJSON(os.Stdout, caller, data...)
}

// JSONF outputs formatted JSON to stdout for debugging.
func (l *Logger) JSONF(format string, args ...any) {
	formatted := fmt.Sprintf(format, args...)
	caller := internal.GetCaller(debugVisualizationDepth, false)
	internal.OutputJSON(os.Stdout, caller, formatted)
}

// ============================================================================
// Global Logger State and Functions
// ============================================================================

// errNoInit is a sentinel error indicating no initialization error occurred.
// Used because atomic.Value cannot store nil values.
var errNoInit = errors.New("")

// Global logger state variables
var (
	defaultLogger  atomic.Pointer[Logger]
	defaultOnce    sync.Once
	defaultInitErr atomic.Value // stores error from initialization (errNoInit means no error)

	// backgroundCloseWg tracks goroutines spawned by SetDefault/InitDefault
	// to close old loggers. This prevents goroutine leaks during rapid replacement.
	backgroundCloseWg sync.WaitGroup
)

func init() {
	// Initialize with no-error state (atomic.Value cannot be empty)
	defaultInitErr.Store(errNoInit)
}

// DefaultInitError returns the error that occurred during default logger initialization.
// Returns nil if initialization was successful or hasn't occurred yet.
// This allows applications to detect if the default logger is running in fallback mode.
//
// Example:
//
//	logger := dd.Default()
//	if err := dd.DefaultInitError(); err != nil {
//	    log.Printf("Warning: default logger initialized with error: %v", err)
//	}
func DefaultInitError() error {
	if v := defaultInitErr.Load(); v != nil {
		if err, ok := v.(error); ok && err != errNoInit {
			return err
		}
	}
	return nil
}

// DefaultWithErr returns the default logger and any initialization error.
// This is useful when you need to verify the default logger was created correctly.
//
// Example:
//
//	logger, err := dd.DefaultWithErr()
//	if err != nil {
//	    // Handle initialization error
//	    log.Fatalf("Failed to initialize default logger: %v", err)
//	}
//	logger.Info("Application started")
func DefaultWithErr() (*Logger, error) {
	return Default(), DefaultInitError()
}

// Default returns the default global logger (thread-safe).
// The logger is created on first call with default configuration.
// Package-level convenience functions use this logger.
// Note: If SetDefault() is called before Default(), the custom logger is returned.
//
// To check if the default logger was initialized correctly, use DefaultInitError()
// or DefaultWithErr():
//
//	if err := dd.DefaultInitError(); err != nil {
//	    // Logger is running in fallback mode
//	}
func Default() *Logger {
	if logger := defaultLogger.Load(); logger != nil {
		return logger
	}

	defaultOnce.Do(func() {
		// Only create if not already set by SetDefault()
		if defaultLogger.Load() == nil {
			logger, err := DefaultConfig().build()
			if err != nil {
				// Store the error for later retrieval
				defaultInitErr.Store(err)

				// Print warning to stderr about fallback logger creation
				fmt.Fprintf(os.Stderr, "[dd] WARNING: Default logger initialization failed: %v\n", err)
				fmt.Fprintln(os.Stderr, "[dd] WARNING: Using fallback logger with stderr output")

				// Create fallback logger using the same Config→internalConfig
				// mapping as the normal build() path (toInternalConfig), then
				// force stderr output. This guarantees the fallback carries
				// every Config field in lock-step with the primary path rather
				// than hand-rolling a partial internalConfig.
				fallbackInternalCfg := defaultConfig().toInternalConfig()
				fallbackInternalCfg.writers = []io.Writer{os.Stderr}
				// The fallback config is fully defaulted (no audit config, a
				// single non-nil writer), so newFromInternalConfig cannot fail
				// here and ignoring the error is safe.
				logger, _ = newFromInternalConfig(fallbackInternalCfg)
			}
			// Install via CAS rather than Store: if SetDefault/InitDefault stored
			// a logger while this build was running, the user's logger wins and
			// the freshly built one is closed. A plain Store would silently
			// overwrite the user's logger — discarding their configuration and
			// leaking it (it was never closed by anyone).
			if logger != nil && !defaultLogger.CompareAndSwap(nil, logger) {
				_ = logger.Close() // lost the install race; don't leak our build
			}
		}
	})

	return defaultLogger.Load()
}

// SetDefault sets the default global logger (thread-safe).
// If a previous default logger exists, it is safely closed in background.
// Passing nil is ignored (no change).
func SetDefault(logger *Logger) {
	if logger == nil {
		return
	}

	closePreviousDefault(defaultLogger.Swap(logger))

	// Clear any initialization error recorded by a previous (fallback) default
	// logger: the newly installed logger is caller-provided and known-good, so
	// DefaultInitError/DefaultWithErr must not keep reporting the stale error.
	// Mirrors InitDefault's clear-on-success.
	defaultInitErr.Store(errNoInit)
}

// InitDefault initializes the default logger with the provided configuration.
// Returns an error if initialization fails. If a default logger already exists,
// it is closed and replaced with a new one.
//
// Example:
//
//	cfg := dd.DefaultConfig()
//	cfg.Level = dd.LevelDebug
//	if err := dd.InitDefault(cfg); err != nil {
//	    log.Fatalf("Failed to initialize logger: %v", err)
//	}
func InitDefault(cfg ...Config) error {
	var c Config
	if len(cfg) > 0 {
		c = cfg[0]
	} else {
		c = DefaultConfig()
	}
	logger, err := c.build()
	if err != nil {
		return err
	}

	oldLogger := defaultLogger.Swap(logger)
	closePreviousDefault(oldLogger)

	// Clear any previous initialization error
	defaultInitErr.Store(errNoInit)

	return nil
}

// closePreviousDefault closes a previously-installed default logger after a short
// delay on a tracked background goroutine, so in-flight writes to the old logger
// can drain before it is torn down. A nil oldLogger is a no-op. Shared by
// SetDefault and InitDefault.
func closePreviousDefault(oldLogger *Logger) {
	if oldLogger == nil {
		return
	}
	backgroundCloseWg.Go(func() {
		time.Sleep(defaultLoggerCloseDelay)
		_ = oldLogger.Close()
	})
}

// ============================================================================
// Package-level Logging Functions (use default logger)
//
// FRAME-SHAPE NOTE: these functions call the logDispatch/logfDispatch/
// logWithDispatch funnels DIRECTLY (not the (*Logger).Log/Logf/LogWith/Print
// entry methods). The entry-caller capture assumes exactly one entry-method
// frame between user code and the funnel (see internal.EntryCaller); routing a
// package-level function through another entry method adds a second frame and
// made the capture report this wrapper (e.g. "logger.go:NNN") as the caller.
// With the direct call, the package-level function itself is the entry-method
// frame and the capture lands on user code.

// Debug logs a message at DEBUG level using the default logger.
func Debug(args ...any) { Default().logDispatch(LevelDebug, args...) }

// Info logs a message at INFO level using the default logger.
func Info(args ...any) { Default().logDispatch(LevelInfo, args...) }

// Warn logs a message at WARN level using the default logger.
func Warn(args ...any) { Default().logDispatch(LevelWarn, args...) }

// Error logs a message at ERROR level using the default logger.
func Error(args ...any) { Default().logDispatch(LevelError, args...) }

// Fatal logs a message at FATAL level using the default logger and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute. For graceful shutdown, use Error() with custom logic.
func Fatal(args ...any) { Default().logDispatch(LevelFatal, args...) }

// Debugf logs a formatted message at DEBUG level using the default logger.
func Debugf(format string, args ...any) { Default().logfDispatch(LevelDebug, format, args...) }

// Infof logs a formatted message at INFO level using the default logger.
func Infof(format string, args ...any) { Default().logfDispatch(LevelInfo, format, args...) }

// Warnf logs a formatted message at WARN level using the default logger.
func Warnf(format string, args ...any) { Default().logfDispatch(LevelWarn, format, args...) }

// Errorf logs a formatted message at ERROR level using the default logger.
func Errorf(format string, args ...any) { Default().logfDispatch(LevelError, format, args...) }

// Fatalf logs a formatted message at FATAL level using the default logger and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute. For graceful shutdown, use Errorf() with custom logic.
func Fatalf(format string, args ...any) { Default().logfDispatch(LevelFatal, format, args...) }

// SetLevel sets the log level for the default logger.
// Returns ErrInvalidLevel if the level is outside the valid range [LevelDebug, LevelFatal].
func SetLevel(level LogLevel) error {
	return Default().SetLevel(level)
}

// GetLevel returns the current log level of the default logger.
func GetLevel() LogLevel {
	return Default().GetLevel()
}

// ============================================================================
// Generic Level Logging Functions
// ============================================================================

// Log logs a message at the specified level using the default logger.
func Log(level LogLevel, args ...any) { Default().logDispatch(level, args...) }

// Logf logs a formatted message at the specified level using the default logger.
func Logf(level LogLevel, format string, args ...any) {
	Default().logfDispatch(level, format, args...)
}

// LogWith logs a structured message at the specified level using the default logger.
func LogWith(level LogLevel, msg string, fields ...Field) {
	Default().logWithDispatch(level, msg, fields...)
}

// ============================================================================
// Level Check Functions
// ============================================================================

// IsLevelEnabled checks if the specified log level is enabled for the default logger.
func IsLevelEnabled(level LogLevel) bool { return Default().IsLevelEnabled(level) }

// IsDebugEnabled checks if DEBUG level is enabled for the default logger.
func IsDebugEnabled() bool { return Default().IsDebugEnabled() }

// IsInfoEnabled checks if INFO level is enabled for the default logger.
func IsInfoEnabled() bool { return Default().IsInfoEnabled() }

// IsWarnEnabled checks if WARN level is enabled for the default logger.
func IsWarnEnabled() bool { return Default().IsWarnEnabled() }

// IsErrorEnabled checks if ERROR level is enabled for the default logger.
func IsErrorEnabled() bool { return Default().IsErrorEnabled() }

// IsFatalEnabled checks if FATAL level is enabled for the default logger.
func IsFatalEnabled() bool { return Default().IsFatalEnabled() }

// ============================================================================
// Field Chaining Functions
// ============================================================================

// WithFields returns a LoggerEntry with pre-set fields using the default logger.
// The fields are inherited by all logging calls on the returned entry.
//
// Example:
//
//	dd.WithFields(dd.String("service", "api"), dd.String("version", "1.0")).
//	    Info("request received")
func WithFields(fields ...Field) *LoggerEntry {
	return Default().WithFields(fields...)
}

// WithField returns a LoggerEntry with a single pre-set field using the default logger.
//
// Example:
//
//	dd.WithField("request_id", "abc123").Info("processing request")
func WithField(key string, value any) *LoggerEntry {
	return Default().WithField(key, value)
}

// ============================================================================
// Lifecycle Functions
// ============================================================================

// Flush flushes any buffered data in the default logger.
func Flush() error { return Default().Flush() }

// ============================================================================
// Writer Management Functions
// ============================================================================

// AddWriter adds a writer to the default logger.
func AddWriter(writer io.Writer) error { return Default().AddWriter(writer) }

// RemoveWriter removes a writer from the default logger.
func RemoveWriter(writer io.Writer) error { return Default().RemoveWriter(writer) }

// WriterCount returns the number of writers in the default logger.
func WriterCount() int { return Default().WriterCount() }

// ============================================================================
// Sampling Functions
// ============================================================================

// SetSampling sets the sampling configuration for the default logger.
func SetSampling(config *SamplingConfig) { Default().SetSampling(config) }

// GetSampling returns the sampling configuration for the default logger.
func GetSampling() *SamplingConfig { return Default().GetSampling() }
