package dd

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// AuditEventType represents the type of security audit event.
type AuditEventType int

const (
	// AuditEventSensitiveDataRedacted is logged when sensitive data is redacted.
	AuditEventSensitiveDataRedacted AuditEventType = iota
	// AuditEventRateLimitExceeded is logged when rate limiting is triggered.
	AuditEventRateLimitExceeded
	// AuditEventReDoSAttempt is logged when a potential ReDoS pattern is detected.
	AuditEventReDoSAttempt
	// AuditEventSecurityViolation is logged for general security violations.
	AuditEventSecurityViolation
	// AuditEventIntegrityViolation is logged when log integrity verification fails.
	AuditEventIntegrityViolation
	// AuditEventInputSanitized is logged when input is sanitized.
	AuditEventInputSanitized
	// AuditEventPathTraversalAttempt is logged when path traversal is detected.
	AuditEventPathTraversalAttempt
	// AuditEventLog4ShellAttempt is logged when Log4Shell pattern is detected.
	AuditEventLog4ShellAttempt
	// AuditEventNullByteInjection is logged when null byte injection is detected.
	AuditEventNullByteInjection
	// AuditEventOverlongEncoding is logged when UTF-8 overlong encoding is detected.
	AuditEventOverlongEncoding
	// AuditEventHomographAttack is logged when homograph attack is detected.
	AuditEventHomographAttack
)

// String returns the string representation of the audit event type.
func (e AuditEventType) String() string {
	switch e {
	case AuditEventSensitiveDataRedacted:
		return "SENSITIVE_DATA_REDACTED"
	case AuditEventRateLimitExceeded:
		return "RATE_LIMIT_EXCEEDED"
	case AuditEventReDoSAttempt:
		return "REDOS_ATTEMPT"
	case AuditEventSecurityViolation:
		return "SECURITY_VIOLATION"
	case AuditEventIntegrityViolation:
		return "INTEGRITY_VIOLATION"
	case AuditEventInputSanitized:
		return "INPUT_SANITIZED"
	case AuditEventPathTraversalAttempt:
		return "PATH_TRAVERSAL_ATTEMPT"
	case AuditEventLog4ShellAttempt:
		return "LOG4SHELL_ATTEMPT"
	case AuditEventNullByteInjection:
		return "NULL_BYTE_INJECTION"
	case AuditEventOverlongEncoding:
		return "OVERLONG_ENCODING"
	case AuditEventHomographAttack:
		return "HOMOGRAPH_ATTACK"
	default:
		return "UNKNOWN"
	}
}

// AuditEvent represents a security audit event.
type AuditEvent struct {
	// Type is the type of audit event.
	Type AuditEventType `json:"type"`
	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`
	// Message is a human-readable description of the event.
	Message string `json:"message"`
	// Pattern is the regex pattern that triggered the event (if applicable).
	Pattern string `json:"pattern,omitempty"`
	// Field is the field name that triggered the event (if applicable).
	Field string `json:"field,omitempty"`
	// Metadata contains additional context about the event.
	Metadata map[string]any `json:"metadata,omitempty"`
	// Severity indicates the severity level of the event.
	Severity AuditSeverity `json:"severity"`
}

// AuditSeverity represents the severity level of an audit event.
type AuditSeverity int

const (
	// AuditSeverityInfo is for informational events.
	AuditSeverityInfo AuditSeverity = iota
	// AuditSeverityWarning is for warning events.
	AuditSeverityWarning
	// AuditSeverityError is for error events.
	AuditSeverityError
	// AuditSeverityCritical is for critical security events.
	AuditSeverityCritical
)

// String returns the string representation of the audit severity.
func (s AuditSeverity) String() string {
	switch s {
	case AuditSeverityInfo:
		return "INFO"
	case AuditSeverityWarning:
		return "WARNING"
	case AuditSeverityError:
		return "ERROR"
	case AuditSeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON implements json.Marshaler for AuditSeverity.
func (s AuditSeverity) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// AuditConfig configures the audit logger.
type AuditConfig struct {
	// Enabled determines if audit logging is enabled.
	Enabled bool
	// Output is the destination for audit logs.
	// If nil, audit events are only available via the Events channel.
	Output *os.File
	// BufferSize is the size of the async event buffer.
	// Default: 1000 events
	BufferSize int
	// IncludeTimestamp determines if timestamps are included.
	IncludeTimestamp bool
	// JSONFormat determines if output should be JSON formatted.
	JSONFormat bool
	// MinimumSeverity is the minimum severity level to log.
	MinimumSeverity AuditSeverity
	// IntegritySigner provides optional integrity protection for audit logs.
	// When configured, each audit event is signed to detect tampering.
	IntegritySigner *IntegritySigner
}

// Validate validates the AuditConfig and returns an error if any field is invalid.
func (c AuditConfig) Validate() error {
	if c.BufferSize < 0 {
		return fmt.Errorf("audit buffer size must be non-negative, got %d", c.BufferSize)
	}
	return nil
}

// DefaultAuditConfig returns an AuditConfig with sensible defaults.
// Note: Audit logging is enabled by default for security monitoring.
func DefaultAuditConfig() AuditConfig {
	return AuditConfig{
		Enabled:          true,
		Output:           os.Stderr,
		BufferSize:       1000,
		IncludeTimestamp: true,
		JSONFormat:       true,
		MinimumSeverity:  AuditSeverityInfo,
	}
}

// AuditLogger logs security audit events asynchronously.
// It uses a buffered channel for event processing to avoid blocking
// the hot path in the logger.
type AuditLogger struct {
	config  *AuditConfig
	events  chan AuditEvent
	done    chan struct{}
	wg      sync.WaitGroup
	closed  atomic.Bool
	dropped atomic.Int64 // Count of dropped events due to full buffer

	// Statistics
	totalEvents atomic.Int64
	byType      sync.Map // map[AuditEventType]*atomic.Int64
}

// NewAuditLogger creates a new AuditLogger with the given configuration.
// If no configuration is provided, DefaultAuditConfig() is used.
// Returns an error if the configuration is invalid.
func NewAuditLogger(cfg ...AuditConfig) (*AuditLogger, error) {
	var config AuditConfig
	if len(cfg) > 0 {
		config = cfg[0]
	} else {
		config = DefaultAuditConfig()
	}
	return newAuditLoggerWithConfig(config)
}

func newAuditLoggerWithConfig(config AuditConfig) (*AuditLogger, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	al := &AuditLogger{
		config: &config,
		events: make(chan AuditEvent, config.BufferSize),
		done:   make(chan struct{}),
	}

	if config.Enabled {
		al.wg.Add(1)
		go al.processEvents()
	}

	return al, nil
}

// Log records an audit event asynchronously.
// If the buffer is full, the event is dropped and the dropped counter is incremented.
func (al *AuditLogger) Log(event AuditEvent) {
	if al == nil || !al.config.Enabled || al.closed.Load() {
		return
	}

	// Check severity threshold
	if event.Severity < al.config.MinimumSeverity {
		return
	}

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Try to send without blocking
	select {
	case al.events <- event:
		al.totalEvents.Add(1)
		al.incrementTypeCount(event.Type)
	default:
		// Buffer full, drop event
		al.dropped.Add(1)
	}
}

// LogSensitiveDataRedaction logs a sensitive data redaction event.
func (al *AuditLogger) LogSensitiveDataRedaction(pattern, field, message string) {
	al.Log(AuditEvent{
		Type:     AuditEventSensitiveDataRedacted,
		Message:  message,
		Pattern:  pattern,
		Field:    field,
		Severity: AuditSeverityInfo,
	})
}

// LogRateLimitExceeded logs a rate limit exceeded event.
func (al *AuditLogger) LogRateLimitExceeded(message string, metadata map[string]any) {
	al.Log(AuditEvent{
		Type:     AuditEventRateLimitExceeded,
		Message:  message,
		Metadata: metadata,
		Severity: AuditSeverityWarning,
	})
}

// LogSecurityViolation logs a security violation event.
func (al *AuditLogger) LogSecurityViolation(violationType string, message string, metadata map[string]any) {
	al.Log(AuditEvent{
		Type:     AuditEventSecurityViolation,
		Message:  fmt.Sprintf("%s: %s", violationType, message),
		Metadata: metadata,
		Severity: AuditSeverityError,
	})
}

// LogReDoSAttempt logs a ReDoS attempt event.
func (al *AuditLogger) LogReDoSAttempt(pattern, message string) {
	al.Log(AuditEvent{
		Type:     AuditEventReDoSAttempt,
		Message:  message,
		Pattern:  pattern,
		Severity: AuditSeverityCritical,
	})
}

// LogIntegrityViolation logs an integrity violation event.
func (al *AuditLogger) LogIntegrityViolation(message string, metadata map[string]any) {
	al.Log(AuditEvent{
		Type:     AuditEventIntegrityViolation,
		Message:  message,
		Metadata: metadata,
		Severity: AuditSeverityCritical,
	})
}

// LogPathTraversalAttempt logs a path traversal attempt event.
func (al *AuditLogger) LogPathTraversalAttempt(path, message string) {
	al.Log(AuditEvent{
		Type:     AuditEventPathTraversalAttempt,
		Message:  message,
		Metadata: map[string]any{"path": path},
		Severity: AuditSeverityCritical,
	})
}

// processEvents processes audit events asynchronously.
func (al *AuditLogger) processEvents() {
	defer al.wg.Done()

	for {
		select {
		case <-al.done:
			// Drain remaining events
			for {
				select {
				case event := <-al.events:
					al.writeEvent(event)
				default:
					return
				}
			}
		case event := <-al.events:
			al.writeEvent(event)
		}
	}
}

// writeEvent writes an event to the configured output.
// If an IntegritySigner is configured, the event is signed for tamper detection.
func (al *AuditLogger) writeEvent(event AuditEvent) {
	if al.config.Output == nil {
		return
	}

	var output string
	if al.config.JSONFormat {
		data, err := json.Marshal(event)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dd: failed to marshal audit event: %v\n", err)
			return
		}
		output = string(data)
	} else {
		if al.config.IncludeTimestamp {
			output = fmt.Sprintf("[%s] %s: %s",
				event.Timestamp.Format(time.RFC3339),
				event.Type,
				event.Message)
		} else {
			output = fmt.Sprintf("[%s] %s", event.Type, event.Message)
		}
		if event.Pattern != "" {
			output += fmt.Sprintf(" pattern=%s", event.Pattern)
		}
		if event.Field != "" {
			output += fmt.Sprintf(" field=%s", event.Field)
		}
	}

	// Sign the event if integrity protection is configured
	if al.config.IntegritySigner != nil {
		signature := al.config.IntegritySigner.Sign(output)
		output = output + " " + signature
	}

	fmt.Fprintln(al.config.Output, output)
}

// incrementTypeCount increments the count for an event type.
// Uses LoadOrStore to atomically handle the check-then-act pattern,
// preventing race conditions where multiple goroutines could create
// duplicate counters for the same event type.
func (al *AuditLogger) incrementTypeCount(eventType AuditEventType) {
	// Try to load existing counter first (fast path)
	if ptr, ok := al.byType.Load(eventType); ok {
		if counter, ok := ptr.(*atomic.Int64); ok {
			counter.Add(1)
			return
		}
	}

	// Slow path: use LoadOrStore to atomically get or create the counter
	counter := &atomic.Int64{}
	counter.Store(1)
	if actual, loaded := al.byType.LoadOrStore(eventType, counter); loaded {
		// Another goroutine created the counter first, use it
		if existingCounter, ok := actual.(*atomic.Int64); ok {
			// Add 1 to account for the initial count we tried to set
			existingCounter.Add(1)
		}
	}
}

// AuditStats holds audit logger statistics.
type AuditStats struct {
	TotalEvents int64                    // Total events logged
	Dropped     int64                    // Events dropped due to full buffer
	ByType      map[AuditEventType]int64 // Events by type
	BufferSize  int                      // Configured buffer size
	BufferUsage int                      // Current buffer usage
}

// Stats returns current audit logger statistics.
func (al *AuditLogger) Stats() AuditStats {
	if al == nil {
		return AuditStats{}
	}

	stats := AuditStats{
		TotalEvents: al.totalEvents.Load(),
		Dropped:     al.dropped.Load(),
		BufferSize:  al.config.BufferSize,
		BufferUsage: len(al.events),
		ByType:      make(map[AuditEventType]int64),
	}

	al.byType.Range(func(key, value any) bool {
		if eventType, ok := key.(AuditEventType); ok {
			if counter, ok := value.(*atomic.Int64); ok {
				stats.ByType[eventType] = counter.Load()
			}
		}
		return true
	})

	return stats
}

// Close stops the audit logger and flushes remaining events.
func (al *AuditLogger) Close() error {
	if al == nil || al.closed.Swap(true) {
		return nil
	}

	close(al.done)
	al.wg.Wait()

	return nil
}

// Clone creates a copy of the AuditConfig.
// Note: IntegritySigner is shared (not cloned) as it maintains internal state.
func (c *AuditConfig) Clone() AuditConfig {
	if c == nil {
		return AuditConfig{}
	}

	return AuditConfig{
		Enabled:          c.Enabled,
		Output:           c.Output,
		BufferSize:       c.BufferSize,
		IncludeTimestamp: c.IncludeTimestamp,
		JSONFormat:       c.JSONFormat,
		MinimumSeverity:  c.MinimumSeverity,
		IntegritySigner:  c.IntegritySigner, // Shared reference
	}
}

// AuditVerificationResult contains the result of audit event verification.
type AuditVerificationResult struct {
	// Valid indicates if the signature is valid.
	Valid bool
	// Event is the parsed audit event (if valid JSON).
	Event *AuditEvent
	// RawEvent is the raw event string without signature.
	RawEvent string
	// Error contains any error encountered during verification.
	Error error
}

// VerifyAuditEvent verifies the integrity of an audit log entry.
// Returns the verification result including the parsed event if valid.
func VerifyAuditEvent(entry string, signer *IntegritySigner) *AuditVerificationResult {
	result := &AuditVerificationResult{}

	if signer == nil {
		result.Valid = false
		result.Error = fmt.Errorf("signer is nil")
		return result
	}

	// Verify the signature
	integrity, err := signer.Verify(entry)
	if err != nil {
		result.Valid = false
		result.Error = err
		return result
	}

	if !integrity.Valid {
		result.Valid = false
		result.RawEvent = integrity.Message
		return result
	}

	result.Valid = true
	result.RawEvent = integrity.Message

	// Try to parse as JSON
	var event AuditEvent
	if err := json.Unmarshal([]byte(result.RawEvent), &event); err == nil {
		result.Event = &event
	}

	return result
}
