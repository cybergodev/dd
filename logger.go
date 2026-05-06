// Package dd provides a high-performance, thread-safe logging library.
package dd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/cybergodev/dd/internal"
)

var (
	messagePool = sync.Pool{
		New: func() any {
			buf := make([]byte, 0, defaultBufferSize)
			return &buf
		},
	}
)

// Compile-time interface verification
var _ LogProvider = (*Logger)(nil)

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
type FatalHandler func()

// WriteErrorHandler is called when a write operation to an io.Writer fails.
// The handler receives the writer that failed and the error that occurred.
// If no handler is set, write errors are silently ignored.
type WriteErrorHandler func(writer io.Writer, err error)

// LevelResolver is a function that determines the effective log level at runtime.
// This allows dynamic log level adjustment based on runtime conditions such as
// system load, error rate, or time of day. The function is called for each log
// entry, so it should be efficient.
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
	writeErrorHandler atomic.Value // stores WriteErrorHandler
	formatter         *internal.MessageFormatter

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
	securityConfig atomic.Value

	// contextExtractors stores the contextExtractorRegistry for extracting
	// fields from context. If nil, default extractors are used.
	contextExtractors atomic.Value // stores *contextExtractorRegistry
	// contextExtractorsMu protects the Clone-Modify-Store sequence in AddContextExtractor
	contextExtractorsMu sync.Mutex

	// hooks stores the HookRegistry for lifecycle hooks.
	hooks atomic.Value // stores *HookRegistry
	// hooksMu protects the Clone-Modify-Store sequence in AddHook to prevent race conditions
	hooksMu sync.Mutex

	// sampling stores the sampling configuration and state.
	sampling atomic.Value // stores *samplingState

	// rateLimiter provides rate limiting to prevent log flooding.
	// Initialized from SecurityConfig.RateLimitConfig when set.
	rateLimiter *internal.RateLimiter

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
		callerDepth:  defaultCallerDepth,
		fatalHandler: config.fatalHandler,
		formatter:    internal.NewMessageFormatter(formatterConfig),
		ctx:          ctx,
		cancel:       cancel,
	}

	// Initialize writers pointer with empty slice
	l.writersPtr.Store(&initialWriters)

	if config.writeErrorHandler != nil {
		l.writeErrorHandler.Store(config.writeErrorHandler)
	}

	l.level.Store(int32(config.level))
	if config.securityConfig != nil {
		l.securityConfig.Store(config.securityConfig.Clone())
		// Initialize rate limiter from security config
		if config.securityConfig.RateLimitConfig != nil {
			l.rateLimiter = internal.NewRateLimiter(config.securityConfig.RateLimitConfig)
		}
	} else {
		l.securityConfig.Store(DefaultSecurityConfig())
	}

	// Initialize field validation
	if config.fieldValidation != nil && config.fieldValidation.Mode != FieldValidationNone {
		l.fieldValidation.Store(config.fieldValidation)
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

	// Initialize audit logger
	if config.auditConfig != nil && config.auditConfig.Enabled {
		al, err := newAuditLoggerWithConfig(*config.auditConfig)
		if err == nil {
			l.auditLogger = al
		}
	}

	if config.writers != nil {
		for _, writer := range config.writers {
			if err := l.AddWriter(writer); err != nil {
				cancel()
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
		l.writeErrorHandler.Store(handler)
	} else {
		// atomic.Value cannot store nil, store a nil placeholder of the correct type
		l.writeErrorHandler.Store(WriteErrorHandler(nil))
	}
}

// getWriteErrorHandler returns the current write error handler (thread-safe).
func (l *Logger) getWriteErrorHandler() WriteErrorHandler {
	if v := l.writeErrorHandler.Load(); v != nil {
		if handler, ok := v.(WriteErrorHandler); ok {
			return handler
		}
	}
	return nil
}

// loadHooks returns the current hook registry or nil if no hooks are set.
func (l *Logger) loadHooks() *HookRegistry {
	if v := l.hooks.Load(); v != nil {
		if registry, ok := v.(*HookRegistry); ok {
			return registry
		}
	}
	return nil
}

// loadContextExtractors returns the current context extractor registry or nil.
func (l *Logger) loadContextExtractors() *contextExtractorRegistry {
	if v := l.contextExtractors.Load(); v != nil {
		if reg, ok := v.(*contextExtractorRegistry); ok {
			return reg
		}
	}
	return nil
}

// loadSamplingState returns the current sampling state or nil.
func (l *Logger) loadSamplingState() *samplingState {
	if v := l.sampling.Load(); v != nil {
		if state, ok := v.(*samplingState); ok {
			return state
		}
	}
	return nil
}

// shouldLog checks if a message should be logged based on level and logger state.
// When a LevelResolver is set, it determines the effective level dynamically.
// Otherwise, the static level is used.
func (l *Logger) shouldLog(level LogLevel) bool {
	var effectiveLevel LogLevel
	if resolver := l.getLevelResolver(); resolver != nil {
		// Use context.TODO() as public log methods don't accept context.
		// The resolver should handle nil-appropriate context gracefully.
		effectiveLevel = resolver(context.TODO())
	} else {
		effectiveLevel = LogLevel(l.level.Load())
	}
	if level < effectiveLevel || level > LevelFatal {
		return false
	}
	if l.closed.Load() {
		return false
	}
	// Check rate limiter before sampling
	if l.rateLimiter != nil && l.rateLimiter.ShouldRateLimit(0) {
		// Emit audit event for rate limit
		if l.auditLogger != nil {
			l.auditLogger.LogRateLimitExceeded("log message rate limited", nil)
		}
		return false
	}
	return l.shouldSample()
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
// Returns true if the logger's level is at or below the specified level.
//
// Example:
//
//	if logger.IsLevelEnabled(dd.LevelDebug) {
//	    // Expensive debug computation only when debug is enabled
//	    logger.DebugWith("Details", dd.Any("data", computeExpensiveDebugInfo()))
//	}
func (l *Logger) IsLevelEnabled(level LogLevel) bool {
	currentLevel := LogLevel(l.level.Load())
	return level >= currentLevel
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

// GetSampling returns the current sampling configuration (thread-safe).
// Returns nil if sampling is not enabled.
func (l *Logger) GetSampling() *SamplingConfig {
	state := l.loadSamplingState()
	if state == nil {
		return nil
	}
	if state.config == nil || !state.config.Enabled {
		return nil
	}
	return state.config
}

// ============================================================================
// Security Methods
// ============================================================================

// SetSecurityConfig atomically sets the security configuration (thread-safe).
func (l *Logger) SetSecurityConfig(config *SecurityConfig) {
	if config == nil {
		config = DefaultSecurityConfig()
	}
	// Update rate limiter when security config changes
	if config.RateLimitConfig != nil {
		l.rateLimiter = internal.NewRateLimiter(config.RateLimitConfig)
	} else {
		l.rateLimiter = nil
	}
	l.securityConfig.Store(config)
}

// GetSecurityConfig returns a copy of the current security configuration (thread-safe).
// Returns DefaultSecurityConfig() if no security config has been set.
// The returned config is a clone, so modifications do not affect the logger's config.
// For internal use within the logger, use getSecurityConfig() which returns the original.
func (l *Logger) GetSecurityConfig() *SecurityConfig {
	config := l.securityConfig.Load()
	if config == nil {
		return DefaultSecurityConfig()
	}
	secConfig, ok := config.(*SecurityConfig)
	if !ok {
		return DefaultSecurityConfig()
	}
	return secConfig.Clone()
}

// getSecurityConfig returns the current security configuration (internal use).
// This returns the original config pointer, not a clone, for efficiency.
// For external access, use GetSecurityConfig() which returns a safe clone.
func (l *Logger) getSecurityConfig() *SecurityConfig {
	if config := l.securityConfig.Load(); config != nil {
		if secConfig, ok := config.(*SecurityConfig); ok {
			return secConfig
		}
	}
	return DefaultSecurityConfig()
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
		// Check if key is sensitive (requires redaction regardless of patterns)
		if internal.IsSensitiveKey(field.Key) {
			needsFiltering = true
			break
		}
		// Check if value is a string that might contain sensitive data patterns
		if _, ok := field.Value.(string); ok && hasPatterns {
			needsFiltering = true
			break
		}
		// Check for complex types that might need recursive filtering
		if internal.IsComplexValue(field.Value) {
			needsFiltering = true
			break
		}
	}

	// If no field needs filtering, return original slice
	if !needsFiltering {
		return fields
	}

	// Pre-allocate result slice to exact size needed
	result := make([]Field, 0, len(fields))

	for _, field := range fields {
		filtered := secConfig.SensitiveFilter.FilterValueRecursive(field.Key, field.Value)
		result = append(result, Field{
			Key:   field.Key,
			Value: filtered,
		})
		// Emit audit event for sensitive data redaction
		if l.auditLogger != nil && filtered != field.Value {
			l.auditLogger.LogSensitiveDataRedaction("", field.Key, "field value redacted during processing")
		}
	}

	return result
}

// applyMessageSecurity applies sensitive data filtering to the raw message (before formatting)
func (l *Logger) applyMessageSecurity(message string) string {
	secConfig := l.getSecurityConfig()
	if secConfig == nil {
		return internal.SanitizeControlChars(message)
	}

	if secConfig.SensitiveFilter != nil && secConfig.SensitiveFilter.IsEnabled() {
		message = secConfig.SensitiveFilter.Filter(message)
	}

	return internal.SanitizeControlChars(message)
}

// applySizeLimit applies message size limit to the formatted message (after formatting)
func (l *Logger) applySizeLimit(message string) string {
	secConfig := l.getSecurityConfig()
	if secConfig == nil {
		return message
	}

	if secConfig.MaxMessageSize > 0 && len(message) > secConfig.MaxMessageSize {
		// Truncate at a valid UTF-8 rune boundary to avoid corrupting multi-byte characters
		truncIdx := secConfig.MaxMessageSize
		for truncIdx > 0 && !utf8.RuneStart(message[truncIdx]) {
			truncIdx--
		}
		message = message[:truncIdx] + "..."
	}

	return message
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
//
// Example:
//
//	// Enable strict snake_case validation
//	logger.SetFieldValidation(dd.StrictSnakeCaseConfig())
func (l *Logger) SetFieldValidation(config *FieldValidationConfig) {
	if config == nil || config.Mode == FieldValidationNone {
		l.fieldValidation.Store(nil)
	} else {
		l.fieldValidation.Store(config)
	}
}

// GetFieldValidation returns the current field validation configuration.
// Returns nil if no validation is configured.
func (l *Logger) GetFieldValidation() *FieldValidationConfig {
	return l.getFieldValidation()
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

	// Load current writers slice
	currentWriters := l.writersPtr.Load()
	if currentWriters == nil {
		return ErrLoggerClosed
	}

	if len(*currentWriters) >= maxWriterCount {
		return ErrMaxWritersExceeded
	}

	// Create new slice with the new writer added
	newWriters := make([]io.Writer, len(*currentWriters)+1)
	copy(newWriters, *currentWriters)
	newWriters[len(*currentWriters)] = writer

	// Atomically swap the pointer
	l.writersPtr.Store(&newWriters)
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

	// Load current writers slice
	currentWriters := l.writersPtr.Load()
	if currentWriters == nil {
		return ErrLoggerClosed
	}

	writerCount := len(*currentWriters)
	for i := 0; i < writerCount; i++ {
		if (*currentWriters)[i] == writer {
			// Create new slice without the removed writer
			newWriters := make([]io.Writer, writerCount-1)
			copy(newWriters, (*currentWriters)[:i])
			copy(newWriters[i:], (*currentWriters)[i+1:])

			// Atomically swap the pointer
			l.writersPtr.Store(&newWriters)
			return nil
		}
	}

	return ErrWriterNotFound
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
func (l *Logger) Flush() error {
	writersPtr := l.writersPtr.Load()
	if writersPtr == nil {
		return nil
	}

	var firstErr error
	for _, w := range *writersPtr {
		if flusher, ok := w.(Flusher); ok {
			if err := flusher.Flush(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// writeMessage writes a message to all configured writers.
// Uses unsafe pointer conversion to avoid string-to-bytes copy when possible.
func (l *Logger) writeMessage(message string) {
	if l.closed.Load() || len(message) == 0 {
		return
	}

	msgLen := len(message)

	// Fast path for single writer: use unsafe to avoid string->[]byte copy.
	// Write message and newline as two separate calls to avoid any allocation.
	// The string's backing array is read-only but safe to read via io.Writer.
	writersPtr := l.writersPtr.Load()
	if writersPtr == nil || len(*writersPtr) == 0 {
		return
	}

	writers := *writersPtr
	writerCount := len(writers)

	// Use zero-copy path for messages that fit within reasonable size.
	// This avoids the pool Get/Put overhead and the string copy entirely.
	if writerCount == 1 {
		w := writers[0]
		// Convert string to []byte without copy using unsafe
		msgBytes := unsafe.Slice(unsafe.StringData(message), msgLen)
		if _, err := l.safeWrite(w, msgBytes); err != nil {
			l.handleWriteError(w, err)
		}
		// Write newline separately to avoid allocating a combined buffer
		if _, err := w.Write(newlineBytes); err != nil {
			l.handleWriteError(w, err)
		}
		return
	}

	// Multi-writer path: use pooled buffer to avoid repeated unsafe conversions
	// (each writer would need its own copy since []byte from unsafe is read-only)
	bufPtr := messagePool.Get().(*[]byte)
	buf := *bufPtr
	defer func() {
		if cap(buf) <= maxBufferSize {
			*bufPtr = buf[:0]
		} else {
			*bufPtr = make([]byte, 0, defaultBufferSize)
		}
		messagePool.Put(bufPtr)
	}()

	needed := msgLen + 1
	if cap(buf) < needed {
		buf = make([]byte, 0, max(needed, defaultBufferSize))
	} else {
		buf = buf[:0]
	}

	buf = append(buf, message...)
	buf = append(buf, '\n')

	for _, writer := range writers {
		if _, err := l.safeWrite(writer, buf); err != nil {
			l.handleWriteError(writer, err)
		}
	}
}

// newlineBytes is a pre-allocated slice for writing trailing newlines.
var newlineBytes = []byte{'\n'}

// handleWriteError handles write errors by calling both legacy handler and hooks.
func (l *Logger) handleWriteError(writer io.Writer, err error) {
	// Call legacy write error handler
	if handler := l.getWriteErrorHandler(); handler != nil {
		handler(writer, err)
	}

	// Trigger OnError hook
	hookCtx := &HookContext{
		Event:     HookOnError,
		Error:     err,
		Writer:    writer,
		Timestamp: time.Now(),
	}
	_ = l.triggerHooks(l.ctx, hookCtx)
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

// closeWritersLocked closes all writers. Must be called with writersMu held.
// If ctx is provided (non-nil), it checks for cancellation between writer closes.
func (l *Logger) closeWritersLocked(ctx context.Context) error {
	currentWriters := l.writersPtr.Swap(nil)
	if currentWriters == nil {
		return nil
	}

	errs := make([]error, 0, len(*currentWriters))
	for _, writer := range *currentWriters {
		// Check context for cancellation (Shutdown path)
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
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

	// Close audit logger
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

	return l.closeWritersLocked(nil) //nolint:staticcheck // intentional: Close() has no context
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

		// Close audit logger
		if l.auditLogger != nil {
			_ = l.auditLogger.Close()
		}

		// Close writers BEFORE canceling internal context.
		// This preserves ctx's deadline semantics so closeWritersLocked
		// can respond to the caller's timeout rather than immediately
		// seeing a canceled context from l.cancel().
		l.writersMu.Lock()
		closeErr := l.closeWritersLocked(ctx)
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
		fmt.Fprintln(os.Stderr, "[dd] Warning: logger close timed out after 5 seconds")
	}

	if l.fatalHandler != nil {
		l.fatalHandler()
	} else {
		os.Exit(1)
	}
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
	message := l.formatter.FormatWithMessage(level, callerDepth, entry.msg, allFields)
	l.writeMessage(l.applySizeLimit(message))

	// Trigger AfterLog hook (only if hooks exist)
	if hasHooks {
		hookCtx.Event = HookAfterLog
		_ = l.triggerHooks(context.Background(), hookCtx)
	}

	if level == LevelFatal {
		l.handleFatal()
	}
}

// Log logs a message at the specified level
func (l *Logger) Log(level LogLevel, args ...any) {
	if l == nil {
		return
	}
	if !l.shouldLog(level) {
		return
	}

	msg := l.applyMessageSecurity(l.formatter.FormatArgsToString(args...))
	l.logCore(level, logEntry{msg: msg})
}

// Logf logs a formatted message at the specified level
func (l *Logger) Logf(level LogLevel, format string, args ...any) {
	if l == nil {
		return
	}
	if !l.shouldLog(level) {
		return
	}

	msg := l.applyMessageSecurity(fmt.Sprintf(format, args...))
	l.logCore(level, logEntry{msg: msg})
}

// LogWith logs a structured message with fields at the specified level
func (l *Logger) LogWith(level LogLevel, msg string, fields ...Field) {
	if l == nil {
		return
	}
	if !l.shouldLog(level) {
		return
	}

	// Only copy original fields if hooks are registered (they may need them)
	var originalFields []Field
	if l.loadHooks() != nil && len(fields) > 0 {
		originalFields = make([]Field, len(fields))
		copy(originalFields, fields)
	}

	msg = l.applyMessageSecurity(msg)
	processedFields := l.processFields(fields)

	l.logCore(level, logEntry{
		msg:            msg,
		fields:         processedFields,
		originalFields: originalFields,
	})
}

// Debug logs a message at DEBUG level.
func (l *Logger) Debug(args ...any) { l.Log(LevelDebug, args...) }

// Info logs a message at INFO level.
func (l *Logger) Info(args ...any) { l.Log(LevelInfo, args...) }

// Warn logs a message at WARN level.
func (l *Logger) Warn(args ...any) { l.Log(LevelWarn, args...) }

// Error logs a message at ERROR level.
func (l *Logger) Error(args ...any) { l.Log(LevelError, args...) }

// Fatal logs a message at FATAL level and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute. For graceful shutdown, use Error() with custom logic.
func (l *Logger) Fatal(args ...any) { l.Log(LevelFatal, args...) }

// Debugf logs a formatted message at DEBUG level.
func (l *Logger) Debugf(format string, args ...any) { l.Logf(LevelDebug, format, args...) }

// Infof logs a formatted message at INFO level.
func (l *Logger) Infof(format string, args ...any) { l.Logf(LevelInfo, format, args...) }

// Warnf logs a formatted message at WARN level.
func (l *Logger) Warnf(format string, args ...any) { l.Logf(LevelWarn, format, args...) }

// Errorf logs a formatted message at ERROR level.
func (l *Logger) Errorf(format string, args ...any) { l.Logf(LevelError, format, args...) }

// Fatalf logs a formatted message at FATAL level and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute. For graceful shutdown, use Errorf() with custom logic.
func (l *Logger) Fatalf(format string, args ...any) { l.Logf(LevelFatal, format, args...) }

// DebugWith logs a structured message with fields at DEBUG level.
func (l *Logger) DebugWith(msg string, fields ...Field) { l.LogWith(LevelDebug, msg, fields...) }

// InfoWith logs a structured message with fields at INFO level.
func (l *Logger) InfoWith(msg string, fields ...Field) { l.LogWith(LevelInfo, msg, fields...) }

// WarnWith logs a structured message with fields at WARN level.
func (l *Logger) WarnWith(msg string, fields ...Field) { l.LogWith(LevelWarn, msg, fields...) }

// ErrorWith logs a structured message with fields at ERROR level.
func (l *Logger) ErrorWith(msg string, fields ...Field) { l.LogWith(LevelError, msg, fields...) }

// FatalWith logs a structured message at FATAL level and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute. For graceful shutdown, use ErrorWith() with custom logic.
func (l *Logger) FatalWith(msg string, fields ...Field) { l.LogWith(LevelFatal, msg, fields...) }

// fmt package replacement methods - output via logger's writers with caller info
//
// IMPORTANT: These Logger methods are DIFFERENT from the package-level dd.Print functions!
//
//	logger.Print()  -> writes to configured writers with security filtering
//	dd.Print()      -> writes directly to stdout WITHOUT security filtering (debug only)
//
// Always use logger.Print/Printf/Println for production logging.
//
// DESIGN NOTE: Print() and Println() behave identically in this library.
// Unlike the standard fmt package where Println adds spaces and newline while Print does not,
// both methods here produce identical output because the underlying Log() method
// always adds a trailing newline. This is intentional for consistency with the
// structured logging pattern where all log entries end with a newline.

// Print writes to configured writers with caller info and newline.
// Uses LevelInfo for filtering. Arguments are joined with spaces.
// Applies sensitive data filtering based on SecurityConfig.
func (l *Logger) Print(args ...any) {
	l.Log(LevelInfo, args...)
}

// Println writes to configured writers with caller info and newline.
// Uses LevelInfo for filtering. Applies sensitive data filtering based on SecurityConfig.
//
// Note: Behaves identically to Print() because the underlying Log() method
// always adds a trailing newline. This differs from fmt.Println behavior.
func (l *Logger) Println(args ...any) {
	l.Log(LevelInfo, args...)
}

// Printf formats according to a format specifier and writes to configured writers with caller info.
// Uses LevelInfo for filtering.
func (l *Logger) Printf(format string, args ...any) {
	l.Logf(LevelInfo, format, args...)
}

// Debug utilities - Text and JSON output for debugging

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
	defaultLogger       atomic.Pointer[Logger]
	defaultOnce         sync.Once
	defaultInitErr      atomic.Value // stores error from initialization (errNoInit means no error)

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

				// Create fallback logger using standard initialization path
				// This ensures all future initialization logic is included
				fallbackCfg := defaultConfig()
				fallbackInternalCfg := &internalConfig{
					level:          fallbackCfg.Level,
					format:         fallbackCfg.Format,
					timeFormat:     fallbackCfg.TimeFormat,
					includeTime:    fallbackCfg.IncludeTime,
					includeLevel:   fallbackCfg.IncludeLevel,
					fullPath:       fallbackCfg.FullPath,
					dynamicCaller:  fallbackCfg.DynamicCaller,
					writers:        []io.Writer{os.Stderr},
					json:           fallbackCfg.JSON,
					securityConfig: fallbackCfg.Security,
					fatalHandler:   fallbackCfg.FatalHandler,
				}
				// newFromInternalConfig always returns nil error, so we can safely ignore it
				logger, _ = newFromInternalConfig(fallbackInternalCfg)
			}
			defaultLogger.Store(logger)
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

	oldLogger := defaultLogger.Swap(logger)

	if oldLogger != nil {
		backgroundCloseWg.Add(1)
		go func() {
			defer backgroundCloseWg.Done()
			time.Sleep(defaultLoggerCloseDelay)
			_ = oldLogger.Close()
		}()
	}
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
	if oldLogger != nil {
		backgroundCloseWg.Add(1)
		go func() {
			defer backgroundCloseWg.Done()
			time.Sleep(defaultLoggerCloseDelay)
			_ = oldLogger.Close()
		}()
	}

	// Clear any previous initialization error
	defaultInitErr.Store(errNoInit)

	return nil
}

// waitForBackgroundCloses waits for all background goroutines spawned by
// SetDefault and InitDefault to close old loggers. This is used internally
// for graceful shutdown to ensure no goroutines are leaked.
//
// Returns true if all goroutines completed, false if the timeout was reached.
func waitForBackgroundCloses(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		backgroundCloseWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// ============================================================================
// Package-level Logging Functions (use default logger)
// ============================================================================

// Debug logs a message at DEBUG level using the default logger.
func Debug(args ...any) { Default().Log(LevelDebug, args...) }

// Info logs a message at INFO level using the default logger.
func Info(args ...any) { Default().Log(LevelInfo, args...) }

// Warn logs a message at WARN level using the default logger.
func Warn(args ...any) { Default().Log(LevelWarn, args...) }

// Error logs a message at ERROR level using the default logger.
func Error(args ...any) { Default().Log(LevelError, args...) }

// Fatal logs a message at FATAL level using the default logger and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute. For graceful shutdown, use Error() with custom logic.
func Fatal(args ...any) { Default().Log(LevelFatal, args...) }

// Debugf logs a formatted message at DEBUG level using the default logger.
func Debugf(format string, args ...any) { Default().Logf(LevelDebug, format, args...) }

// Infof logs a formatted message at INFO level using the default logger.
func Infof(format string, args ...any) { Default().Logf(LevelInfo, format, args...) }

// Warnf logs a formatted message at WARN level using the default logger.
func Warnf(format string, args ...any) { Default().Logf(LevelWarn, format, args...) }

// Errorf logs a formatted message at ERROR level using the default logger.
func Errorf(format string, args ...any) { Default().Logf(LevelError, format, args...) }

// Fatalf logs a formatted message at FATAL level using the default logger and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute. For graceful shutdown, use Errorf() with custom logic.
func Fatalf(format string, args ...any) { Default().Logf(LevelFatal, format, args...) }

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
func Log(level LogLevel, args ...any) { Default().Log(level, args...) }

// Logf logs a formatted message at the specified level using the default logger.
func Logf(level LogLevel, format string, args ...any) {
	Default().Logf(level, format, args...)
}

// LogWith logs a structured message at the specified level using the default logger.
func LogWith(level LogLevel, msg string, fields ...Field) {
	Default().LogWith(level, msg, fields...)
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
