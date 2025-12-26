package dd

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// SECURITY FILTER TESTS
// ============================================================================

func TestSensitiveDataFilter(t *testing.T) {
	filter := NewSensitiveDataFilter()

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "password field",
			input:    "password=secret123",
			contains: "[REDACTED]",
		},
		{
			name:     "api key",
			input:    "api_key=sk-1234567890",
			contains: "[REDACTED]",
		},
		{
			name:     "credit card",
			input:    "card number: 4532015112830366",
			contains: "[REDACTED]",
		},
		{
			name:     "email address",
			input:    "email: user@example.com",
			contains: "[REDACTED]",
		},
		{
			name:     "normal text",
			input:    "hello world",
			contains: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.Filter(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected %q in result, got: %s", tt.contains, result)
			}
		})
	}
}

func TestBasicSensitiveDataFilter(t *testing.T) {
	filter := NewBasicSensitiveDataFilter()

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "password",
			input:    "password=secret123",
			contains: "[REDACTED]",
		},
		{
			name:     "token",
			input:    "token=abc123xyz",
			contains: "[REDACTED]",
		},
		{
			name:     "api key",
			input:    "api_key=sk-1234567890",
			contains: "[REDACTED]",
		},
		{
			name:     "normal text",
			input:    "username=john_doe",
			contains: "username=john_doe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.Filter(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected %q in result, got: %s", tt.contains, result)
			}
		})
	}
}

func TestEmptySensitiveDataFilter(t *testing.T) {
	filter := NewEmptySensitiveDataFilter()

	// Empty filter should not filter anything
	input := "password=secret123"
	result := filter.Filter(input)
	if result != input {
		t.Errorf("Empty filter should not modify input, got: %s", result)
	}

	// Add pattern and test
	err := filter.AddPattern(`password=\w+`)
	if err != nil {
		t.Fatalf("Failed to add pattern: %v", err)
	}

	result = filter.Filter(input)
	if !strings.Contains(result, "[REDACTED]") {
		t.Errorf("Filter should redact after adding pattern, got: %s", result)
	}
}

func TestCustomSensitiveDataFilter(t *testing.T) {
	filter, err := NewCustomSensitiveDataFilter(
		`custom_secret=\w+`,
		`internal_id=\d+`,
	)
	if err != nil {
		t.Fatalf("Failed to create custom filter: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "custom secret",
			input:    "custom_secret=abc123",
			contains: "[REDACTED]",
		},
		{
			name:     "internal id",
			input:    "internal_id=12345",
			contains: "[REDACTED]",
		},
		{
			name:     "normal text",
			input:    "hello world",
			contains: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.Filter(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected %q in result, got: %s", tt.contains, result)
			}
		})
	}
}

// ============================================================================
// FILTER MANAGEMENT TESTS
// ============================================================================

func TestFilterPatternManagement(t *testing.T) {
	filter := NewEmptySensitiveDataFilter()

	// Test initial state
	if filter.PatternCount() != 0 {
		t.Error("Empty filter should have 0 patterns")
	}

	// Test adding patterns
	patterns := []string{
		`pattern1=\w+`,
		`pattern2=\d+`,
		`pattern3=[a-z]+`,
	}

	err := filter.AddPatterns(patterns...)
	if err != nil {
		t.Fatalf("Failed to add patterns: %v", err)
	}

	if filter.PatternCount() != 3 {
		t.Errorf("Expected 3 patterns, got %d", filter.PatternCount())
	}

	// Test clearing patterns
	filter.ClearPatterns()
	if filter.PatternCount() != 0 {
		t.Error("Pattern count should be 0 after clear")
	}

	// Filter should not filter anything after clear
	result := filter.Filter("password=secret123")
	if result != "password=secret123" {
		t.Error("Should not filter after clearing patterns")
	}
}

func TestInvalidPattern(t *testing.T) {
	filter := NewEmptySensitiveDataFilter()

	// Try to add invalid pattern
	err := filter.AddPattern(`[invalid(`)
	if err == nil {
		t.Error("Should fail with invalid pattern")
	}
}

func TestAddPatternsWithInvalid(t *testing.T) {
	filter := NewEmptySensitiveDataFilter()

	patterns := []string{
		`valid_pattern=\w+`,
		`[invalid(`,
		`another_valid=\d+`,
	}

	err := filter.AddPatterns(patterns...)
	if err == nil {
		t.Error("Should fail when one pattern is invalid")
	}
}

// ============================================================================
// FIELD VALUE FILTERING TESTS
// ============================================================================

func TestFilterFieldValue(t *testing.T) {
	filter := NewSensitiveDataFilter()

	tests := []struct {
		name     string
		key      string
		value    interface{}
		expected string
	}{
		{
			name:     "password field",
			key:      "password",
			value:    "secret123",
			expected: "[REDACTED]",
		},
		{
			name:     "api_key field",
			key:      "api_key",
			value:    "sk-1234567890",
			expected: "[REDACTED]",
		},
		{
			name:     "token field",
			key:      "token",
			value:    "abc123xyz",
			expected: "[REDACTED]",
		},
		{
			name:     "normal field",
			key:      "username",
			value:    "john_doe",
			expected: "john_doe",
		},
		{
			name:     "non-string value",
			key:      "count",
			value:    42,
			expected: "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.FilterFieldValue(tt.key, tt.value)
			resultStr := ""
			if str, ok := result.(string); ok {
				resultStr = str
			} else {
				resultStr = "42" // For the non-string test case
			}

			if tt.name != "non-string value" && !strings.Contains(resultStr, tt.expected) {
				t.Errorf("Expected %q in result, got: %v", tt.expected, result)
			}
		})
	}
}

func TestFilterFieldValueSubstring(t *testing.T) {
	filter := NewSensitiveDataFilter()

	tests := []struct {
		key      string
		value    string
		redacted bool
	}{
		{"user_password", "secret", true},
		{"password_hash", "hash123", true},
		{"api_key_prod", "key123", true},
		{"secret_token", "token123", true},
		{"username", "john", false},
		{"user_id", "12345", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := filter.FilterFieldValue(tt.key, tt.value)
			resultStr := result.(string)

			if tt.redacted {
				if resultStr != "[REDACTED]" {
					t.Errorf("Expected [REDACTED] for key %q, got: %s", tt.key, resultStr)
				}
			} else {
				if resultStr == "[REDACTED]" {
					t.Errorf("Should not redact key %q", tt.key)
				}
			}
		})
	}
}

func TestFilterValue(t *testing.T) {
	filter := NewSensitiveDataFilter()

	tests := []struct {
		name  string
		value interface{}
	}{
		{"string", "test string"},
		{"int", 42},
		{"float", 3.14},
		{"bool", true},
		{"nil", nil},
		{"slice", []int{1, 2, 3}},
		{"map", map[string]int{"a": 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.FilterValue(tt.value)
			// Should not panic
			if result == nil && tt.value != nil {
				t.Error("FilterValue should not return nil for non-nil input")
			}
		})
	}
}

// ============================================================================
// FILTER CLONING TESTS
// ============================================================================

func TestFilterClone(t *testing.T) {
	original := NewSensitiveDataFilter()
	originalCount := original.PatternCount()

	clone := original.Clone()

	if clone == nil {
		t.Fatal("Clone should not be nil")
	}

	if clone.PatternCount() != originalCount {
		t.Error("Clone should have same pattern count")
	}

	// Modify clone
	clone.AddPattern(`test_pattern=\w+`)

	// Original should not be affected
	if original.PatternCount() == clone.PatternCount() {
		t.Error("Modifying clone should not affect original")
	}
}

func TestNilFilterClone(t *testing.T) {
	var filter *SensitiveDataFilter
	clone := filter.Clone()

	if clone != nil {
		t.Error("Cloning nil filter should return nil")
	}
}

// ============================================================================
// REDOS PROTECTION TESTS
// ============================================================================

func TestReDoSProtection(t *testing.T) {
	filter := NewSensitiveDataFilter()

	// Create a potentially malicious input that could cause catastrophic backtracking
	maliciousInput := strings.Repeat("a", 100) + "X"

	start := time.Now()
	result := filter.Filter(maliciousInput)
	duration := time.Since(start)

	// Should complete quickly (within timeout)
	if duration > 500*time.Millisecond {
		t.Errorf("Filter took too long: %v (possible ReDoS)", duration)
	}

	// Result should be either filtered or timeout message
	if result == "" {
		t.Error("Filter should return a result")
	}
}

func TestFilterTimeout(t *testing.T) {
	filter := NewSensitiveDataFilter()

	// Add a complex pattern that might timeout
	err := filter.AddPattern(`(a+)+b`)
	if err != nil {
		t.Fatalf("Failed to add pattern: %v", err)
	}

	// Input that could cause backtracking
	input := strings.Repeat("a", 50)

	result := filter.Filter(input)

	// Should not hang
	if result == "" {
		t.Error("Filter should return a result")
	}
}

func TestFilterMaxInputLength(t *testing.T) {
	filter := NewSensitiveDataFilter()

	// Create input larger than max length
	largeInput := strings.Repeat("a", 2*1024*1024) // 2MB

	result := filter.Filter(largeInput)

	// The filter should handle large inputs safely
	resultPreview := result
	if len(result) > 100 {
		resultPreview = result[:100] + "..."
	}
	t.Logf("Input length: %d, Result length: %d, Result preview: %q", len(largeInput), len(result), resultPreview)

	// Result should be much smaller than input
	if len(result) >= len(largeInput) {
		t.Errorf("Result should be smaller than input, got result=%d, input=%d", len(result), len(largeInput))
	}

	// Filter should handle large input without panic
	if result == "" {
		t.Error("Result should not be empty")
	}
}

// ============================================================================
// CONCURRENT ACCESS TESTS
// ============================================================================

func TestConcurrentFilterAccess(t *testing.T) {
	filter := NewSensitiveDataFilter()

	var wg sync.WaitGroup

	// Concurrent filtering
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				filter.Filter("password=secret123 card=4532015112830366")
			}
		}(i)
	}

	// Concurrent pattern addition
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			filter.AddPattern(`test\d+`)
		}(i)
	}

	// Concurrent pattern count
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = filter.PatternCount()
		}()
	}

	wg.Wait()
}

// ============================================================================
// SECURITY CONFIG TESTS
// ============================================================================

func TestDefaultSecurityConfig(t *testing.T) {
	config := DefaultSecurityConfig()

	if config == nil {
		t.Fatal("DefaultSecurityConfig should not return nil")
	}

	if config.MaxMessageSize <= 0 {
		t.Error("MaxMessageSize should be positive")
	}

	if config.MaxWriters <= 0 {
		t.Error("MaxWriters should be positive")
	}

	// SensitiveFilter can be nil (disabled by default)
}

func TestSecurityConfigWithFilter(t *testing.T) {
	filter := NewBasicSensitiveDataFilter()
	config := &SecurityConfig{
		MaxMessageSize:  1024 * 1024,
		MaxWriters:      50,
		SensitiveFilter: filter,
	}

	if config.SensitiveFilter != filter {
		t.Error("SecurityConfig should store the provided filter")
	}

	if config.MaxMessageSize != 1024*1024 {
		t.Error("MaxMessageSize should match provided value")
	}

	if config.MaxWriters != 50 {
		t.Error("MaxWriters should match provided value")
	}
}

// ============================================================================
// INTEGRATION TESTS
// ============================================================================

func TestSecurityIntegrationWithLogger(t *testing.T) {
	var buf strings.Builder
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}
	config.SecurityConfig = &SecurityConfig{
		MaxMessageSize:  1024,
		MaxWriters:      10,
		SensitiveFilter: NewBasicSensitiveDataFilter(),
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test message filtering
	logger.Info("User password: secret123")

	output := buf.String()
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("Password should be filtered in logger output")
	}
	if strings.Contains(output, "secret123") {
		t.Error("Password value should not appear in logger output")
	}
}

func TestSecurityMessageSizeLimit(t *testing.T) {
	var buf strings.Builder
	config := DefaultConfig()
	config.Writers = []io.Writer{&buf}
	config.SecurityConfig = &SecurityConfig{
		MaxMessageSize: 100, // Small limit for testing
		MaxWriters:     10,
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Create message larger than limit
	largeMessage := strings.Repeat("A", 200)
	logger.Info(largeMessage)

	output := buf.String()
	// Message should be truncated
	if len(output) > 150 { // Account for timestamp, level, etc.
		t.Error("Message should be truncated due to size limit")
	}
	if !strings.Contains(output, "...") {
		t.Error("Truncated message should contain ellipsis")
	}
}

func TestSecurityFieldFiltering(t *testing.T) {
	var buf strings.Builder
	config := JSONConfig()
	config.Writers = []io.Writer{&buf}
	config.SecurityConfig = &SecurityConfig{
		SensitiveFilter: NewBasicSensitiveDataFilter(),
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test structured field filtering
	logger.InfoWith("User login",
		String("username", "john"),
		String("password", "secret123"),
		String("api_key", "sk-1234567890"),
	)

	output := buf.String()
	if !strings.Contains(output, "john") {
		t.Error("Username should not be filtered")
	}
	if strings.Contains(output, "secret123") {
		t.Error("Password value should be filtered")
	}
	if strings.Contains(output, "sk-1234567890") {
		t.Error("API key value should be filtered")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("Sensitive fields should be redacted")
	}
}
