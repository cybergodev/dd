// Package dd provides a high-performance, thread-safe logging library.
package dd

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/dd/internal/jsonformat"
	"github.com/cybergodev/dd/internal/logformat"
)

var messagePool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 1024)
		return &buf
	},
}

type LogLevel int8

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

type FatalHandler func()

type Logger struct {
	level         atomic.Int32
	format        LogFormat
	timeFormat    string
	callerDepth   int
	includeCaller bool
	includeTime   bool
	includeLevel  bool
	fullPath      bool
	dynamicCaller bool

	writers []io.Writer
	mu      sync.RWMutex

	closed    atomic.Bool
	closeOnce sync.Once

	securityConfig atomic.Value
	fatalHandler   FatalHandler

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	jsonConfig *JSONOptions
}

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
		format:        config.Format,
		timeFormat:    config.TimeFormat,
		callerDepth:   defaultCallerDepth,
		includeCaller: config.IncludeCaller,
		includeTime:   config.IncludeTime,
		includeLevel:  config.IncludeLevel,
		fullPath:      config.FullPath,
		writers:       make([]io.Writer, len(config.Writers)),
		fatalHandler:  config.FatalHandler,
		ctx:           ctx,
		cancel:        cancel,
		dynamicCaller: config.DynamicCaller,
	}

	copy(l.writers, config.Writers)
	l.level.Store(int32(config.Level))
	l.securityConfig.Store(config.SecurityConfig)

	if config.Format == FormatJSON {
		if config.JSON != nil {
			l.jsonConfig = config.JSON
		} else {
			l.jsonConfig = DefaultJSONOptions()
		}
	}

	return l, nil
}

func (l *Logger) GetLevel() LogLevel {
	return LogLevel(l.level.Load())
}

func (l *Logger) SetLevel(level LogLevel) {
	l.level.Store(int32(level))
}

func (l *Logger) SetSecurityConfig(config *SecurityConfig) {
	if config == nil {
		return
	}
	l.securityConfig.Store(config)
}

func (l *Logger) GetSecurityConfig() *SecurityConfig {
	v := l.securityConfig.Load()
	if v == nil {
		return nil
	}
	secConfig := v.(*SecurityConfig)
	if secConfig == nil {
		return nil
	}
	clone := &SecurityConfig{
		MaxMessageSize: secConfig.MaxMessageSize,
		MaxWriters:     secConfig.MaxWriters,
	}
	if secConfig.SensitiveFilter != nil {
		clone.SensitiveFilter = secConfig.SensitiveFilter.Clone()
	}
	return clone
}

func (l *Logger) getSecurityConfig() *SecurityConfig {
	v := l.securityConfig.Load()
	if v == nil {
		return nil
	}
	return v.(*SecurityConfig)
}

func (l *Logger) AddWriter(w io.Writer) error {
	if w == nil {
		return fmt.Errorf("writer cannot be nil")
	}

	if l.closed.Load() {
		return fmt.Errorf("logger is closed")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Check writer count limit
	if secConfig := l.getSecurityConfig(); secConfig != nil && secConfig.MaxWriters > 0 {
		if len(l.writers) >= secConfig.MaxWriters {
			return fmt.Errorf("maximum writer count (%d) exceeded", secConfig.MaxWriters)
		}
	}

	l.writers = append(l.writers, w)
	return nil
}

func (l *Logger) RemoveWriter(w io.Writer) {
	if w == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for i, writer := range l.writers {
		if writer == w {
			l.writers = append(l.writers[:i], l.writers[i+1:]...)
			break
		}
	}
}

func (l *Logger) logWithFieldsAndDepth(level LogLevel, msg string, fields []Field, callerDepth int) {
	if !l.shouldLog(level) {
		return
	}

	fieldMap := l.processFields(fields)
	filteredMsg := l.filterMessage(msg)
	message := l.formatMessageWithDepth(level, filteredMsg, fieldMap, callerDepth)
	message = l.applySecurity(message)
	l.writeMessage(message)

	if level == LevelFatal {
		l.handleFatal()
	}
}

func (l *Logger) processFields(fields []Field) map[string]any {
	if len(fields) == 0 {
		return nil
	}

	fieldMap := make(map[string]any, len(fields))
	secConfig := l.getSecurityConfig()
	hasFilter := secConfig != nil && secConfig.SensitiveFilter != nil

	for _, field := range fields {
		safeKey := field.Key
		if needsSanitization(field.Key) {
			safeKey = sanitizeFieldKey(field.Key)
		}
		if hasFilter {
			fieldMap[safeKey] = secConfig.SensitiveFilter.FilterFieldValue(field.Key, field.Value)
		} else {
			fieldMap[safeKey] = field.Value
		}
	}

	return fieldMap
}

const (
	maxFieldKeyLength  = 256
	poolMaxBufferSize  = 4096
	defaultCallerDepth = 6
	maxCallerDepth     = 15
)

func needsSanitization(key string) bool {
	keyLen := len(key)
	if keyLen == 0 || keyLen > maxFieldKeyLength {
		return true
	}

	for i := 0; i < keyLen; i++ {
		c := key[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return true
		}
	}
	return false
}

func (l *Logger) filterMessage(msg string) string {
	secConfig := l.getSecurityConfig()
	if secConfig != nil && secConfig.SensitiveFilter != nil {
		return secConfig.SensitiveFilter.Filter(msg)
	}
	return msg
}

func (l *Logger) shouldLog(level LogLevel) bool {
	return !l.closed.Load() && level >= l.GetLevel() && level >= LevelDebug && level <= LevelFatal
}

func (l *Logger) Log(level LogLevel, args ...any) {
	if !l.shouldLog(level) {
		return
	}

	// Use Sprintln logic to add spaces between args, then trim trailing newline
	msg := fmt.Sprintln(args...)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}

	message := l.formatMessageWithDepth(level, msg, nil, 6)
	message = l.applySecurity(message)
	l.writeMessage(message)

	if level == LevelFatal {
		l.handleFatal()
	}
}

func (l *Logger) formatMessageWithDepth(level LogLevel, msg string, fields map[string]any, callerDepth int) string {
	if l.format == FormatJSON {
		return l.formatJSONWithDepth(level, msg, fields, callerDepth)
	}
	return l.formatTextWithDepth(level, msg, callerDepth)
}

func (l *Logger) formatTextWithDepth(level LogLevel, msg string, callerDepth int) string {
	finalDepth := callerDepth
	if l.dynamicCaller {
		detectedDepth := l.detectCallerDepthWithHint(callerDepth)
		finalDepth = detectedDepth + 2
	}

	return logformat.FormatMessage(
		logformat.LogLevel(level),
		l.includeTime,
		l.timeFormat,
		l.includeLevel,
		l.includeCaller,
		finalDepth,
		l.fullPath,
		msg,
	)
}

func (l *Logger) formatJSONWithDepth(level LogLevel, msg string, fields map[string]any, callerDepth int) string {
	var opts *jsonformat.JSONOptions
	if l.jsonConfig != nil {
		var fieldNames *jsonformat.JSONFieldNames
		if l.jsonConfig.FieldNames != nil {
			fieldNames = &jsonformat.JSONFieldNames{
				Timestamp: l.jsonConfig.FieldNames.Timestamp,
				Level:     l.jsonConfig.FieldNames.Level,
				Caller:    l.jsonConfig.FieldNames.Caller,
				Message:   l.jsonConfig.FieldNames.Message,
				Fields:    l.jsonConfig.FieldNames.Fields,
			}
		}
		opts = &jsonformat.JSONOptions{
			PrettyPrint: l.jsonConfig.PrettyPrint,
			Indent:      l.jsonConfig.Indent,
			FieldNames:  fieldNames,
		}
	}

	finalDepth := callerDepth
	if l.dynamicCaller {
		detectedDepth := l.detectCallerDepthWithHint(callerDepth)
		finalDepth = detectedDepth + 2
	}

	var jsonMsg string
	var err error
	if opts != nil {
		jsonMsg, err = jsonformat.FormatMessageWithOptions(
			jsonformat.LogLevel(level),
			l.includeTime,
			l.timeFormat,
			l.includeLevel,
			l.includeCaller,
			finalDepth,
			l.fullPath,
			msg,
			fields,
			opts,
		)
	} else {
		jsonMsg, err = jsonformat.FormatMessage(
			jsonformat.LogLevel(level),
			l.includeTime,
			l.timeFormat,
			l.includeLevel,
			l.includeCaller,
			finalDepth,
			l.fullPath,
			msg,
			fields,
		)
	}

	if err != nil {
		return fmt.Sprintf(`{"error":"json format failed: %v","message":%q}`, err, msg)
	}
	return jsonMsg
}

func (l *Logger) logWithExtraDepth(level LogLevel, extraDepth int, args ...any) {
	if !l.shouldLog(level) {
		return
	}

	// Use Sprintln logic to add spaces between args, then trim trailing newline
	msg := fmt.Sprintln(args...)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}

	message := l.formatMessageWithDepth(level, msg, nil, l.callerDepth+extraDepth)
	message = l.applySecurity(message)
	l.writeMessage(message)

	if level == LevelFatal {
		l.handleFatal()
	}
}

func (l *Logger) logfWithExtraDepth(level LogLevel, extraDepth int, format string, args ...any) {
	if !l.shouldLog(level) {
		return
	}

	message := l.formatMessageWithDepth(level, fmt.Sprintf(format, args...), nil, l.callerDepth+extraDepth)
	message = l.applySecurity(message)
	l.writeMessage(message)

	if level == LevelFatal {
		l.handleFatal()
	}
}

func (l *Logger) applySecurity(message string) string {
	secConfig := l.getSecurityConfig()
	if secConfig == nil {
		return sanitizeMessage(message, 0)
	}

	if secConfig.SensitiveFilter != nil && secConfig.SensitiveFilter.IsEnabled() {
		message = secConfig.SensitiveFilter.Filter(message)
	}

	maxSize := secConfig.MaxMessageSize
	if maxSize > 0 && len(message) > maxSize {
		message = message[:maxSize] + truncatedSuffix
	}

	return sanitizeMessage(message, maxSize)
}

func (l *Logger) handleFatal() {
	l.Close()
	if l.fatalHandler != nil {
		l.fatalHandler()
	} else {
		os.Exit(1)
	}
}

func (l *Logger) writeMessage(message string) {
	if l.closed.Load() {
		return
	}

	l.mu.RLock()
	writers := l.writers
	writerCount := len(writers)
	l.mu.RUnlock()

	if writerCount == 0 {
		return
	}

	bufPtr := messagePool.Get().(*[]byte)
	buf := *bufPtr

	needed := len(message) + 1
	if cap(buf) < needed {
		newCap := needed
		if newCap < 1024 {
			newCap = 1024
		}
		buf = make([]byte, 0, newCap)
	} else {
		buf = buf[:0]
	}

	buf = append(buf, message...)
	buf = append(buf, '\n')

	if writerCount == 1 && writers[0] != nil {
		_, _ = writers[0].Write(buf)
	} else {
		for _, w := range writers {
			if w != nil {
				_, _ = w.Write(buf)
			}
		}
	}

	if cap(buf) <= poolMaxBufferSize {
		*bufPtr = buf
		messagePool.Put(bufPtr)
	}
}

func (l *Logger) detectCallerDepthWithHint(hint int) int {
	if !l.dynamicCaller {
		return hint
	}

	const pkgPrefix = "github.com/cybergodev/dd"

	for depth := 2; depth <= maxCallerDepth; depth++ {
		pc, file, _, ok := runtime.Caller(depth)
		if !ok {
			return hint
		}

		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}

		funcName := fn.Name()

		if strings.HasPrefix(funcName, "runtime.") {
			continue
		}

		if strings.Contains(funcName, pkgPrefix) && !strings.HasSuffix(file, "_test.go") {
			continue
		}

		return depth - 1
	}

	return hint
}

func (l *Logger) Debug(args ...any) { l.Log(LevelDebug, args...) }
func (l *Logger) Info(args ...any)  { l.Log(LevelInfo, args...) }
func (l *Logger) Warn(args ...any)  { l.Log(LevelWarn, args...) }
func (l *Logger) Error(args ...any) { l.Log(LevelError, args...) }
func (l *Logger) Fatal(args ...any) { l.Log(LevelFatal, args...) }

func (l *Logger) logf(level LogLevel, format string, args ...any) {
	if !l.shouldLog(level) {
		return
	}
	msg := fmt.Sprintf(format, args...)
	message := l.formatMessageWithDepth(level, msg, nil, defaultCallerDepth)
	message = l.applySecurity(message)
	l.writeMessage(message)

	if level == LevelFatal {
		l.handleFatal()
	}
}

func (l *Logger) Debugf(format string, args ...any) { l.logf(LevelDebug, format, args...) }
func (l *Logger) Infof(format string, args ...any)  { l.logf(LevelInfo, format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { l.logf(LevelWarn, format, args...) }
func (l *Logger) Errorf(format string, args ...any) { l.logf(LevelError, format, args...) }
func (l *Logger) Fatalf(format string, args ...any) { l.logf(LevelFatal, format, args...) }

func (l *Logger) Close() error {
	var errs []error

	l.closeOnce.Do(func() {
		l.closed.Store(true)

		if l.cancel != nil {
			l.cancel()
		}

		done := make(chan struct{})
		go func() {
			l.wg.Wait()
			close(done)
		}()

		const closeTimeout = 5 * time.Second
		select {
		case <-done:
		case <-time.After(closeTimeout):
			errs = append(errs, fmt.Errorf("timeout waiting for background goroutines"))
		}

		l.mu.Lock()
		defer l.mu.Unlock()

		for i, writer := range l.writers {
			if writer == os.Stdout || writer == os.Stderr || writer == os.Stdin {
				continue
			}
			if closer, ok := writer.(io.Closer); ok {
				if closeErr := closer.Close(); closeErr != nil {
					errs = append(errs, fmt.Errorf("writer[%d]: %w", i, closeErr))
				}
			}
		}
	})

	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	var errMsg strings.Builder
	errMsg.WriteString("multiple close errors: ")
	for i, err := range errs {
		if i > 0 {
			errMsg.WriteString("; ")
		}
		errMsg.WriteString(err.Error())
	}
	return fmt.Errorf("%s", errMsg.String())
}

var (
	defaultLogger atomic.Value // *Logger
	defaultOnce   sync.Once
)

// Default returns the default global logger (thread-safe)
// Initializes with DefaultConfig on first call
func Default() *Logger {
	defaultOnce.Do(func() {
		logger, err := New(nil)
		if err != nil {
			panic(fmt.Sprintf("dd: failed to initialize default logger: %v", err))
		}
		defaultLogger.Store(logger)
	})

	return defaultLogger.Load().(*Logger)
}

// SetDefault sets the default global logger (thread-safe)
// The old logger is not automatically closed to prevent race conditions
// Users should manually close the old logger if needed
func SetDefault(logger *Logger) {
	if logger == nil {
		return
	}
	defaultLogger.Store(logger)
}

// GetDefaultLogger returns the current default logger without initialization
// Returns nil if no default logger has been set
func GetDefaultLogger() *Logger {
	if logger := defaultLogger.Load(); logger != nil {
		return logger.(*Logger)
	}
	return nil
}

func Debug(args ...any)                 { Default().logWithExtraDepth(LevelDebug, 1, args...) }
func Info(args ...any)                  { Default().logWithExtraDepth(LevelInfo, 1, args...) }
func Warn(args ...any)                  { Default().logWithExtraDepth(LevelWarn, 1, args...) }
func Error(args ...any)                 { Default().logWithExtraDepth(LevelError, 1, args...) }
func Fatal(args ...any)                 { Default().logWithExtraDepth(LevelFatal, 1, args...) }
func Debugf(format string, args ...any) { Default().logfWithExtraDepth(LevelDebug, 1, format, args...) }
func Infof(format string, args ...any)  { Default().logfWithExtraDepth(LevelInfo, 1, format, args...) }
func Warnf(format string, args ...any)  { Default().logfWithExtraDepth(LevelWarn, 1, format, args...) }
func Errorf(format string, args ...any) { Default().logfWithExtraDepth(LevelError, 1, format, args...) }
func Fatalf(format string, args ...any) { Default().logfWithExtraDepth(LevelFatal, 1, format, args...) }
func SetLevel(level LogLevel)           { Default().SetLevel(level) }
