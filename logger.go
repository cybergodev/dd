// Package dd provides a high-performance, thread-safe logging library.
package dd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"

	"github.com/cybergodev/dd/internal/types"
)

var (
	// Simplified message buffer pool - over-engineering removed
	messagePool = sync.Pool{
		New: func() any {
			buf := make([]byte, 0, DefaultBufferSize)
			return &buf
		},
	}
)

// LogLevel is an alias for the shared type
type LogLevel = types.LogLevel

// Log level constants
const (
	LevelDebug = types.LevelDebug
	LevelInfo  = types.LevelInfo
	LevelWarn  = types.LevelWarn
	LevelError = types.LevelError
	LevelFatal = types.LevelFatal
)

type FatalHandler func()

// Logger provides high-performance, thread-safe logging with structured fields support.
// All public methods are goroutine-safe and can be called concurrently.
type Logger struct {
	// Atomic fields first for optimal memory alignment
	level  atomic.Int32
	closed atomic.Bool

	// Immutable configuration (set once during initialization)
	callerDepth  int
	fatalHandler FatalHandler
	formatter    *MessageFormatter

	// Mutable state protected by synchronization
	writers        []io.Writer
	mu             sync.RWMutex
	securityConfig atomic.Value // *SecurityConfig

	// Lifecycle management
	closeOnce sync.Once
	ctx       context.Context
	cancel    context.CancelFunc
}

// New creates a new Logger with the given configuration.
// All public methods of the returned Logger are goroutine-safe.
func New(configs ...*LoggerConfig) (*Logger, error) {
	var config *LoggerConfig
	if len(configs) == 0 || configs[0] == nil {
		config = DefaultConfig()
	} else {
		config = configs[0]
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid logger configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	l := &Logger{
		callerDepth:  DefaultCallerDepth,
		fatalHandler: config.FatalHandler,
		formatter:    newMessageFormatter(config),
		writers:      make([]io.Writer, 0, len(config.Writers)),
		ctx:          ctx,
		cancel:       cancel,
	}

	// Set atomic values
	l.level.Store(int32(config.Level))
	l.securityConfig.Store(config.SecurityConfig)

	// Add writers if provided
	if config.Writers != nil {
		for _, writer := range config.Writers {
			if err := l.AddWriter(writer); err != nil {
				cancel()
				return nil, fmt.Errorf("failed to add writer: %w", err)
			}
		}
	}

	return l, nil
}

// GetLevel returns the current log level (thread-safe).
//
//go:inline
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

// SetSecurityConfig atomically sets the security configuration (thread-safe).
func (l *Logger) SetSecurityConfig(config *SecurityConfig) {
	if config == nil {
		config = DefaultSecurityConfig()
	}
	l.securityConfig.Store(config)
}

// GetSecurityConfig returns a copy of the current security configuration (thread-safe).
func (l *Logger) GetSecurityConfig() *SecurityConfig {
	if config := l.securityConfig.Load(); config != nil {
		return config.(*SecurityConfig)
	}
	return DefaultSecurityConfig()
}

// AddWriter adds a writer to the logger in a thread-safe manner.
func (l *Logger) AddWriter(writer io.Writer) error {
	if writer == nil {
		return ErrNilWriter
	}

	if l.closed.Load() {
		return ErrLoggerClosed
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Check again after acquiring lock
	if l.closed.Load() {
		return ErrLoggerClosed
	}

	if len(l.writers) >= MaxWriterCount {
		return ErrMaxWritersExceeded
	}

	l.writers = append(l.writers, writer)
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

	l.mu.Lock()
	defer l.mu.Unlock()

	// Check again after acquiring lock
	if l.closed.Load() {
		return ErrLoggerClosed
	}

	for i, w := range l.writers {
		if w == writer {
			// Remove writer by swapping with last element and truncating
			l.writers[i] = l.writers[len(l.writers)-1]
			l.writers = l.writers[:len(l.writers)-1]
			return nil
		}
	}

	return fmt.Errorf("writer not found")
}

// Close closes the logger and all associated resources (thread-safe).
func (l *Logger) Close() error {
	var closeErr error

	l.closeOnce.Do(func() {
		l.closed.Store(true)
		l.cancel()

		l.mu.Lock()
		defer l.mu.Unlock()

		// Close all closeable writers (except standard streams)
		for _, writer := range l.writers {
			if closer, ok := writer.(io.Closer); ok {
				// Don't close standard streams (stdout, stderr, stdin)
				if writer != os.Stdout && writer != os.Stderr && writer != os.Stdin {
					if err := closer.Close(); err != nil && closeErr == nil {
						closeErr = fmt.Errorf("failed to close writer: %w", err)
					}
				}
			}
		}

		// Clear writers slice
		l.writers = nil
	})

	return closeErr
}

// shouldLog checks if a message should be logged based on level and logger state
func (l *Logger) shouldLog(level LogLevel) bool {
	// Optimize: check level first (most common filter), then closed state
	currentLevel := LogLevel(l.level.Load())
	if level < currentLevel || level < LevelDebug || level > LevelFatal {
		return false
	}
	return !l.closed.Load()
}

// getSecurityConfig returns the current security configuration
func (l *Logger) getSecurityConfig() *SecurityConfig {
	if config := l.securityConfig.Load(); config != nil {
		return config.(*SecurityConfig)
	}
	return DefaultSecurityConfig()
}

// Log logs a message at the specified level
func (l *Logger) Log(level LogLevel, args ...any) {
	if !l.shouldLog(level) {
		return
	}

	msg := fmt.Sprint(args...)
	message := l.formatter.formatMessage(level, l.callerDepth, msg)
	l.writeMessage(l.applySecurity(message))

	if level == LevelFatal {
		l.handleFatal()
	}
}

// Logf logs a formatted message at the specified level
func (l *Logger) Logf(level LogLevel, format string, args ...any) {
	if !l.shouldLog(level) {
		return
	}

	message := l.formatter.formatMessagef(level, l.callerDepth, format, args...)
	l.writeMessage(l.applySecurity(message))

	if level == LevelFatal {
		l.handleFatal()
	}
}

// LogWith logs a structured message with fields at the specified level
func (l *Logger) LogWith(level LogLevel, msg string, fields ...Field) {
	if !l.shouldLog(level) {
		return
	}

	processedFields := l.processFields(fields)
	message := l.formatter.formatMessageWith(level, l.callerDepth, msg, processedFields)
	l.writeMessage(l.applySecurity(message))

	if level == LevelFatal {
		l.handleFatal()
	}
}

// processFields processes and filters structured fields
func (l *Logger) processFields(fields []Field) []Field {
	if len(fields) == 0 {
		return fields
	}

	secConfig := l.getSecurityConfig()
	if secConfig == nil || secConfig.SensitiveFilter == nil || !secConfig.SensitiveFilter.IsEnabled() {
		return fields
	}

	// Apply security filtering to field values
	filtered := make([]Field, len(fields))
	for i, field := range fields {
		filtered[i] = Field{
			Key:   field.Key,
			Value: secConfig.SensitiveFilter.FilterFieldValue(field.Key, field.Value),
		}
	}

	return filtered
}

// applySecurity applies security measures to the final message
func (l *Logger) applySecurity(message string) string {
	secConfig := l.getSecurityConfig()
	if secConfig == nil {
		return message
	}

	// Apply message size limit
	if secConfig.MaxMessageSize > 0 && len(message) > secConfig.MaxMessageSize {
		message = message[:secConfig.MaxMessageSize] + "..."
	}

	// Apply sensitive data filtering
	if secConfig.SensitiveFilter != nil && secConfig.SensitiveFilter.IsEnabled() {
		message = secConfig.SensitiveFilter.Filter(message)
	}

	return sanitizeControlChars(message)
}

// sanitizeControlChars removes control characters from the message
func sanitizeControlChars(message string) string {
	if len(message) == 0 {
		return message
	}

	// Fast path: check if sanitization is needed
	hasControlChars := false
	for i := 0; i < len(message); i++ {
		if isControlChar(message[i]) {
			hasControlChars = true
			break
		}
	}

	if !hasControlChars {
		return message
	}

	// Slow path: remove control characters
	result := make([]byte, 0, len(message))
	for i := 0; i < len(message); i++ {
		c := message[i]
		if !isControlChar(c) {
			result = append(result, c)
		}
	}

	return string(result)
}

// isControlChar checks if a byte is a control character
func isControlChar(c byte) bool {
	return c == '\x00' || (c < 32 && c != '\n' && c != '\r' && c != '\t') || c == 127
}

// handleFatal handles fatal log messages
func (l *Logger) handleFatal() {
	_ = l.Close()
	if l.fatalHandler != nil {
		l.fatalHandler()
	} else {
		os.Exit(1)
	}
}

// writeMessage writes a message to all configured writers
func (l *Logger) writeMessage(message string) {
	if l.closed.Load() || len(message) == 0 {
		return
	}

	// Get buffer from pool
	bufPtr := messagePool.Get().(*[]byte)
	buf := *bufPtr
	defer func() {
		if cap(buf) <= MaxBufferSize {
			*bufPtr = buf[:0] // Reset length but keep capacity
			messagePool.Put(bufPtr)
		}
	}()

	// Prepare message buffer
	needed := len(message) + 1
	if cap(buf) < needed {
		buf = make([]byte, 0, max(needed, DefaultBufferSize))
	} else {
		buf = buf[:0]
	}

	buf = append(buf, message...)
	buf = append(buf, '\n')

	// Get writers list
	l.mu.RLock()
	writerCount := len(l.writers)
	if writerCount == 0 {
		l.mu.RUnlock()
		return
	}

	// Optimize: single writer fast path
	if writerCount == 1 {
		w := l.writers[0]
		l.mu.RUnlock()
		_, _ = w.Write(buf)
		return
	}

	// Multiple writers: copy slice then release lock
	writers := make([]io.Writer, writerCount)
	copy(writers, l.writers)
	l.mu.RUnlock()

	// Write to all writers
	for _, writer := range writers {
		_, _ = writer.Write(buf)
	}
}

// Convenience logging methods
func (l *Logger) Debug(args ...any) { l.Log(LevelDebug, args...) }
func (l *Logger) Info(args ...any)  { l.Log(LevelInfo, args...) }
func (l *Logger) Warn(args ...any)  { l.Log(LevelWarn, args...) }
func (l *Logger) Error(args ...any) { l.Log(LevelError, args...) }
func (l *Logger) Fatal(args ...any) { l.Log(LevelFatal, args...) }

func (l *Logger) Debugf(format string, args ...any) { l.Logf(LevelDebug, format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.Logf(LevelInfo, format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.Logf(LevelWarn, format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.Logf(LevelError, format, args...) }
func (l *Logger) Fatalf(format string, args ...any) { l.Logf(LevelFatal, format, args...) }

func (l *Logger) DebugWith(msg string, fields ...Field) { l.LogWith(LevelDebug, msg, fields...) }
func (l *Logger) InfoWith(msg string, fields ...Field)  { l.LogWith(LevelInfo, msg, fields...) }
func (l *Logger) WarnWith(msg string, fields ...Field)  { l.LogWith(LevelWarn, msg, fields...) }
func (l *Logger) ErrorWith(msg string, fields ...Field) { l.LogWith(LevelError, msg, fields...) }
func (l *Logger) FatalWith(msg string, fields ...Field) { l.LogWith(LevelFatal, msg, fields...) }

// Global default logger management
var (
	defaultLogger atomic.Pointer[Logger]
	defaultOnce   sync.Once
)

// Default returns the default global logger (thread-safe)
func Default() *Logger {
	defaultOnce.Do(func() {
		logger, err := New(nil)
		if err != nil {
			// Create a minimal fallback logger that always works
			ctx, cancel := context.WithCancel(context.Background())
			logger = &Logger{
				callerDepth: DefaultCallerDepth,
				formatter:   newMessageFormatter(DefaultConfig()),
				writers:     []io.Writer{os.Stderr},
				ctx:         ctx,
				cancel:      cancel,
			}
			logger.level.Store(int32(LevelInfo))
			logger.securityConfig.Store(DefaultSecurityConfig())
		}
		defaultLogger.Store(logger)
	})

	return defaultLogger.Load()
}

// SetDefault sets the default global logger (thread-safe)
func SetDefault(logger *Logger) {
	if logger != nil {
		defaultLogger.Store(logger)
	}
}

// Package-level convenience functions
func Debug(args ...any)                 { Default().Log(LevelDebug, args...) }
func Info(args ...any)                  { Default().Log(LevelInfo, args...) }
func Warn(args ...any)                  { Default().Log(LevelWarn, args...) }
func Error(args ...any)                 { Default().Log(LevelError, args...) }
func Fatal(args ...any)                 { Default().Log(LevelFatal, args...) }
func Debugf(format string, args ...any) { Default().Logf(LevelDebug, format, args...) }
func Infof(format string, args ...any)  { Default().Logf(LevelInfo, format, args...) }
func Warnf(format string, args ...any)  { Default().Logf(LevelWarn, format, args...) }
func Errorf(format string, args ...any) { Default().Logf(LevelError, format, args...) }
func Fatalf(format string, args ...any) { Default().Logf(LevelFatal, format, args...) }
func SetLevel(level LogLevel)           { _ = Default().SetLevel(level) }
