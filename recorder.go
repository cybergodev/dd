package dd

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
)

// LogEntry represents a captured log entry for testing purposes.
type LogEntry struct {
	Level     LogLevel
	Message   string
	Fields    []Field
	Timestamp time.Time
	Format    LogFormat
	RawOutput string // Original formatted output
}

// LoggerRecorder captures log entries for testing.
// It implements io.Writer and can be used as a logger output.
// Thread-safe for concurrent use.
type LoggerRecorder struct {
	mu      sync.Mutex
	entries []LogEntry
	format  LogFormat
	buf     bytes.Buffer
}

// Pre-compiled regex patterns for text log entry parsing.
// Compiled once at package init to avoid recompilation on every parse call.
//
// LIMITATION: These patterns assume the default time format (ISO 8601) and
// default level strings (DEBUG/INFO/WARN/ERROR/FATAL). If you customize
// TimeFormat or JSONFieldNames in the logger config, the text-mode parsing
// may silently fail to extract level/timestamp. For custom formats, use
// JSON format (FormatJSON) which parses reliably via encoding/json.
var (
	// levelRegex extracts log level from format "[TIMESTAMP  LEVEL]"
	levelRegex = regexp.MustCompile(`\[\d{4}-\d{2}-\d{2}T[^\]]*?\s+(\w+)\]`)
	// tsRegex extracts timestamp from "[YYYY-MM-DDTHH:MM:SS..."
	tsRegex = regexp.MustCompile(`\[(\d{4}-\d{2}-\d{2}T[^\s]+)`)
	// afterBracketRegex extracts content after "] file:line "
	afterBracketRegex = regexp.MustCompile(`\] [^:]+:\d+ (.*)$`)
	// fieldStartRegex finds where key=value pairs begin
	fieldStartRegex = regexp.MustCompile(`\s+(\w+=)`)
)

// NewLoggerRecorder creates a new LoggerRecorder.
// The format parameter specifies the expected log format for parsing.
//
// Example:
//
//	recorder := dd.NewLoggerRecorder()
//	cfg := dd.DefaultConfig()
//	cfg.Targets = []dd.OutputTarget{dd.CustomOutput(recorder.Writer())}
//	logger, _ := dd.New(cfg)
//	logger.Info("test message")
//	entries := recorder.Entries()
func NewLoggerRecorder() *LoggerRecorder {
	return &LoggerRecorder{
		entries: make([]LogEntry, 0),
		format:  FormatText,
	}
}

// Writer returns an io.Writer for use with logger configuration.
// The writer captures all log output for later inspection.
func (r *LoggerRecorder) Writer() io.Writer {
	return &recorderWriter{recorder: r}
}

// SetFormat sets the expected log format for parsing.
// This should match the format configured in the logger.
func (r *LoggerRecorder) SetFormat(format LogFormat) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.format = format
}

// recorderWriter wraps LoggerRecorder to implement io.Writer
type recorderWriter struct {
	recorder *LoggerRecorder
}

// Write implements io.Writer. Each write is parsed as a complete log entry.
func (w *recorderWriter) Write(p []byte) (n int, err error) {
	w.recorder.write(p)
	return len(p), nil
}

// write parses and stores a log entry.
// Whitespace-only writes (e.g., standalone newlines from the single-writer fast path)
// are silently ignored to avoid creating empty entries.
func (r *LoggerRecorder) write(p []byte) {
	// Skip whitespace-only writes produced by the single-writer newline optimization
	if len(bytes.TrimSpace(p)) == 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	raw := string(p)
	entry := LogEntry{
		RawOutput: raw,
		Format:    r.format,
	}

	// Parse based on format
	if r.format == FormatJSON {
		r.parseJSONEntry(&entry, raw)
	} else {
		r.parseTextEntry(&entry, raw)
	}

	r.entries = append(r.entries, entry)
}

// parseJSONEntry parses a JSON formatted log entry
func (r *LoggerRecorder) parseJSONEntry(entry *LogEntry, raw string) {
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err == nil {
		if level, ok := data["level"].(string); ok {
			entry.Level = parseLevelString(level)
		}
		if msg, ok := data["message"].(string); ok {
			entry.Message = msg
		}
		if ts, ok := data["timestamp"].(string); ok {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				entry.Timestamp = t
			}
		}
		// Extract fields from JSON
		entry.Fields = extractFieldsFromJSON(data)
	}
}

// parseTextEntry parses a text formatted log entry
// Format: [TIMESTAMP  LEVEL] file:line message key=value ...
// Example: [2024-01-15T10:30:00+08:00  INFO] test.go:14 Test message user=john
func (r *LoggerRecorder) parseTextEntry(entry *LogEntry, raw string) {
	// Remove trailing newline for easier parsing
	raw = trimNewline(raw)

	// Extract level - format is "[TIMESTAMP  LEVEL]"
	if matches := levelRegex.FindStringSubmatch(raw); len(matches) > 1 {
		entry.Level = parseLevelString(matches[1])
	}

	// Extract timestamp (ISO format with timezone)
	if matches := tsRegex.FindStringSubmatch(raw); len(matches) > 1 {
		if t, err := time.Parse(time.RFC3339, matches[1]); err == nil {
			entry.Timestamp = t
		}
	}

	// Extract message and fields
	if matches := afterBracketRegex.FindStringSubmatch(raw); len(matches) > 1 {
		remainder := matches[1]
		// Find where key=value pairs start
		if idx := fieldStartRegex.FindStringIndex(remainder); idx != nil {
			entry.Message = remainder[:idx[0]]
			// Parse fields
			fieldPart := remainder[idx[0]+1:]
			entry.Fields = parseKeyValueFields(fieldPart)
		} else {
			entry.Message = remainder
		}
	}
}

// trimNewline removes trailing newline characters
func trimNewline(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '\r' {
		s = s[:len(s)-1]
	}
	return s
}

// parseKeyValueFields parses "key=value key2=value2" format
func parseKeyValueFields(s string) []Field {
	var fields []Field
	// Simple parsing - split by space, then by =
	parts := splitFields(s)
	for _, part := range parts {
		if eq := strings.IndexByte(part, '='); eq > 0 {
			key := part[:eq]
			value := part[eq+1:]
			fields = append(fields, Field{Key: key, Value: value})
		}
	}
	return fields
}

// splitFields splits by space but handles quoted values
func splitFields(s string) []string {
	var result []string
	inQuote := false
	start := 0

	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			inQuote = !inQuote
		} else if s[i] == ' ' && !inQuote {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

// parseLevelString converts a level string to LogLevel
func parseLevelString(s string) LogLevel {
	switch s {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "FATAL":
		return LevelFatal
	default:
		return LevelInfo
	}
}

// extractFieldsFromJSON extracts fields from a JSON log entry
func extractFieldsFromJSON(data map[string]any) []Field {
	var fields []Field
	reserved := map[string]bool{
		"level": true, "message": true, "timestamp": true,
		"time": true, "caller": true, "file": true, "line": true,
	}

	for k, v := range data {
		if reserved[k] {
			continue
		}
		fields = append(fields, Field{Key: k, Value: v})
	}
	return fields
}

// NewLogger creates a new Logger configured to write to this recorder.
// This is a convenience method for quickly creating a test logger.
// Returns an error if logger creation fails.
//
// Example:
//
//	recorder := dd.NewLoggerRecorder()
//	logger, err := recorder.NewLogger()
//	if err != nil {
//	    t.Fatal(err)
//	}
//	logger.Info("test")
func (r *LoggerRecorder) NewLogger(cfg ...Config) (*Logger, error) {
	var c Config
	if len(cfg) > 0 {
		c = cfg[0]
	} else {
		c = DefaultConfig()
	}
	c.Targets = []OutputTarget{CustomOutput(r.Writer())}
	return c.build()
}

// Entries returns all captured log entries.
// Returns a copy to prevent modification of internal state.
func (r *LoggerRecorder) Entries() []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make([]LogEntry, len(r.entries))
	copy(result, r.entries)
	return result
}

// Count returns the number of captured log entries.
func (r *LoggerRecorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// Clear removes all captured log entries.
func (r *LoggerRecorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = make([]LogEntry, 0)
	r.buf.Reset()
}

// HasEntries returns true if at least one entry has been captured.
func (r *LoggerRecorder) HasEntries() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries) > 0
}

// LastEntry returns the most recent log entry, or nil if no entries exist.
func (r *LoggerRecorder) LastEntry() *LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.entries) == 0 {
		return nil
	}
	entry := r.entries[len(r.entries)-1]
	return &entry
}

// EntriesAtLevel returns all entries at the specified log level.
func (r *LoggerRecorder) EntriesAtLevel(level LogLevel) []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	var result []LogEntry
	for _, entry := range r.entries {
		if entry.Level == level {
			result = append(result, entry)
		}
	}
	return result
}

// ContainsMessage returns true if any entry contains the specified message.
func (r *LoggerRecorder) ContainsMessage(msg string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range r.entries {
		if entry.Message == msg || strings.Contains(entry.Message, msg) {
			return true
		}
	}
	return false
}

// ContainsField returns true if any entry contains a field with the specified key.
func (r *LoggerRecorder) ContainsField(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range r.entries {
		for _, field := range entry.Fields {
			if field.Key == key {
				return true
			}
		}
	}
	return false
}

// GetFieldValue returns the value of the first field with the specified key,
// or nil if no such field exists.
func (r *LoggerRecorder) GetFieldValue(key string) any {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range r.entries {
		for _, field := range entry.Fields {
			if field.Key == key {
				return field.Value
			}
		}
	}
	return nil
}
