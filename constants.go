package dd

import "time"

// Core constants for the dd logging library
const (
	// Caller depth constants
	DefaultCallerDepth      = 3 // Default depth for caller detection
	ConvenienceDepth        = 4 // Depth for convenience functions
	StructuredDepth         = 4 // Depth for structured logging
	DebugVisualizationDepth = 2 // Depth for debug visualization

	// Buffer and pool constants
	DefaultBufferSize    = 1024     // Default message buffer size
	MaxBufferSize        = 4 * 1024 // Maximum buffer size for pooling
	FieldBuilderCapacity = 256      // Initial capacity for field string builder
	EstimatedFieldSize   = 24       // Estimated size per field for pre-allocation

	// Security and validation constants
	MaxPathLength     = 4096            // Maximum file path length
	MaxMessageSize    = 5 * 1024 * 1024 // Maximum message size (5MB)
	MaxInputLength    = 256 * 1024      // Maximum input length for filters (256KB)
	MaxWriterCount    = 100             // Maximum number of writers
	MaxBackupCount    = 1000            // Maximum backup file count
	MaxFileSizeMB     = 10240           // Maximum file size (10GB)
	MaxFieldKeyLength = 256             // Maximum field key length

	// File writer constants
	DefaultMaxSizeMB    = 100                    // Default file rotation size
	DefaultMaxBackups   = 10                     // Default backup count
	DefaultMaxAge       = 30 * 24 * time.Hour    // Default file retention
	DefaultBufferSizeKB = 1                      // Default buffer size in KB
	MaxBufferSizeKB     = 10 * 1024              // Maximum buffer size (10MB)
	AutoFlushThreshold  = 2                      // Buffer size divisor for auto-flush
	AutoFlushInterval   = 100 * time.Millisecond // Auto-flush interval
	DirPermissions      = 0700                   // Directory permissions
	FilePermissions     = 0600                   // File permissions

	// Filter and timeout constants
	DefaultFilterTimeout = 50 * time.Millisecond // Default regex timeout
	EmptyFilterTimeout   = 10 * time.Millisecond // Timeout for empty filters
	ChunkSize            = 1024                  // Processing chunk size
	RetryAttempts        = 3                     // File operation retry attempts
	RetryDelay           = 10 * time.Millisecond // Retry delay
	VerifyBufferSize     = 1024                  // Buffer size for verification

	// Default file paths
	DefaultLogFile = "logs/app.log" // Default log file path
)

// String constants for common field names and formats
const (
	DefaultTimeFormat = "2006-01-02T15:04:05Z07:00" // RFC3339 format for backward compatibility
	DevTimeFormat     = "15:04:05.000"              // Development time format

	// JSON field names
	DefaultTimestampField = "timestamp"
	DefaultLevelField     = "level"
	DefaultCallerField    = "caller"
	DefaultMessageField   = "message"
	DefaultFieldsField    = "fields"
	DefaultErrorField     = "error"

	// JSON formatting
	DefaultJSONIndent = "  "
)
