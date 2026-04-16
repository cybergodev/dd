package dd

import (
	"errors"
	"fmt"
	"io"
)

// Error codes for structured error handling.
// These codes are used internally by LoggerError to map to public sentinel errors
// so that errors.Is(err, dd.ErrInvalidLevel) works correctly.
// Do not use these codes directly in application code; use the Err* sentinel errors.
const (
	errCodeNilConfig          = "NIL_CONFIG"
	errCodeNilWriter          = "NIL_WRITER"
	errCodeNilFilter          = "NIL_FILTER"
	errCodeNilHook            = "NIL_HOOK"
	errCodeNilExtractor       = "NIL_EXTRACTOR"
	errCodeLoggerClosed       = "LOGGER_CLOSED"
	errCodeWriterNotFound     = "WRITER_NOT_FOUND"
	errCodeInvalidLevel       = "INVALID_LEVEL"
	errCodeInvalidFormat      = "INVALID_FORMAT"
	errCodeMaxWritersExceeded = "MAX_WRITERS_EXCEEDED"
	errCodeEmptyFilePath      = "EMPTY_FILE_PATH"
	errCodePathTooLong        = "PATH_TOO_LONG"
	errCodePathTraversal      = "PATH_TRAVERSAL"
	errCodeNullByte           = "NULL_BYTE"
	errCodeInvalidPath        = "INVALID_PATH"
	errCodeSymlinkNotAllowed  = "SYMLINK_NOT_ALLOWED"
	errCodeHardlinkNotAllowed = "HARDLINK_NOT_ALLOWED"
	errCodeOverlongEncoding   = "OVERLONG_ENCODING"
	errCodeMaxSizeExceeded    = "MAX_SIZE_EXCEEDED"
	errCodeMaxBackupsExceeded = "MAX_BACKUPS_EXCEEDED"
	errCodeBufferSizeTooLarge = "BUFFER_SIZE_TOO_LARGE"
	errCodeInvalidPattern     = "INVALID_PATTERN"
	errCodeEmptyPattern       = "EMPTY_PATTERN"
	errCodePatternTooLong     = "PATTERN_TOO_LONG"
	errCodeReDoSPattern       = "REDOS_PATTERN"
	errCodePatternFailed      = "PATTERN_FAILED"
	errCodeConfigValidation   = "CONFIG_VALIDATION"
	errCodeWriterAdd          = "WRITER_ADD"
	errCodeMultipleConfigs    = "MULTIPLE_CONFIGS"
	errCodeNilMultiWriter     = "NIL_MULTIWRITER"
)

// LoggerError represents a structured error with additional context.
// It implements error, Unwrap(), and Is() interfaces for fine-grained error matching.
//
// Example usage:
//
//	logger, err := dd.New(config)
//	if err != nil {
//	    var loggerErr *dd.LoggerError
//	    if errors.As(err, &loggerErr) {
//	        fmt.Printf("Error code: %s\n", loggerErr.Code)
//	        fmt.Printf("Context: %v\n", loggerErr.Context)
//	    }
//	    if errors.Is(err, dd.ErrInvalidLevel) {
//	        // Handle invalid level specifically
//	    }
//	}
type LoggerError struct {
	Code    string         // Machine-readable error code (e.g., "INVALID_LEVEL")
	Message string         // Human-readable message
	Cause   error          // Underlying error (for wrapping)
	Context map[string]any // Additional context for debugging
}

// Error implements the error interface.
func (e *LoggerError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause for use with errors.Is() and errors.As().
func (e *LoggerError) Unwrap() error {
	return e.Cause
}

// errorCodeToSentinel maps error codes to their corresponding sentinel errors.
var errorCodeToSentinel = map[string]error{
	errCodeNilConfig:          ErrNilConfig,
	errCodeNilWriter:          ErrNilWriter,
	errCodeNilFilter:          ErrNilFilter,
	errCodeNilHook:            ErrNilHook,
	errCodeNilExtractor:       ErrNilExtractor,
	errCodeLoggerClosed:       ErrLoggerClosed,
	errCodeWriterNotFound:     ErrWriterNotFound,
	errCodeInvalidLevel:       ErrInvalidLevel,
	errCodeInvalidFormat:      ErrInvalidFormat,
	errCodeMaxWritersExceeded: ErrMaxWritersExceeded,
	errCodeEmptyFilePath:      ErrEmptyFilePath,
	errCodePathTooLong:        ErrPathTooLong,
	errCodePathTraversal:      ErrPathTraversal,
	errCodeNullByte:           ErrNullByte,
	errCodeInvalidPath:        ErrInvalidPath,
	errCodeSymlinkNotAllowed:  ErrSymlinkNotAllowed,
	errCodeHardlinkNotAllowed: ErrHardlinkNotAllowed,
	errCodeOverlongEncoding:   ErrOverlongEncoding,
	errCodeMaxSizeExceeded:    ErrMaxSizeExceeded,
	errCodeMaxBackupsExceeded: ErrMaxBackupsExceeded,
	errCodeBufferSizeTooLarge: ErrBufferSizeTooLarge,
	errCodeInvalidPattern:     ErrInvalidPattern,
	errCodeEmptyPattern:       ErrEmptyPattern,
	errCodePatternTooLong:     ErrPatternTooLong,
	errCodeReDoSPattern:       ErrReDoSPattern,
	errCodePatternFailed:      ErrPatternFailed,
	errCodeConfigValidation:   ErrConfigValidation,
	errCodeWriterAdd:          ErrWriterAdd,
	errCodeMultipleConfigs:    ErrMultipleConfigs,
	errCodeNilMultiWriter:     ErrNilMultiWriter,
}

// Is enables matching against sentinel errors using errors.Is().
// This allows LoggerError instances to match their corresponding sentinel errors.
func (e *LoggerError) Is(target error) bool {
	if sentinel, ok := errorCodeToSentinel[e.Code]; ok {
		return target == sentinel
	}
	return false
}

// newError creates a new LoggerError with the given code and message.
func newError(code, message string) *LoggerError {
	return &LoggerError{
		Code:    code,
		Message: message,
	}
}

// wrapError wraps an existing error with a code and message.
// If the error is nil, returns nil.
func wrapError(code, message string, cause error) *LoggerError {
	if cause == nil {
		return nil
	}
	return &LoggerError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// WithContext adds context to a LoggerError.
// Returns a new LoggerError with the context added.
func (e *LoggerError) WithContext(key string, value any) *LoggerError {
	if e == nil {
		return nil
	}
	newContext := make(map[string]any, len(e.Context)+1)
	for k, v := range e.Context {
		newContext[k] = v
	}
	newContext[key] = value
	return &LoggerError{
		Code:    e.Code,
		Message: e.Message,
		Cause:   e.Cause,
		Context: newContext,
	}
}

// WithField adds a field to the LoggerError context.
// This is an alias for WithContext for naming consistency with Logger.
func (e *LoggerError) WithField(key string, value any) *LoggerError {
	return e.WithContext(key, value)
}

// Sentinel errors for backward compatibility.
// These can be used with errors.Is() for simple error matching.
var (
	ErrNilConfig          = errors.New("config cannot be nil")
	ErrNilWriter          = errors.New("writer cannot be nil")
	ErrNilFilter          = errors.New("filter cannot be nil")
	ErrNilHook            = errors.New("hook cannot be nil")
	ErrNilExtractor       = errors.New("context extractor cannot be nil")
	ErrLoggerClosed       = errors.New("logger is closed")
	ErrWriterNotFound     = errors.New("writer not found")
	ErrInvalidLevel       = errors.New("invalid log level")
	ErrInvalidFormat      = errors.New("invalid log format")
	ErrMaxWritersExceeded = errors.New("maximum writer count exceeded")
	ErrEmptyFilePath      = errors.New("file path cannot be empty")
	ErrPathTooLong        = errors.New("file path too long")
	ErrPathTraversal      = errors.New("path traversal detected")
	ErrNullByte           = errors.New("null byte in input")
	ErrInvalidPath        = errors.New("invalid file path")
	ErrSymlinkNotAllowed  = errors.New("symlinks not allowed")
	ErrHardlinkNotAllowed = errors.New("hardlinks not allowed")
	ErrOverlongEncoding   = errors.New("UTF-8 overlong encoding detected")
	ErrMaxSizeExceeded    = errors.New("maximum size exceeded")
	ErrMaxBackupsExceeded = errors.New("maximum backup count exceeded")
	ErrBufferSizeTooLarge = errors.New("buffer size too large")
	ErrInvalidPattern     = errors.New("invalid regex pattern")
	ErrEmptyPattern       = errors.New("pattern cannot be empty")
	ErrPatternTooLong     = errors.New("pattern length exceeds maximum")
	ErrReDoSPattern       = errors.New("pattern contains dangerous nested quantifiers that may cause ReDoS")
	ErrPatternFailed      = errors.New("failed to add pattern")
	ErrConfigValidation   = errors.New("configuration validation failed")
	ErrWriterAdd          = errors.New("failed to add writer")
	ErrMultipleConfigs    = errors.New("multiple configs provided, expected 0 or 1")
	ErrNilMultiWriter     = errors.New("multiwriter is nil")
)

// WriterError represents an error from a single writer in a MultiWriter.
type WriterError struct {
	Index  int       // Index of the writer in the MultiWriter
	Writer io.Writer // The writer that encountered the error
	Err    error     // The error that occurred
}

// Error implements the error interface.
func (e *WriterError) Error() string {
	if e == nil {
		return "<nil WriterError>"
	}
	if e.Err != nil {
		return fmt.Sprintf("writer[%d]: %v", e.Index, e.Err)
	}
	return fmt.Sprintf("writer[%d]: unknown error", e.Index)
}

// Unwrap returns the underlying error.
func (e *WriterError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// MultiWriterError collects errors from multiple writers.
// This is returned by MultiWriter.Write() when one or more writers fail.
type MultiWriterError struct {
	Errors []WriterError // Collection of writer errors
}

// Error implements the error interface.
func (e *MultiWriterError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return ""
	}

	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}

	msgs := make([]string, 0, len(e.Errors))
	for _, err := range e.Errors {
		msgs = append(msgs, err.Error())
	}
	return fmt.Sprintf("multiple writer errors: %v", msgs)
}

// Unwrap returns all underlying errors for use with errors.As().
// Note: errors.Is() will check against each wrapped error.
func (e *MultiWriterError) Unwrap() []error {
	if e == nil || len(e.Errors) == 0 {
		return nil
	}

	errs := make([]error, len(e.Errors))
	for i, we := range e.Errors {
		errs[i] = we.Err
	}
	return errs
}

// HasErrors returns true if any errors were collected.
func (e *MultiWriterError) HasErrors() bool {
	return e != nil && len(e.Errors) > 0
}

// ErrorCount returns the number of errors collected.
func (e *MultiWriterError) ErrorCount() int {
	if e == nil {
		return 0
	}
	return len(e.Errors)
}

// FirstError returns the first error that occurred.
func (e *MultiWriterError) FirstError() error {
	if e == nil || len(e.Errors) == 0 {
		return nil
	}
	return &e.Errors[0]
}

// addError adds a writer error to the collection.
func (e *MultiWriterError) addError(index int, writer io.Writer, err error) {
	e.Errors = append(e.Errors, WriterError{
		Index:  index,
		Writer: writer,
		Err:    err,
	})
}
