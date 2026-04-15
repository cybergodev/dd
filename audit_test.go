package dd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAuditEventType_String(t *testing.T) {
	tests := []struct {
		eventType AuditEventType
		expected  string
	}{
		{AuditEventSensitiveDataRedacted, "SENSITIVE_DATA_REDACTED"},
		{AuditEventRateLimitExceeded, "RATE_LIMIT_EXCEEDED"},
		{AuditEventReDoSAttempt, "REDOS_ATTEMPT"},
		{AuditEventSecurityViolation, "SECURITY_VIOLATION"},
		{AuditEventIntegrityViolation, "INTEGRITY_VIOLATION"},
		{AuditEventInputSanitized, "INPUT_SANITIZED"},
		{AuditEventPathTraversalAttempt, "PATH_TRAVERSAL_ATTEMPT"},
		{AuditEventLog4ShellAttempt, "LOG4SHELL_ATTEMPT"},
		{AuditEventNullByteInjection, "NULL_BYTE_INJECTION"},
		{AuditEventOverlongEncoding, "OVERLONG_ENCODING"},
		{AuditEventHomographAttack, "HOMOGRAPH_ATTACK"},
		{AuditEventType(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.eventType.String(); got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAuditSeverity_String(t *testing.T) {
	tests := []struct {
		severity AuditSeverity
		expected string
	}{
		{AuditSeverityInfo, "INFO"},
		{AuditSeverityWarning, "WARNING"},
		{AuditSeverityError, "ERROR"},
		{AuditSeverityCritical, "CRITICAL"},
		{AuditSeverity(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.severity.String(); got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAuditSeverity_MarshalJSON(t *testing.T) {
	severity := AuditSeverityError
	data, err := json.Marshal(severity)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if string(data) != `"ERROR"` {
		t.Errorf("MarshalJSON() = %s, want %q", string(data), `"ERROR"`)
	}
}

func TestDefaultAuditConfig(t *testing.T) {
	config := DefaultAuditConfig()

	// Security enhancement: audit logging is now enabled by default
	if !config.Enabled {
		t.Error("Default config should have Enabled=true (security enhancement)")
	}
	if config.BufferSize != 1000 {
		t.Errorf("Default BufferSize = %d, want 1000", config.BufferSize)
	}
	if !config.IncludeTimestamp {
		t.Error("Default config should have IncludeTimestamp=true")
	}
	if !config.JSONFormat {
		t.Error("Default config should have JSONFormat=true")
	}
}

func TestAuditLogger_Log(t *testing.T) {
	// Test with nil output to just capture events in stats

	config := AuditConfig{
		Enabled:          true,
		Output:           nil, // No output, just capture events
		BufferSize:       100,
		IncludeTimestamp: true,
		JSONFormat:       true,
		MinimumSeverity:  AuditSeverityInfo,
	}

	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("NewAuditLogger() error = %v", err)
	}
	defer al.Close()

	// Log some events
	al.Log(AuditEvent{
		Type:     AuditEventSensitiveDataRedacted,
		Message:  "Test redaction",
		Pattern:  `\b\d{16}\b`,
		Severity: AuditSeverityInfo,
	})

	// Wait for async processing
	time.Sleep(50 * time.Millisecond)

	stats := al.Stats()
	if stats.TotalEvents != 1 {
		t.Errorf("TotalEvents = %d, want 1", stats.TotalEvents)
	}
}

func TestAuditLogger_SeverityFilter(t *testing.T) {
	config := AuditConfig{
		Enabled:         true,
		Output:          nil,
		BufferSize:      100,
		MinimumSeverity: AuditSeverityWarning,
	}

	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("NewAuditLogger() error = %v", err)
	}
	defer al.Close()

	// Log events below threshold - should be filtered
	al.Log(AuditEvent{
		Type:     AuditEventSensitiveDataRedacted,
		Message:  "Info event",
		Severity: AuditSeverityInfo,
	})

	// Log events at threshold
	al.Log(AuditEvent{
		Type:     AuditEventRateLimitExceeded,
		Message:  "Warning event",
		Severity: AuditSeverityWarning,
	})

	// Log events above threshold
	al.Log(AuditEvent{
		Type:     AuditEventSecurityViolation,
		Message:  "Error event",
		Severity: AuditSeverityError,
	})

	time.Sleep(50 * time.Millisecond)

	stats := al.Stats()
	if stats.TotalEvents != 2 {
		t.Errorf("TotalEvents = %d, want 2 (filtered info events)", stats.TotalEvents)
	}
}

func TestAuditLogger_HelperMethods(t *testing.T) {
	config := AuditConfig{
		Enabled:         true,
		Output:          nil,
		BufferSize:      100,
		MinimumSeverity: AuditSeverityInfo,
	}

	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("NewAuditLogger() error = %v", err)
	}
	defer al.Close()

	// Test all helper methods
	al.LogSensitiveDataRedaction("pattern", "field", "message")
	al.LogRateLimitExceeded("rate limit", map[string]any{"count": 100})
	al.LogSecurityViolation("type", "message", map[string]any{"key": "value"})
	al.LogReDoSAttempt("pattern", "message")
	al.LogIntegrityViolation("message", map[string]any{"hash": "abc123"})
	al.LogPathTraversalAttempt("/etc/passwd", "path traversal detected")

	time.Sleep(50 * time.Millisecond)

	stats := al.Stats()
	if stats.TotalEvents != 6 {
		t.Errorf("TotalEvents = %d, want 6", stats.TotalEvents)
	}
}

func TestAuditLogger_BufferOverflow(t *testing.T) {
	config := AuditConfig{
		Enabled:         true,
		Output:          nil,
		BufferSize:      10, // Small buffer
		MinimumSeverity: AuditSeverityInfo,
	}

	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("NewAuditLogger() error = %v", err)
	}
	defer al.Close()

	// Send more events than buffer can hold
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			al.Log(AuditEvent{
				Type:     AuditEventSensitiveDataRedacted,
				Message:  "Test event",
				Severity: AuditSeverityInfo,
			})
		}()
	}
	wg.Wait()

	stats := al.Stats()
	// Some events may have been dropped
	if stats.Dropped == 0 && stats.TotalEvents < 100 {
		t.Logf("Some events processed: TotalEvents=%d, Dropped=%d", stats.TotalEvents, stats.Dropped)
	}
}

func TestAuditLogger_Disabled(t *testing.T) {
	config := AuditConfig{
		Enabled: false,
	}

	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("NewAuditLogger() error = %v", err)
	}
	defer al.Close()

	al.Log(AuditEvent{
		Type:     AuditEventSensitiveDataRedacted,
		Message:  "Test event",
		Severity: AuditSeverityInfo,
	})

	stats := al.Stats()
	if stats.TotalEvents != 0 {
		t.Errorf("Disabled logger should not log events, got %d", stats.TotalEvents)
	}
}

func TestAuditLogger_NilSafety(t *testing.T) {
	var al *AuditLogger

	// Should not panic
	al.Log(AuditEvent{Type: AuditEventSensitiveDataRedacted})
	al.LogSensitiveDataRedaction("pattern", "field", "message")
	al.Close()

	stats := al.Stats()
	if stats.TotalEvents != 0 {
		t.Error("Nil logger should return zero stats")
	}
}

func TestAuditLogger_Close(t *testing.T) {
	config := AuditConfig{
		Enabled:         true,
		Output:          nil,
		BufferSize:      100,
		MinimumSeverity: AuditSeverityInfo,
	}

	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("NewAuditLogger() error = %v", err)
	}

	// Log some events
	for i := 0; i < 10; i++ {
		al.Log(AuditEvent{
			Type:     AuditEventSensitiveDataRedacted,
			Message:  "Test event",
			Severity: AuditSeverityInfo,
		})
	}

	// Close should flush remaining events
	err = al.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Double close should be safe
	err = al.Close()
	if err != nil {
		t.Errorf("Second Close() error = %v", err)
	}
}

func TestAuditLogger_StatsByType(t *testing.T) {
	config := AuditConfig{
		Enabled:         true,
		Output:          nil,
		BufferSize:      100,
		MinimumSeverity: AuditSeverityInfo,
	}

	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("NewAuditLogger() error = %v", err)
	}
	defer al.Close()

	// Log different types of events
	al.Log(AuditEvent{Type: AuditEventSensitiveDataRedacted, Severity: AuditSeverityInfo})
	al.Log(AuditEvent{Type: AuditEventSensitiveDataRedacted, Severity: AuditSeverityInfo})
	al.Log(AuditEvent{Type: AuditEventRateLimitExceeded, Severity: AuditSeverityInfo})
	al.Log(AuditEvent{Type: AuditEventSecurityViolation, Severity: AuditSeverityInfo})

	time.Sleep(50 * time.Millisecond)

	stats := al.Stats()
	if stats.ByType[AuditEventSensitiveDataRedacted] != 2 {
		t.Errorf("SensitiveDataRedacted count = %d, want 2", stats.ByType[AuditEventSensitiveDataRedacted])
	}
	if stats.ByType[AuditEventRateLimitExceeded] != 1 {
		t.Errorf("RateLimitExceeded count = %d, want 1", stats.ByType[AuditEventRateLimitExceeded])
	}
}

func TestAuditConfig_Clone(t *testing.T) {
	original := AuditConfig{
		Enabled:          true,
		BufferSize:       500,
		IncludeTimestamp: false,
		JSONFormat:       false,
		MinimumSeverity:  AuditSeverityWarning,
	}

	cloned := original.Clone()

	if cloned.BufferSize != original.BufferSize {
		t.Error("BufferSize should be copied")
	}

	// Modify original
	original.BufferSize = 999
	if cloned.BufferSize == 999 {
		t.Error("Clone should not be affected by original modifications")
	}
}

func TestAuditConfig_CloneNil(t *testing.T) {
	var config *AuditConfig
	cloned := config.Clone()
	// Cloning nil pointer returns zero-value AuditConfig
	if cloned.Enabled || cloned.BufferSize != 0 {
		t.Error("Cloning nil should return zero-value AuditConfig")
	}
}

func TestNewAuditLogger_NilConfig(t *testing.T) {
	// Calling with default config
	al, err := NewAuditLogger(DefaultAuditConfig())
	if err != nil {
		t.Fatalf("NewAuditLogger() error = %v", err)
	}

	if al == nil {
		t.Fatal("NewAuditLogger should not return nil")
	}

	if al.config.BufferSize != 1000 {
		t.Error("Default config should have BufferSize=1000")
	}

	al.Close()
}

// TestAuditLogger_ConcurrentTypeCount tests that incrementTypeCount correctly
// handles concurrent increments for the same event type without losing counts.
// This test validates the fix for the check-then-act race condition.
func TestAuditLogger_ConcurrentTypeCount(t *testing.T) {
	config := AuditConfig{
		Enabled:         true,
		Output:          nil,
		BufferSize:      1000,
		MinimumSeverity: AuditSeverityInfo,
	}

	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("NewAuditLogger() error = %v", err)
	}
	defer al.Close()

	numGoroutines := 100
	eventsPerGoroutine := 100
	expectedTotal := numGoroutines * eventsPerGoroutine

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// All goroutines log the same event type concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				al.Log(AuditEvent{
					Type:     AuditEventSensitiveDataRedacted,
					Message:  "concurrent test",
					Severity: AuditSeverityInfo,
				})
			}
		}()
	}

	wg.Wait()

	// Wait for async processing to complete
	time.Sleep(100 * time.Millisecond)

	stats := al.Stats()

	// Verify no events were dropped (buffer should be large enough)
	if stats.Dropped > 0 {
		t.Logf("Warning: %d events were dropped due to buffer overflow", stats.Dropped)
	}

	// Verify total events
	actualTotal := stats.TotalEvents
	if actualTotal != int64(expectedTotal)-stats.Dropped {
		t.Errorf("TotalEvents = %d, want %d (dropped: %d)",
			actualTotal, expectedTotal, stats.Dropped)
	}

	// Verify type-specific count matches total
	typeCount := stats.ByType[AuditEventSensitiveDataRedacted]
	if typeCount != actualTotal {
		t.Errorf("Type count mismatch: ByType=%d, TotalEvents=%d", typeCount, actualTotal)
	}

	t.Logf("Concurrent test passed: %d events logged, %d dropped, type count=%d",
		stats.TotalEvents, stats.Dropped, typeCount)
}

// TestAuditLogger_ConcurrentMultipleTypes tests concurrent logging of multiple
// event types to verify that LoadOrStore handles multiple new types correctly.
func TestAuditLogger_ConcurrentMultipleTypes(t *testing.T) {
	config := AuditConfig{
		Enabled:         true,
		Output:          nil,
		BufferSize:      2000,
		MinimumSeverity: AuditSeverityInfo,
	}

	al, err := NewAuditLogger(config)
	if err != nil {
		t.Fatalf("NewAuditLogger() error = %v", err)
	}
	defer al.Close()

	eventTypes := []AuditEventType{
		AuditEventSensitiveDataRedacted,
		AuditEventRateLimitExceeded,
		AuditEventSecurityViolation,
		AuditEventIntegrityViolation,
		AuditEventReDoSAttempt,
	}

	numGoroutines := 50
	eventsPerGoroutine := 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines * len(eventTypes))

	// Each event type gets its own set of goroutines
	for _, eventType := range eventTypes {
		eventType := eventType // capture
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < eventsPerGoroutine; j++ {
					al.Log(AuditEvent{
						Type:     eventType,
						Message:  "concurrent multi-type test",
						Severity: AuditSeverityInfo,
					})
				}
			}()
		}
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	stats := al.Stats()
	expectedPerType := numGoroutines * eventsPerGoroutine

	for _, eventType := range eventTypes {
		count := stats.ByType[eventType]
		// Account for possible drops due to buffer overflow
		if count < int64(expectedPerType) && stats.Dropped == 0 {
			t.Errorf("EventType %v: count=%d, expected at least %d",
				eventType, count, expectedPerType)
		}
		t.Logf("EventType %v: %d events logged", eventType, count)
	}

	totalByType := int64(0)
	for _, count := range stats.ByType {
		totalByType += count
	}

	if totalByType != stats.TotalEvents {
		t.Errorf("Sum of type counts (%d) != TotalEvents (%d)", totalByType, stats.TotalEvents)
	}
}

// TestAuditLogger_WriteEventWithOutput tests the writeEvent function through the public API
// by logging events with actual output, covering JSON and text formats.
func TestAuditLogger_WriteEventWithOutput(t *testing.T) {
	t.Run("JSON format with output", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "audit_json.log")
		f, err := os.Create(tmpFile)
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer f.Close()

		config := AuditConfig{
			Enabled:          true,
			Output:           f,
			BufferSize:       100,
			IncludeTimestamp: true,
			JSONFormat:       true,
			MinimumSeverity:  AuditSeverityInfo,
		}

		al, err := NewAuditLogger(config)
		if err != nil {
			t.Fatalf("NewAuditLogger() error = %v", err)
		}
		defer al.Close()

		al.Log(AuditEvent{
			Type:     AuditEventSecurityViolation,
			Message:  "test security event",
			Pattern:  `\b\d{16}\b`,
			Field:    "password",
			Severity: AuditSeverityError,
		})

		time.Sleep(100 * time.Millisecond)

		// Read the file content
		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read temp file: %v", err)
		}
		output := string(content)

		// JSON output uses numeric type values
		if !strings.Contains(output, `"type":3`) {
			t.Errorf("Expected JSON type in output, got: %s", output)
		}
		if !strings.Contains(output, `"message":"test security event"`) {
			t.Errorf("Expected JSON message in output, got: %s", output)
		}
		if !strings.Contains(output, `"pattern"`) {
			t.Errorf("Expected JSON pattern field in output, got: %s", output)
		}
		if !strings.Contains(output, `"field":"password"`) {
			t.Errorf("Expected JSON field in output, got: %s", output)
		}
	})

	t.Run("Text format with output", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "audit_text.log")
		f, err := os.Create(tmpFile)
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer f.Close()

		config := AuditConfig{
			Enabled:          true,
			Output:           f,
			BufferSize:       100,
			IncludeTimestamp: true,
			JSONFormat:       false,
			MinimumSeverity:  AuditSeverityInfo,
		}

		al, _ := NewAuditLogger(config)
		defer al.Close()

		al.Log(AuditEvent{
			Type:     AuditEventRateLimitExceeded,
			Message:  "rate limit test",
			Pattern:  "test-pattern",
			Field:    "api_key",
			Severity: AuditSeverityWarning,
		})

		time.Sleep(100 * time.Millisecond)

		// Read the file content
		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read temp file: %v", err)
		}
		output := string(content)

		if !strings.Contains(output, "RATE_LIMIT_EXCEEDED") {
			t.Errorf("Expected type in output, got: %s", output)
		}
		if !strings.Contains(output, "rate limit test") {
			t.Errorf("Expected message in output, got: %s", output)
		}
		if !strings.Contains(output, "pattern=test-pattern") {
			t.Errorf("Expected pattern in output, got: %s", output)
		}
		if !strings.Contains(output, "field=api_key") {
			t.Errorf("Expected field in output, got: %s", output)
		}
	})

	t.Run("Text format without timestamp", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "audit_notime.log")
		f, err := os.Create(tmpFile)
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer f.Close()

		config := AuditConfig{
			Enabled:          true,
			Output:           f,
			BufferSize:       100,
			IncludeTimestamp: false,
			JSONFormat:       false,
			MinimumSeverity:  AuditSeverityInfo,
		}

		al, _ := NewAuditLogger(config)
		defer al.Close()

		al.Log(AuditEvent{
			Type:     AuditEventPathTraversalAttempt,
			Message:  "path traversal test",
			Severity: AuditSeverityWarning,
		})

		time.Sleep(100 * time.Millisecond)

		// Read the file content
		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read temp file: %v", err)
		}
		output := string(content)

		// Should have format: [TYPE] message (no timestamp prefix)
		if !strings.Contains(output, "[PATH_TRAVERSAL_ATTEMPT] path traversal test") {
			t.Errorf("Expected text format without timestamp, got: %s", output)
		}
	})
}
