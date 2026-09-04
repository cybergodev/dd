package dd

import (
	"fmt"

	"github.com/cybergodev/dd/internal"
)

// LoggerEntry represents a logger with pre-set fields.
// Fields are inherited and merged with additional fields passed to logging methods.
// LoggerEntry is immutable - each WithFields call returns a new entry.
type LoggerEntry struct {
	logger *Logger
	fields []Field
}

// newLoggerEntry creates a new LoggerEntry with the given logger and fields.
func newLoggerEntry(logger *Logger, fields []Field) *LoggerEntry {
	// Copy fields to ensure immutability
	copiedFields := make([]Field, len(fields))
	copy(copiedFields, fields)
	return &LoggerEntry{
		logger: logger,
		fields: copiedFields,
	}
}

// maxFieldCount limits the maximum number of fields to prevent CPU exhaustion
// from O(n*m) linear search in mergeFieldSlicesSmall.
// This is a reasonable limit for structured logging use cases.
const maxFieldCount = 1000

// mergeFieldSlices combines two field slices, with newFields overriding existingFields.
// This is a shared utility function used by both WithFields and mergeFields.
// Optimization: Uses linear search for small field counts to avoid map allocation.
// SECURITY: Enforces maximum field count to prevent CPU exhaustion attacks.
func mergeFieldSlices(existingFields, newFields []Field) []Field {
	// Fast path: no existing fields
	if len(existingFields) == 0 {
		return newFields
	}
	// Fast path: no new fields
	if len(newFields) == 0 {
		return existingFields
	}

	existingLen := len(existingFields)
	newLen := len(newFields)

	// SECURITY: Enforce maximum field count to prevent CPU exhaustion.
	// Each input slice is capped at maxFieldCount, which bounds the merged
	// result at 2×maxFieldCount and the O(n×m) linear-search work accordingly.
	if existingLen > maxFieldCount {
		existingFields = existingFields[:maxFieldCount]
		existingLen = maxFieldCount
	}
	if newLen > maxFieldCount {
		newFields = newFields[:maxFieldCount]
		newLen = maxFieldCount
	}

	// For small field counts, use linear search to avoid map allocation.
	// Threshold: the linear scan must stay short on both sides, so the small
	// path is capped at newLen <= 4 AND existingLen <= 8 (the O(n×m) work is
	// then bounded); anything larger takes the map-based path below.
	if newLen <= 4 && existingLen <= 8 {
		return mergeFieldSlicesSmall(existingFields, newFields)
	}

	// For larger field counts, use map-based approach
	return mergeFieldSlicesLarge(existingFields, newFields)
}

// mergeFieldSlicesSmall handles merging for small field counts without map allocation.
// Uses linear search which is faster for small N due to cache locality.
func mergeFieldSlicesSmall(existingFields, newFields []Field) []Field {
	merged := make([]Field, 0, len(existingFields)+len(newFields))

	// Add existing fields that aren't overridden (linear search)
	for _, existing := range existingFields {
		overridden := false
		for _, newF := range newFields {
			if newF.Key == existing.Key {
				overridden = true
				break
			}
		}
		if !overridden {
			merged = append(merged, existing)
		}
	}

	// Add all new fields
	merged = append(merged, newFields...)

	return merged
}

// mergeFieldSlicesLarge handles merging for large field counts using a map.
// Map provides O(1) lookup which is faster for larger field counts.
func mergeFieldSlicesLarge(existingFields, newFields []Field) []Field {
	merged := make([]Field, 0, len(existingFields)+len(newFields))

	// Track which keys have been set by new fields
	newKeys := make(map[string]struct{}, len(newFields))
	for _, f := range newFields {
		newKeys[f.Key] = struct{}{}
	}

	// Add existing fields that aren't overridden
	for _, f := range existingFields {
		if _, exists := newKeys[f.Key]; !exists {
			merged = append(merged, f)
		}
	}

	// Add all new fields
	merged = append(merged, newFields...)

	return merged
}

// WithFields returns a new LoggerEntry with additional fields.
// Fields are merged with existing fields, with new fields overriding existing ones.
//
// Example:
//
//	entry := logger.WithFields(dd.String("service", "api"))
//	entry2 := entry.WithFields(dd.String("version", "1.0"))
//	entry2.Info("request received") // Contains both service and version fields
func (e *LoggerEntry) WithFields(fields ...Field) *LoggerEntry {
	if len(fields) == 0 {
		return e
	}

	// Fast path: no existing fields
	if len(e.fields) == 0 {
		return newLoggerEntry(e.logger, fields)
	}

	return newLoggerEntry(e.logger, mergeFieldSlices(e.fields, fields))
}

// WithField returns a new LoggerEntry with a single additional field.
// This is a convenience method equivalent to WithFields with a single field.
//
// Example:
//
//	entry := logger.WithField("request_id", "abc123")
func (e *LoggerEntry) WithField(key string, value any) *LoggerEntry {
	return e.WithFields(Field{Key: key, Value: value})
}

// mergeFields combines entry fields with method fields.
// Method fields can override entry fields with the same key.
func (e *LoggerEntry) mergeFields(fields []Field) []Field {
	return mergeFieldSlices(e.fields, fields)
}

// entryLogDispatch is the LoggerEntry twin of (*Logger).logDispatch: the
// single funnel for the entry's non-structured methods (Log, the level
// wrappers, and the Print family), which must all call it DIRECTLY so the
// entry-dispatch caller capture keeps its fixed frame shape (see
// internal.EntryCaller).
func (e *LoggerEntry) entryLogDispatch(level LogLevel, args ...any) {
	if e == nil || e.logger == nil {
		return
	}
	l := e.logger
	if !l.shouldLog(level) {
		return
	}

	// Entry-dispatch caller capture (see internal.EntryCaller).
	var caller string
	if l.dynamicCaller {
		caller = internal.EntryCaller(l.formatter.FullPath())
	}
	l.logWithLazyMessage(level, func() string {
		return l.formatter.FormatArgsToString(args...)
	}, e.fields, caller, entryCallerDepth)
}

// entryLogfDispatch is entryLogDispatch for the formatted-message family.
// Same frame-shape requirement.
func (e *LoggerEntry) entryLogfDispatch(level LogLevel, format string, args ...any) {
	if e == nil || e.logger == nil {
		return
	}
	l := e.logger
	if !l.shouldLog(level) {
		return
	}

	// Entry-dispatch caller capture (see internal.EntryCaller).
	var caller string
	if l.dynamicCaller {
		caller = internal.EntryCaller(l.formatter.FullPath())
	}
	l.logWithLazyMessage(level, func() string {
		return fmt.Sprintf(format, args...)
	}, e.fields, caller, entryCallerDepth)
}

// entryLogWithDispatch is entryLogDispatch for the structured family.
// Same frame-shape requirement.
func (e *LoggerEntry) entryLogWithDispatch(level LogLevel, msg string, fields ...Field) {
	if e == nil || e.logger == nil {
		return
	}
	l := e.logger
	if !l.shouldLog(level) {
		return
	}

	// Entry-dispatch caller capture (see internal.EntryCaller).
	var caller string
	if l.dynamicCaller {
		caller = internal.EntryCaller(l.formatter.FullPath())
	}
	l.logFiltered(level, msg, e.mergeFields(fields), caller, entryCallerDepth)
}

// Log logs a message at the specified level with the entry's fields.
// The arguments are formatted only after the level gate passes (matching
// (*Logger).Log), so user String()/Error() methods are not invoked when the
// entry is filtered out.
func (e *LoggerEntry) Log(level LogLevel, args ...any) { e.entryLogDispatch(level, args...) }

// Logf logs a formatted message at the specified level with the entry's fields.
// Formatting is deferred until the level gate passes (matching (*Logger).Logf),
// so the format string is not evaluated for filtered-out entries.
func (e *LoggerEntry) Logf(level LogLevel, format string, args ...any) {
	e.entryLogfDispatch(level, format, args...)
}

// LogWith logs a structured message with the entry's fields plus additional fields.
func (e *LoggerEntry) LogWith(level LogLevel, msg string, fields ...Field) {
	e.entryLogWithDispatch(level, msg, fields...)
}

// Convenience methods for each log level

// Debug logs a message at DEBUG level with the entry's pre-set fields.
func (e *LoggerEntry) Debug(args ...any) { e.entryLogDispatch(LevelDebug, args...) }

// Info logs a message at INFO level with the entry's pre-set fields.
func (e *LoggerEntry) Info(args ...any) { e.entryLogDispatch(LevelInfo, args...) }

// Warn logs a message at WARN level with the entry's pre-set fields.
func (e *LoggerEntry) Warn(args ...any) { e.entryLogDispatch(LevelWarn, args...) }

// Error logs a message at ERROR level with the entry's pre-set fields.
func (e *LoggerEntry) Error(args ...any) { e.entryLogDispatch(LevelError, args...) }

// Fatal logs a message at FATAL level with the entry's pre-set fields and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute.
func (e *LoggerEntry) Fatal(args ...any) { e.entryLogDispatch(LevelFatal, args...) }

// Debugf logs a formatted message at DEBUG level with the entry's pre-set fields.
func (e *LoggerEntry) Debugf(format string, args ...any) {
	e.entryLogfDispatch(LevelDebug, format, args...)
}

// Infof logs a formatted message at INFO level with the entry's pre-set fields.
func (e *LoggerEntry) Infof(format string, args ...any) {
	e.entryLogfDispatch(LevelInfo, format, args...)
}

// Warnf logs a formatted message at WARN level with the entry's pre-set fields.
func (e *LoggerEntry) Warnf(format string, args ...any) {
	e.entryLogfDispatch(LevelWarn, format, args...)
}

// Errorf logs a formatted message at ERROR level with the entry's pre-set fields.
func (e *LoggerEntry) Errorf(format string, args ...any) {
	e.entryLogfDispatch(LevelError, format, args...)
}

// Fatalf logs a formatted message at FATAL level with the entry's pre-set fields and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute.
func (e *LoggerEntry) Fatalf(format string, args ...any) {
	e.entryLogfDispatch(LevelFatal, format, args...)
}

// DebugWith logs a structured message with additional fields at DEBUG level.
func (e *LoggerEntry) DebugWith(msg string, fields ...Field) {
	e.entryLogWithDispatch(LevelDebug, msg, fields...)
}

// InfoWith logs a structured message with additional fields at INFO level.
func (e *LoggerEntry) InfoWith(msg string, fields ...Field) {
	e.entryLogWithDispatch(LevelInfo, msg, fields...)
}

// WarnWith logs a structured message with additional fields at WARN level.
func (e *LoggerEntry) WarnWith(msg string, fields ...Field) {
	e.entryLogWithDispatch(LevelWarn, msg, fields...)
}

// ErrorWith logs a structured message with additional fields at ERROR level.
func (e *LoggerEntry) ErrorWith(msg string, fields ...Field) {
	e.entryLogWithDispatch(LevelError, msg, fields...)
}

// FatalWith logs a structured message at FATAL level and terminates the program via os.Exit(1).
// WARNING: defer statements will NOT execute.
func (e *LoggerEntry) FatalWith(msg string, fields ...Field) {
	e.entryLogWithDispatch(LevelFatal, msg, fields...)
}

// Print methods - output via logger's writers with caller info and entry's fields.
// These methods use LevelInfo for filtering and apply sensitive data filtering.

// Print writes to configured writers with caller info and the entry's fields.
// Uses LevelInfo for filtering. Arguments are joined with spaces.
func (e *LoggerEntry) Print(args ...any) { e.entryLogDispatch(LevelInfo, args...) }

// Println writes to configured writers with caller info and the entry's fields.
// Uses LevelInfo for filtering. Note: Behaves identically to Print() because Log() already adds a newline.
func (e *LoggerEntry) Println(args ...any) { e.entryLogDispatch(LevelInfo, args...) }

// Printf formats according to a format specifier and writes to configured writers
// with caller info and the entry's fields. Uses LevelInfo for filtering.
func (e *LoggerEntry) Printf(format string, args ...any) {
	e.entryLogfDispatch(LevelInfo, format, args...)
}

// Logger methods for WithFields

// WithFields returns a LoggerEntry with pre-set fields.
// The fields are inherited by all logging calls on the returned entry.
//
// Example:
//
//	entry := logger.WithFields(dd.String("service", "api"), dd.String("version", "1.0"))
//	entry.Info("request received") // Contains service and version fields
//	entry.WithFields(dd.String("user", "john")).Info("user action") // Contains all three fields
func (l *Logger) WithFields(fields ...Field) *LoggerEntry {
	return newLoggerEntry(l, fields)
}

// WithField returns a LoggerEntry with a single pre-set field.
// This is a convenience method equivalent to WithFields with a single field.
//
// Example:
//
//	entry := logger.WithField("request_id", "abc123")
func (l *Logger) WithField(key string, value any) *LoggerEntry {
	return newLoggerEntry(l, []Field{{Key: key, Value: value}})
}
