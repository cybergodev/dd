package dd

import "errors"

// Core errors
var (
	// ErrNilConfig is returned when a nil configuration is provided
	ErrNilConfig = errors.New("config cannot be nil")

	// ErrNilWriter is returned when attempting to add a nil writer
	ErrNilWriter = errors.New("writer cannot be nil")

	// ErrLoggerClosed is returned when operations are attempted on a closed logger
	ErrLoggerClosed = errors.New("logger is closed")

	// ErrInvalidLevel is returned when an invalid log level is provided
	ErrInvalidLevel = errors.New("invalid log level")

	// ErrInvalidFormat is returned when an invalid log format is provided
	ErrInvalidFormat = errors.New("invalid log format")

	// ErrMaxWritersExceeded is returned when the maximum writer count is exceeded
	ErrMaxWritersExceeded = errors.New("maximum writer count exceeded")

	// ErrEmptyFilePath is returned when an empty file path is provided
	ErrEmptyFilePath = errors.New("file path cannot be empty")

	// ErrPathTooLong is returned when a file path exceeds maximum length
	ErrPathTooLong = errors.New("file path too long")

	// ErrPathTraversal is returned when path traversal is detected
	ErrPathTraversal = errors.New("path traversal detected")

	// ErrNullByte is returned when a null byte is found in input
	ErrNullByte = errors.New("null byte in input")

	// ErrInvalidPath is returned when a file path is invalid
	ErrInvalidPath = errors.New("invalid file path")

	// ErrSymlinkNotAllowed is returned when a symlink is encountered
	ErrSymlinkNotAllowed = errors.New("symlinks not allowed")

	// ErrMaxSizeExceeded is returned when maximum size is exceeded
	ErrMaxSizeExceeded = errors.New("maximum size exceeded")

	// ErrMaxBackupsExceeded is returned when maximum backup count is exceeded
	ErrMaxBackupsExceeded = errors.New("maximum backup count exceeded")

	// ErrBufferSizeTooLarge is returned when buffer size exceeds maximum
	ErrBufferSizeTooLarge = errors.New("buffer size too large")

	// ErrInvalidFilterLevel is returned when an invalid filter level is provided
	ErrInvalidFilterLevel = errors.New("invalid filter level")

	// ErrInvalidPattern is returned when a regex pattern is invalid
	ErrInvalidPattern = errors.New("invalid regex pattern")
)
