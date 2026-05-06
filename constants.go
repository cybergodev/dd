package dd

import (
	"time"

	"github.com/cybergodev/dd/internal"
)

// LogFormat defines the output format for log messages.
type LogFormat = internal.LogFormat

const (
	// FormatText outputs log messages in human-readable text format.
	FormatText LogFormat = internal.LogFormatText
	// FormatJSON outputs log messages as structured JSON objects.
	FormatJSON LogFormat = internal.LogFormatJSON
)

const (
	// defaultCallerDepth is the number of stack frames to skip when
	// determining the caller of a log function.
	// Value 3 accounts for: runtime.Caller -> GetCaller -> Log method -> user code
	defaultCallerDepth = 3

	// entryCallerDepth is the additional stack frames to skip when logging
	// from a LoggerEntry. LoggerEntry adds 2 extra layers: Entry.Info -> Entry.Log
	entryCallerDepth = 2
)

const (
	// defaultBufferSize is the initial capacity for message buffers.
	// 1024 bytes covers most typical log messages without reallocation.
	defaultBufferSize = 1024

	// maxBufferSize is the maximum buffer capacity returned to the pool.
	// Buffers larger than 4KB are replaced with default-sized buffers to
	// prevent memory bloat from occasional large messages. This value
	// balances memory efficiency with performance for typical workloads.
	maxBufferSize = 4 * 1024
)

const (
	// maxPathLength limits file paths to 4096 bytes (POSIX PATH_MAX).
	// Prevents path traversal attacks and memory exhaustion from malicious paths.
	maxPathLength = 4096

	// maxMessageSize limits formatted log messages to 5MB.
	// This prevents memory exhaustion from extremely large log entries while
	// allowing substantial content (e.g., stack traces with context).
	maxMessageSize = 5 * 1024 * 1024

	// maxInputLength limits input for sensitive data filtering to 256KB.
	// Beyond this size, filtering becomes CPU-intensive. The boundary-aware
	// truncation ensures sensitive data is still detected at the edges.
	maxInputLength = 256 * 1024

	// maxWriterCount limits concurrent writers to 100.
	// This prevents resource exhaustion from misconfigured loggers while
	// allowing reasonable multi-output scenarios (file + stdout + network).
	maxWriterCount = 100

	// maxPatternLength limits regex patterns to 1000 characters.
	// Longer patterns are rarely needed and may indicate ReDoS attempts.
	maxPatternLength = 1000

	// maxQuantifierRange limits regex quantifier ranges (e.g., {1,1000}).
	// Prevents ReDoS attacks with patterns like (a{1,1000000})+.
	maxQuantifierRange = 1000

	// maxRecursionDepth limits recursive filtering of nested structures.
	// Prevents stack overflow from deeply nested or circular data.
	maxRecursionDepth = 100

	// maxFilterElements limits total elements processed during recursive filtering.
	// Prevents resource exhaustion from structures with many fields/slice elements.
	maxFilterElements = 10000
)

const (
	// maxBackupCount limits the number of backup files to retain.
	// This prevents disk exhaustion from excessive log file accumulation.
	maxBackupCount = 1000

	// maxFileSizeMB limits the maximum size of a single log file to 10GB.
	// Files larger than this will trigger rotation.
	maxFileSizeMB = 10240
)

const (
	// DefaultMaxSizeMB is the default maximum log file size in megabytes before rotation.
	DefaultMaxSizeMB = 100
	// DefaultMaxBackups is the default maximum number of rotated backup files to retain.
	DefaultMaxBackups = 10
	// DefaultMaxAge is the default maximum age for backup files before cleanup.
	DefaultMaxAge = 30 * 24 * time.Hour
	defaultBufferSizeKB = 1
	maxBufferSizeKB     = 10 * 1024
	autoFlushThreshold  = 2
	autoFlushInterval   = 100 * time.Millisecond
)

// File system permission constants.
const (
	// dirPermissions is the permission mode for creating directories (rwx------).
	dirPermissions = 0700
)

// FilePermissions (0600) for log files is defined in internal/json.go.

const (
	// defaultFilterTimeout is the maximum time for sensitive data filtering.
	// This timeout protects against ReDoS (Regular Expression Denial of Service)
	// attacks on malicious input with complex regex patterns.
	// 50ms provides reasonable protection while allowing thorough filtering.
	defaultFilterTimeout = 50 * time.Millisecond

	// emptyFilterTimeout is used for filters with no patterns.
	// Shorter timeout since minimal processing is needed.
	emptyFilterTimeout = 10 * time.Millisecond

	// maxConcurrentFilters limits concurrent regex filtering goroutines.
	// Prevents resource exhaustion in high-concurrency scenarios.
	maxConcurrentFilters = 100

	// filterMediumInputThreshold is the input size threshold for synchronous chunked processing.
	// Inputs between fastPathThreshold and this value use synchronous chunked processing.
	// Inputs larger than this use async processing with timeout protection.
	filterMediumInputThreshold = 100 * fastPathThreshold // 10KB

	// filterDirectProcessThreshold is the maximum input size for direct processing
	// without chunking during timeout-protected filtering.
	filterDirectProcessThreshold = 32 * 1024 // 32KB

	// filterChunkSize is the size of chunks for processing large inputs.
	filterChunkSize = 4096 // 4KB
)

const (
	// DefaultTimeFormat is the default timestamp format (ISO 8601 with timezone).
	DefaultTimeFormat = "2006-01-02T15:04:05Z07:00"
	devTimeFormat     = "15:04:05.000"
)

const (
	// defaultFatalFlushTimeout is the maximum time to wait for logger flush
	// during fatal log handling. This ensures the program can exit even if
	// the underlying writer is blocked or unresponsive.
	defaultFatalFlushTimeout = 5 * time.Second

	// defaultLoggerCloseDelay is the delay before closing an old logger
	// when SetDefault() is called with a new logger. This allows in-flight
	// log operations to complete before the old logger is closed.
	defaultLoggerCloseDelay = 100 * time.Millisecond
)

const (
	defaultJSONIndent = "  "
	// fastPathThreshold is the input length threshold (bytes) below which
	// regex filtering skips chunking and timeout protection.
	// Small inputs are processed directly for better performance.
	fastPathThreshold = 100

	// boundaryCheckSize is the size of the boundary region to check for sensitive data
	// when truncating input. This ensures patterns spanning the truncation boundary
	// are still detected and redacted.
	// Set to 512 to cover most sensitive data patterns (credit cards, SSNs, API keys, etc.)
	boundaryCheckSize = 512

	// chunkOverlapSize is the overlap size between chunks during chunked filtering.
	// This ensures sensitive data patterns that span chunk boundaries are still detected.
	// Must be >= maximum expected sensitive pattern length.
	chunkOverlapSize = 512
)

const (
	// debugVisualizationDepth is the caller depth for debug visualization functions.
	// Value of 2 means: 0 = current function, 1 = caller, 2 = caller's caller.
	debugVisualizationDepth = 2
)

// DefaultLogPath is the default path for log files.
const DefaultLogPath = "logs/app.log"
