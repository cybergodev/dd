// Package dd provides a high-performance, thread-safe logging library.
package dd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cybergodev/dd/internal/jsonformat"
	"github.com/cybergodev/dd/internal/logformat"
)

var (
	messagePool = sync.Pool{
		New: func() any {
			buf := make([]byte, 0, 1024)
			return &buf
		},
	}
)

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
	// 原子操作字段放在前面，优化内存对齐和缓存性能
	level  atomic.Int32
	closed atomic.Bool

	// 不可变配置，初始化后不再修改
	format        LogFormat
	timeFormat    string
	callerDepth   int
	includeCaller bool
	includeTime   bool
	includeLevel  bool
	fullPath      bool
	dynamicCaller bool
	jsonConfig    *JSONOptions
	fatalHandler  FatalHandler

	// 需要保护的可变状态
	writers        []io.Writer
	mu             sync.RWMutex
	securityConfig atomic.Value

	// 生命周期管理
	closeOnce sync.Once
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
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

// GetLevel returns the current log level (inlined for performance)
//
//go:inline
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
	secConfig := l.getSecurityConfig()
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
	secConfig, ok := v.(*SecurityConfig)
	if !ok {
		return nil
	}
	return secConfig
}

func (l *Logger) AddWriter(w io.Writer) error {
	if w == nil {
		return ErrNilWriter
	}

	if l.closed.Load() {
		return ErrLoggerClosed
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	secConfig := l.getSecurityConfig()
	if secConfig != nil && secConfig.MaxWriters > 0 {
		if len(l.writers) >= secConfig.MaxWriters {
			return fmt.Errorf("%w (%d)", ErrMaxWritersExceeded, secConfig.MaxWriters)
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

	for i := range l.writers {
		if l.writers[i] == w {
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
	message := l.formatMessageWithDepth(level, msg, fieldMap, callerDepth)
	l.writeMessage(message)

	if level == LevelFatal {
		l.handleFatal()
	}
}

func (l *Logger) processFields(fields []Field) map[string]any {
	fieldCount := len(fields)
	if fieldCount == 0 {
		return nil
	}

	// 预分配 map 容量，减少重新分配
	fieldMap := make(map[string]any, fieldCount)

	// 获取安全配置，避免重复调用
	secConfig := l.getSecurityConfig()
	var filter *SensitiveDataFilter
	if secConfig != nil && secConfig.SensitiveFilter != nil && secConfig.SensitiveFilter.IsEnabled() {
		filter = secConfig.SensitiveFilter
	}

	// 分离有过滤器和无过滤器的处理路径，提升性能
	if filter == nil {
		// 快速路径：无过滤器
		for i := range fieldCount {
			field := fields[i]
			key := field.Key

			if len(key) > 0 && len(key) <= maxFieldKeyLength && !needsSanitizationFast(key) {
				fieldMap[key] = field.Value
			} else {
				if needsSanitization(key) {
					key = sanitizeFieldKey(key)
				}
				fieldMap[key] = field.Value
			}
		}
	} else {
		// 慢速路径：有过滤器
		for i := range fieldCount {
			field := fields[i]
			key := field.Key

			if len(key) > 0 && len(key) <= maxFieldKeyLength && !needsSanitizationFast(key) {
				fieldMap[key] = filter.FilterFieldValue(key, field.Value)
			} else {
				if needsSanitization(key) {
					key = sanitizeFieldKey(key)
				}
				fieldMap[key] = filter.FilterFieldValue(field.Key, field.Value)
			}
		}
	}

	return fieldMap
}

const (
	maxFieldKeyLength  = 256
	poolMaxBufferSize  = 4096
	defaultCallerDepth = 6
	maxCallerDepth     = 15
	minPoolCapacity    = 1024
	closeTimeout       = 5 * time.Second
)

// 快速检查字符是否需要清理的查找表
var needsCleanupTable [256]bool

func init() {
	// 初始化查找表
	for i := range 256 {
		c := byte(i)
		// 标记需要清理的字符（与 isValidKeyChar 逻辑相反）
		needsCleanupTable[i] = !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.')
	}
}

// 快速检查是否需要清理（用于热路径）
func needsSanitizationFast(key string) bool {
	keyLen := len(key)
	if keyLen == 0 || keyLen > maxFieldKeyLength {
		return true
	}

	// 快速检查前几个字符
	checkLen := min(keyLen, 8)

	for i := range checkLen {
		if needsCleanupTable[key[i]] {
			return true
		}
	}
	return false
}

func needsSanitization(key string) bool {
	keyLen := len(key)
	if keyLen == 0 || keyLen > maxFieldKeyLength {
		return true
	}

	for i := range keyLen {
		if !isValidKeyChar(key[i]) {
			return true
		}
	}
	return false
}

func (l *Logger) filterMessage(msg string) string {
	secConfig := l.getSecurityConfig()
	if secConfig == nil || secConfig.SensitiveFilter == nil {
		return msg
	}

	if secConfig.SensitiveFilter.IsEnabled() {
		return secConfig.SensitiveFilter.Filter(msg)
	}
	return msg
}

func (l *Logger) shouldLog(level LogLevel) bool {
	// 优化：先检查级别（最常见的过滤条件），然后检查关闭状态
	// 避免不必要的原子操作
	currentLevel := LogLevel(l.level.Load())
	if level < currentLevel || level < LevelDebug || level > LevelFatal {
		return false
	}
	return !l.closed.Load()
}

func (l *Logger) Log(level LogLevel, args ...any) {
	if !l.shouldLog(level) {
		return
	}

	msg := fmt.Sprintln(args...)
	if msgLen := len(msg); msgLen > 0 && msg[msgLen-1] == '\n' {
		msg = msg[:msgLen-1]
	}

	message := l.formatMessageWithDepth(level, msg, nil, defaultCallerDepth)
	l.writeMessage(message)

	if level == LevelFatal {
		l.handleFatal()
	}
}

func (l *Logger) formatMessageWithDepth(level LogLevel, msg string, fields map[string]any, callerDepth int) string {
	// 预过滤消息以避免重复处理
	msg = l.filterMessage(msg)

	var formatted string
	if l.format == FormatJSON {
		formatted = l.formatJSONWithDepth(level, msg, fields, callerDepth)
	} else {
		formatted = l.formatTextWithDepth(level, msg, callerDepth)
	}

	return l.applySecurity(formatted)
}

func (l *Logger) applySecurity(message string) string {
	msgLen := len(message)

	secConfig := l.getSecurityConfig()
	if secConfig != nil && secConfig.MaxMessageSize > 0 && msgLen > secConfig.MaxMessageSize {
		message = message[:secConfig.MaxMessageSize] + truncatedSuffix
	}

	return sanitizeControlChars(message)
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

	msg := fmt.Sprintln(args...)
	if msgLen := len(msg); msgLen > 0 && msg[msgLen-1] == '\n' {
		msg = msg[:msgLen-1]
	}

	message := l.formatMessageWithDepth(level, msg, nil, l.callerDepth+extraDepth)
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
	l.writeMessage(message)

	if level == LevelFatal {
		l.handleFatal()
	}
}

func sanitizeControlChars(message string) string {
	msgLen := len(message)
	if msgLen == 0 {
		return message
	}

	// Fast path: check if sanitization is needed
	hasControlChars := false
	for i := range msgLen {
		if isControlChar(message[i]) {
			hasControlChars = true
			break
		}
	}

	if !hasControlChars {
		return message
	}

	// Slow path: remove control characters
	result := make([]byte, 0, msgLen)
	for i := range msgLen {
		c := message[i]
		if !isControlChar(c) {
			result = append(result, c)
		}
	}

	return string(result)
}

func isControlChar(c byte) bool {
	return c == '\x00' || (c < 32 && c != '\n' && c != '\r' && c != '\t') || c == 127
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

	msgLen := len(message)
	if msgLen == 0 {
		return
	}

	// 获取缓冲区
	bufPtr := messagePool.Get().(*[]byte)
	buf := *bufPtr
	defer func() {
		if cap(buf) <= poolMaxBufferSize {
			*bufPtr = buf[:0] // 重置长度但保留容量
			messagePool.Put(bufPtr)
		}
	}()

	// 准备消息缓冲区
	needed := msgLen + 1
	if cap(buf) < needed {
		buf = make([]byte, 0, max(needed, minPoolCapacity))
	} else {
		buf = buf[:0]
	}

	buf = append(buf, message...)
	buf = append(buf, '\n')

	// 获取写入器列表
	l.mu.RLock()
	writerCount := len(l.writers)
	if writerCount == 0 {
		l.mu.RUnlock()
		return
	}

	// 优化：单个写入器的快速路径
	if writerCount == 1 {
		w := l.writers[0]
		l.mu.RUnlock()
		_, _ = w.Write(buf)
		return
	}

	// 多个写入器：复制切片后释放锁
	writers := make([]io.Writer, writerCount)
	copy(writers, l.writers)
	l.mu.RUnlock()

	// 顺序写入多个写入器（保持简单可靠）
	for i := range writerCount {
		_, _ = writers[i].Write(buf)
	}
}

// 简化的调用深度检测，移除复杂的动态检测逻辑
func (l *Logger) detectCallerDepthWithHint(hint int) int {
	if !l.dynamicCaller {
		return hint
	}

	// 简化逻辑：基于提示深度进行有限调整
	// 避免复杂的文件路径检查，减少性能开销
	adjustedDepth := hint
	if hint > 0 && hint < maxCallerDepth-2 {
		adjustedDepth = hint + 1
	}

	return adjustedDepth
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

		select {
		case <-done:
		case <-time.After(closeTimeout):
			errs = append(errs, fmt.Errorf("timeout waiting for background goroutines"))
		}

		l.mu.Lock()
		for i, writer := range l.writers {
			if writer == os.Stdout || writer == os.Stderr {
				continue
			}
			if closer, ok := writer.(io.Closer); ok {
				if closeErr := closer.Close(); closeErr != nil {
					errs = append(errs, fmt.Errorf("writer[%d]: %w", i, closeErr))
				}
			}
		}
		l.mu.Unlock()
	})

	errCount := len(errs)
	if errCount == 0 {
		return nil
	}
	if errCount == 1 {
		return errs[0]
	}

	var errMsg strings.Builder
	errMsg.WriteString("multiple close errors: ")
	for i := range errCount {
		if i > 0 {
			errMsg.WriteString("; ")
		}
		errMsg.WriteString(errs[i].Error())
	}
	return fmt.Errorf("%s", errMsg.String())
}

var (
	defaultLogger atomic.Pointer[Logger]
	defaultOnce   sync.Once
)

// Default returns the default global logger (thread-safe)
// Initializes with DefaultConfig on first call
func Default() *Logger {
	defaultOnce.Do(func() {
		logger, err := New(nil)
		if err != nil {
			ctx, cancel := context.WithCancel(context.Background())
			logger = &Logger{
				format:        FormatText,
				timeFormat:    time.RFC3339,
				callerDepth:   defaultCallerDepth,
				includeCaller: false,
				includeTime:   true,
				includeLevel:  true,
				fullPath:      false,
				writers:       []io.Writer{os.Stderr},
				ctx:           ctx,
				cancel:        cancel,
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
	if logger == nil {
		return
	}
	defaultLogger.Store(logger)
}

// GetDefaultLogger returns the current default logger without initialization
// Returns nil if no default logger has been set
func GetDefaultLogger() *Logger {
	return defaultLogger.Load()
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
