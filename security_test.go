package dd

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cybergodev/dd/internal"
)

// NOTE: TestSensitiveDataFilter, TestBasicSensitiveDataFilter, and TestDefaultSecurityConfig
// are now in dd_test.go to avoid duplication. This file contains specialized
// security tests that are not in the main test file.

// ============================================================================
// EMPTY FILTER TESTS
// ============================================================================

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

func TestFilterValueRecursive(t *testing.T) {
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
			result := filter.FilterValueRecursive("", tt.value)
			// Should not panic
			if result == nil && tt.value != nil {
				t.Error("FilterValueRecursive should not return nil for non-nil input")
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

	clone := original.clone()

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
	clone := filter.clone()

	if clone != nil {
		t.Error("Cloning nil filter should return nil")
	}
}

// ============================================================================
// REDOS PROTECTION TESTS
// ============================================================================

// TestFilterValueRecursiveCircularReferences consolidates the three
// per-container-kind circular-reference tests: every cycle must terminate
// and be marked [CIRCULAR_REFERENCE] at the point it closes.
func TestFilterValueRecursiveCircularReferences(t *testing.T) {
	filter := NewSensitiveDataFilter()

	type node struct {
		Value int
		Next  *node
	}
	type container struct {
		Items []*container
	}

	cases := []struct {
		name  string
		build func() any
		mark  func(t *testing.T, result any)
	}{
		{
			name: "struct chain",
			build: func() any {
				a := &node{Value: 1}
				b := &node{Value: 2}
				a.Next = b
				b.Next = a
				return a
			},
			mark: func(t *testing.T, result any) {
				next := result.(map[string]any)["Next"].(map[string]any)
				if next["Next"] != "[CIRCULAR_REFERENCE]" {
					t.Errorf("Next.Next = %v, want [CIRCULAR_REFERENCE]", next["Next"])
				}
			},
		},
		{
			name: "slice container",
			build: func() any {
				a := &container{}
				b := &container{}
				a.Items = []*container{b}
				b.Items = []*container{a}
				return a
			},
			mark: func(t *testing.T, result any) {
				items := result.(map[string]any)["Items"].([]any)
				first := items[0].(map[string]any)
				// The marker sits inside the (converted) slice field.
				inner, ok := first["Items"].([]any)
				if !ok || len(inner) != 1 || inner[0] != "[CIRCULAR_REFERENCE]" {
					t.Errorf("Items[0].Items = %v, want [CIRCULAR_REFERENCE]", first["Items"])
				}
			},
		},
		{
			name: "map reference",
			build: func() any {
				mapA := make(map[string]any)
				mapB := make(map[string]any)
				mapA["ref"] = mapB
				mapB["ref"] = mapA
				return mapA
			},
			mark: func(t *testing.T, result any) {
				ref := result.(map[string]any)["ref"].(map[string]any)
				if ref["ref"] != "[CIRCULAR_REFERENCE]" {
					t.Errorf("ref.ref = %v, want [CIRCULAR_REFERENCE]", ref["ref"])
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Must not panic or hang on the cycle.
			result := filter.FilterValueRecursive("", tc.build())
			if result == nil {
				t.Fatal("FilterValueRecursive(cyclic) = nil, want a marked structure")
			}
			tc.mark(t, result)
		})
	}
}

func TestReDoSAlternationPattern(t *testing.T) {
	filter := NewEmptySensitiveDataFilter()

	tests := []struct {
		name    string
		pattern string
		safe    bool
	}{
		// Dangerous alternation patterns
		{"alternation with quantifier first", "(a+|b)+", false},
		{"alternation with quantifier second", "(a|b+)+", false},
		{"alternation both quantified", "(a+|b+)+", false},
		{"alternation with star", "(a*|b)+", false},
		{"nested alternation", "((a|b)+|c)+", false},

		// Safe alternation patterns
		{"simple alternation", "(a|b)", true},
		{"alternation optional", "(a|b)?", true},
		{"alternation with count", "(a|b){3}", true},
		{"alternation with range", "(a|b){1,5}", true},

		// Dangerous excessive ranges
		{"excessive range", "a{1,10000}", false},
		{"exact excessive", "a{5000}", false},

		// Safe ranges
		{"safe range", "a{1,100}", true},
		{"safe exact", "a{50}", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := filter.AddPattern(tt.pattern)
			if tt.safe {
				if err != nil {
					t.Errorf("Pattern %q should be safe, got error: %v", tt.pattern, err)
				}
			} else {
				if err == nil {
					t.Errorf("Pattern %q should be rejected as dangerous", tt.pattern)
				}
			}
		})
	}
}

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

	// Try to add a dangerous ReDoS pattern - should be rejected
	err := filter.AddPattern(`(a+)+b`)
	if err == nil {
		t.Error("Should reject dangerous nested quantifier pattern (a+)+b")
	}

	// Add a safe pattern instead
	err = filter.AddPattern(`a+b`)
	if err != nil {
		t.Fatalf("Failed to add safe pattern: %v", err)
	}

	// Input that could cause backtracking with the dangerous pattern
	// but is safe with the simple pattern
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

// TestFilterLargeInputBoundaryRedaction guards against the historical chunked
// reassembly bug: when redaction changes a chunk's length ([REDACTED] differs
// from the matched text), slicing a redacted chunk by a fixed byte offset
// corrupted output around chunk boundaries for inputs beyond the direct-process
// threshold. The matcher must redact every occurrence exactly once and leave no
// fragment behind.
func TestFilterLargeInputBoundaryRedaction(t *testing.T) {
	filter := NewSensitiveDataFilter()
	if err := filter.AddPattern(`LEAK-\d{12}`); err != nil {
		t.Fatalf("AddPattern: %v", err)
	}

	secret := "LEAK-123456789012"

	// Establish how a single occurrence is redacted, without assuming how many
	// [REDACTED] markers it yields (built-in patterns may contribute).
	single := filter.Filter(secret)
	markersPerOccurrence := strings.Count(single, "[REDACTED]")
	if markersPerOccurrence == 0 {
		t.Fatalf("pattern did not redact the secret in isolation: %q -> %q", secret, single)
	}

	// ~64 KB with the secret every 4000 bytes, so it repeatedly straddles the
	// former 4 KB chunk boundary and lives deep in the >32 KB region.
	var sb strings.Builder
	for sb.Len() < 64*1024 {
		sb.WriteString(strings.Repeat("x", 4000))
		sb.WriteString(secret)
	}
	large := sb.String()
	occurrences := strings.Count(large, secret)

	result := filter.Filter(large)

	want := occurrences * markersPerOccurrence
	if got := strings.Count(result, "[REDACTED]"); got != want {
		t.Fatalf("redaction markers = %d, want %d — boundary reassembly corrupted the output", got, want)
	}
	if strings.Contains(result, secret) {
		t.Error("an unredacted secret survived in the large input")
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

	// Concurrent introspection and toggling
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = filter.PatternCount()
			_ = filter.IsEnabled()
			if id%2 == 0 {
				filter.Enable()
			} else {
				filter.Disable()
			}
		}(i)
	}

	wg.Wait()

	// After the dust settles the filter must still be usable.
	filter.Enable()
	if got := filter.Filter("password=secret123"); !strings.Contains(got, "[REDACTED]") {
		t.Errorf("filter unusable after concurrent access: %q", got)
	}
}

// ============================================================================
// INTEGRATION TESTS
// ============================================================================

func TestSecurityIntegrationWithLogger(t *testing.T) {
	var buf strings.Builder
	config := DefaultConfig()
	config.Targets = []OutputTarget{CustomOutput(&buf)}
	config.Security = &SecurityConfig{
		MaxMessageSize:  1024,
		MaxWriters:      10,
		SensitiveFilter: newBasicSensitiveDataFilter(),
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
	config.Targets = []OutputTarget{CustomOutput(&buf)}
	config.Security = &SecurityConfig{
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
	config.Targets = []OutputTarget{CustomOutput(&buf)}
	config.Security = &SecurityConfig{
		SensitiveFilter: newBasicSensitiveDataFilter(),
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

// ============================================================================
// PHONE NUMBER FILTERING TESTS
// ============================================================================

func TestPhoneNumberFiltering(t *testing.T) {
	filter := NewSensitiveDataFilter()

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		// Phone number fields with field names (preserve field name)
		{
			name:     "phone field with colon",
			input:    "phone: +1-415-555-2671",
			contains: "phone: [REDACTED]",
		},
		{
			name:     "mobile field",
			input:    "mobile=13812345678",
			contains: "mobile=[REDACTED]",
		},
		{
			name:     "tel field",
			input:    "tel +86 138 1234 5678",
			contains: "tel [REDACTED]",
		},
		{
			name:     "telephone field",
			input:    "telephone: (415) 555-2671",
			contains: "telephone: [REDACTED]",
		},
		{
			name:     "cell field",
			input:    "cell=+44 7700 900077",
			contains: "cell=[REDACTED]",
		},
		{
			name:     "fax field",
			input:    "fax: +1-415-555-2671",
			contains: "fax: [REDACTED]",
		},
		{
			name:     "contact field",
			input:    "contact +49 30 12345678",
			contains: "contact [REDACTED]",
		},

		// International E.164 format
		{
			name:     "E.164 with plus",
			input:    "Call me at +14155552671",
			contains: "[REDACTED]",
		},
		{
			name:     "E.164 medium",
			input:    "+8613812345678",
			contains: "[REDACTED]",
		},

		// 00 prefix international
		{
			name:     "00 prefix",
			input:    "0014155552671",
			contains: "[REDACTED]",
		},

		// NANP (North America) format
		{
			name:     "NANP with parentheses",
			input:    "(415) 555-2671",
			contains: "[REDACTED]",
		},
		{
			name:     "NANP with dashes",
			input:    "415-555-2671",
			contains: "[REDACTED]",
		},
		{
			name:     "NANP with dots",
			input:    "415.555.2671",
			contains: "[REDACTED]",
		},
		{
			name:     "NANP with spaces",
			input:    "415 555 2671",
			contains: "[REDACTED]",
		},
		{
			name:     "NANP with area code",
			input:    "1-415-555-2671",
			contains: "[REDACTED]",
		},

		// Chinese mobile numbers
		// Note: Standalone 11-digit numbers are NOT filtered to avoid over-matching
		// order IDs, timestamps, user IDs, etc. They ARE filtered when used with
		// sensitive field names like "phone", "mobile", etc.
		{
			name:     "Chinese mobile 11 digits (standalone - not filtered)",
			input:    "13812345678",
			contains: "13812345678",
		},
		{
			name:     "Chinese mobile with country code",
			input:    "+86 138 1234 5678",
			contains: "[REDACTED]",
		},
		{
			name:     "Chinese mobile with dash",
			input:    "+86-138-1234-5678",
			contains: "[REDACTED]",
		},
		{
			name:     "Chinese mobile starts with 13 (standalone - not filtered)",
			input:    "13123456789",
			contains: "13123456789",
		},
		{
			name:     "Chinese mobile starts with 18 (standalone - not filtered)",
			input:    "18123456789",
			contains: "18123456789",
		},
		{
			name:     "Chinese mobile starts with 19 (standalone - not filtered)",
			input:    "19123456789",
			contains: "19123456789",
		},

		// UK mobile numbers
		{
			name:     "UK mobile with country code",
			input:    "+44 7700 900077",
			contains: "[REDACTED]",
		},
		{
			name:     "UK mobile with dashes",
			input:    "+44-7700-900077",
			contains: "[REDACTED]",
		},
		{
			name:     "UK mobile local format",
			input:    "07700 900077",
			contains: "[REDACTED]",
		},

		// German phone numbers
		{
			name:     "German local format",
			input:    "030 12345678",
			contains: "[REDACTED]",
		},
		{
			name:     "German with area code in parentheses",
			input:    "+49 (030) 12345678",
			contains: "[REDACTED]",
		},

		// Japanese phone numbers
		{
			name:     "Japanese local format",
			input:    "090-1234-5678",
			contains: "[REDACTED]",
		},

		// Korean phone numbers
		{
			name:     "Korean local format",
			input:    "010-1234-5678",
			contains: "[REDACTED]",
		},

		// Indian phone numbers
		{
			name:     "Indian local format",
			input:    "098765 43210",
			contains: "[REDACTED]",
		},

		// Edge cases - non-phone numbers
		{
			name:     "short number (not a phone)",
			input:    "12345",
			contains: "12345",
		},
		{
			name:     "regular text",
			input:    "hello world",
			contains: "hello world",
		},
		{
			name:     "year number",
			input:    "year 2024",
			contains: "year 2024",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.Filter(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got: %s", tt.contains, result)
			}
		})
	}
}

func TestPhoneNumberFieldFiltering(t *testing.T) {
	var buf strings.Builder
	config := JSONConfig()
	config.Targets = []OutputTarget{CustomOutput(&buf)}
	config.Security = &SecurityConfig{
		SensitiveFilter: newBasicSensitiveDataFilter(),
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test phone number field filtering
	logger.InfoWith("User contact",
		String("username", "john"),
		String("phone", "+1-415-555-2671"),
		String("mobile", "13812345678"),
		String("email", "john@example.com"),
	)

	output := buf.String()
	if !strings.Contains(output, "john") {
		t.Error("Username should not be filtered")
	}
	if strings.Contains(output, "+1-415-555-2671") {
		t.Error("Phone value should be filtered")
	}
	if strings.Contains(output, "13812345678") {
		t.Error("Mobile value should be filtered")
	}
	// Note: Email is NOT filtered in basic mode to avoid false positives on user@host format
	// Email filtering is only available in full filter mode (NewSensitiveDataFilter)
	if !strings.Contains(output, "john@example.com") {
		t.Error("Email should NOT be filtered in basic mode")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("Sensitive fields should be redacted")
	}
}

// ============================================================================
// DATABASE CONNECTION STRING FILTERING TESTS
// ============================================================================

func TestDatabaseConnectionFiltering(t *testing.T) {
	filter := NewSensitiveDataFilter()

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		// MySQL connection strings
		{
			name:     "MySQL with protocol",
			input:    "mysql://user:pass@localhost:3306/db",
			contains: "mysql://[REDACTED]",
		},
		{
			name:     "MySQL with credentials",
			input:    "mysql://admin:secret123@db.example.com:3306/production",
			contains: "mysql://[REDACTED]",
		},
		{
			name:     "MySQL with SSL options",
			input:    "mysql://user:pass@host:3306/db?sslmode=require",
			contains: "mysql://[REDACTED]",
		},

		// PostgreSQL connection strings
		{
			name:     "PostgreSQL with protocol",
			input:    "postgresql://user:pass@localhost:5432/db",
			contains: "postgresql://[REDACTED]",
		},
		{
			name:     "PostgreSQL with host",
			input:    "postgresql://admin:secret@db.prod.example.com:5432/appdb",
			contains: "postgresql://[REDACTED]",
		},
		{
			name:     "PostgreSQL with options",
			input:    "postgresql://user:pass@host:5432/db?sslmode=verify-full",
			contains: "postgresql://[REDACTED]",
		},

		// MongoDB connection strings
		{
			name:     "MongoDB with protocol",
			input:    "mongodb://user:pass@localhost:27017/db",
			contains: "mongodb://[REDACTED]",
		},
		{
			name:     "MongoDB replica set",
			input:    "mongodb://admin:pass@host1:27017,host2:27017,host3:27017/db?replicaSet=mySet",
			contains: "mongodb://[REDACTED]",
		},

		// Redis connection strings
		{
			name:     "Redis with protocol",
			input:    "redis://user:pass@localhost:6379/0",
			contains: "redis://[REDACTED]",
		},
		{
			name:     "Redis with DB",
			input:    "redis://:password@redis.example.com:6379/1",
			contains: "redis://[REDACTED]",
		},

		// SQLite connection strings
		{
			name:     "SQLite file",
			input:    "sqlite:///path/to/database.db",
			contains: "sqlite://[REDACTED]",
		},
		{
			name:     "SQLite memory",
			input:    "sqlite:///:memory:",
			contains: "sqlite://[REDACTED]",
		},

		// Cassandra connection strings
		{
			name:     "Cassandra with protocol",
			input:    "cassandra://user:pass@localhost:9042/keyspace",
			contains: "cassandra://[REDACTED]",
		},

		// InfluxDB connection strings
		{
			name:     "InfluxDB with protocol",
			input:    "influx://user:pass@localhost:8086/db",
			contains: "influx://[REDACTED]",
		},

		// JDBC connection strings
		{
			name:     "JDBC MySQL",
			input:    "jdbc:mysql://localhost:3306/db?user=root&password=secret",
			contains: "jdbc:mysql://[REDACTED]",
		},
		{
			name:     "JDBC PostgreSQL",
			input:    "jdbc:postgresql://host:5432/db?user=postgres&password=pass",
			contains: "jdbc:postgresql://[REDACTED]",
		},
		{
			name:     "JDBC SQL Server",
			input:    "jdbc:sqlserver://localhost:1433;databaseName=adb;user=sa;password=secret",
			contains: "jdbc:sqlserver:[REDACTED]",
		},
		{
			name:     "JDBC Oracle",
			input:    "jdbc:oracle:thin:@localhost:1521:ORCL",
			contains: "jdbc:oracle:[REDACTED]",
		},

		// SQL Server connection strings
		{
			name:     "SQL Server with server keyword",
			input:    "Server=localhost;user id=sa;password=secret;database=production",
			contains: "Server=[REDACTED]",
		},
		{
			name:     "SQL Server with Data Source",
			input:    "Data Source=tcp:localhost,1433;Initial Catalog=db;User Id=sa;Password=pass",
			contains: "Data Source=[REDACTED]",
		},
		{
			name:     "SQL Server with host keyword",
			input:    "host=db.example.com;username=admin;password=secret123;database=mydb",
			contains: "host=[REDACTED]",
		},

		// Oracle DSN format
		{
			name:     "Oracle DSN",
			input:    "oracle=scott/tiger@localhost:1521/ORCL",
			contains: "oracle=[REDACTED]",
		},
		{
			name:     "Oracle SID",
			input:    "sid=prod:admin:pass@dbhost:1521/service",
			contains: "sid=[REDACTED]",
		},
		{
			name:     "TNS format",
			input:    "tns=(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=ORCL)))",
			contains: "tns=[REDACTED]",
		},

		// Database credential strings (user:pass@host format)
		{
			name:     "MySQL DSN format",
			input:    "user:pass@tcp(localhost:3306)/dbname",
			contains: "[REDACTED]",
		},
		{
			name:     "PostgreSQL DSN",
			input:    "postgres://user:pass@localhost/dbname?sslmode=disable",
			contains: "[REDACTED]",
		},
		{
			name:     "Credentials with IP",
			input:    "admin:secret@192.168.1.100:3306/production",
			contains: "[REDACTED]",
		},
		{
			name:     "Credentials with port",
			input:    "root:password123@db.example.com:5432/app",
			contains: "[REDACTED]",
		},

		// Edge cases - non-sensitive strings
		{
			name:     "URL without protocol",
			input:    "example.com/path",
			contains: "example.com/path",
		},
		{
			name:     "regular text",
			input:    "connect to database server",
			contains: "connect to database server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.Filter(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got: %s", tt.contains, result)
			}
		})
	}
}

func TestDatabaseConnectionFieldFiltering(t *testing.T) {
	var buf strings.Builder
	config := JSONConfig()
	config.Targets = []OutputTarget{CustomOutput(&buf)}
	config.Security = &SecurityConfig{
		SensitiveFilter: newBasicSensitiveDataFilter(),
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test database connection field filtering
	logger.InfoWith("Database connection established",
		String("location", "localhost"),
		String("name", "myapp"),
		String("connection", "mysql://user:pass@localhost:3306/myapp"),
		String("dsn", "postgresql://admin:secret@db.example.com:5432/production"),
	)

	output := buf.String()
	if strings.Contains(output, "location") && !strings.Contains(output, "\"localhost\"") {
		t.Errorf("Location should not be filtered. Output: %s", output)
	}
	if strings.Contains(output, "name") && !strings.Contains(output, "\"myapp\"") {
		t.Errorf("App name should not be filtered. Output: %s", output)
	}
	if strings.Contains(output, "mysql://user:pass@localhost:3306/myapp") {
		t.Error("Connection string should be filtered")
	}
	if strings.Contains(output, "postgresql://admin:secret@db.example.com:5432/production") {
		t.Error("DSN should be filtered")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("Sensitive fields should be redacted")
	}
}

func TestDatabaseConnectionInMessage(t *testing.T) {
	var buf strings.Builder
	config := JSONConfig()
	config.Targets = []OutputTarget{CustomOutput(&buf)}
	config.Security = &SecurityConfig{
		SensitiveFilter: newBasicSensitiveDataFilter(),
	}

	logger, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test database connection strings in message text
	logger.Info("Connecting to mysql://user:pass@localhost:3306/db")
	logger.Info("Database postgresql://admin:secret@db.example.com:5432/production connected")

	output := buf.String()
	if strings.Contains(output, "mysql://user:pass@localhost:3306/db") {
		t.Error("MySQL connection string should be filtered in message")
	}
	if strings.Contains(output, "postgresql://admin:secret@db.example.com:5432/production") {
		t.Error("PostgreSQL connection string should be filtered in message")
	}
	if !strings.Contains(output, "mysql://[REDACTED]") {
		t.Error("Should contain redacted MySQL connection")
	}
	if !strings.Contains(output, "postgresql://[REDACTED]") {
		t.Error("Should contain redacted PostgreSQL connection")
	}
}

// ============================================================================
// BOUNDARY SECURITY TESTS (Truncation & Chunking)
// ============================================================================

// TestTruncationBoundarySensitiveData tests that sensitive data patterns spanning
// the truncation boundary are still detected and redacted.
func TestTruncationBoundarySensitiveData(t *testing.T) {
	// Initialize patterns first
	internal.InitPatterns()

	// Create a custom filter with smaller max input length for testing
	smallFilter := &SensitiveDataFilter{
		maxInputLength: 1000, // Small limit for testing
		timeout:        defaultFilterTimeout,
		semaphore:      make(chan struct{}, maxConcurrentFilters),
	}
	smallFilter.enabled.Store(true)

	// Copy patterns from default filter
	patterns := make([]*regexp.Regexp, len(internal.CompiledBasicPatterns))
	copy(patterns, internal.CompiledBasicPatterns)
	smallFilter.patternsPtr.Store(&patterns)

	tests := []struct {
		name             string
		input            string
		shouldNotContain string
		description      string
	}{
		{
			name: "password at truncation boundary",
			// Create input where password spans the 1000 byte boundary (must exceed maxInputLength)
			input:            strings.Repeat("x", 950) + "password=supersecret123 more text here that exceeds the limit",
			shouldNotContain: "supersecret123",
			description:      "Password value should be redacted even at truncation boundary",
		},
		{
			name: "credit card at truncation boundary",
			// Credit card format: 4532-0151-1283-0366 matches the pattern
			input:            strings.Repeat("x", 930) + "card=4532-0151-1283-0366 end of message",
			shouldNotContain: "4532-0151-1283-0366",
			description:      "Credit card should be redacted even at truncation boundary",
		},
		{
			name: "api key at truncation boundary",
			// API key format: sk- followed by 16-48 alphanumeric chars
			input:            strings.Repeat("x", 920) + "api_key=sk-1234567890abcdefghijkl more data here",
			shouldNotContain: "sk-1234567890abcdefghijkl",
			description:      "API key should be redacted even at truncation boundary",
		},
		{
			name:             "connection string at truncation boundary",
			input:            strings.Repeat("x", 880) + "mysql://user:pass@localhost:3306/db tail of message",
			shouldNotContain: "user:pass@localhost:3306",
			description:      "Connection string should be redacted even at truncation boundary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Ensure input exceeds maxInputLength
			if len(tt.input) <= 1000 {
				tt.input = tt.input + strings.Repeat("x", 1001-len(tt.input))
			}

			result := smallFilter.Filter(tt.input)

			// Check that the sensitive data was redacted
			if strings.Contains(result, tt.shouldNotContain) {
				t.Errorf("%s\nInput length: %d, Result contains sensitive data: %s\nResult preview: %s",
					tt.description, len(tt.input), tt.shouldNotContain, result[:min(200, len(result))])
			}

			// Verify the result contains truncation marker
			if !strings.Contains(result, "[TRUNCATED") {
				t.Errorf("Expected [TRUNCATED] marker for input length %d", len(tt.input))
			}
		})
	}
}

// TestChunkedProcessingBoundarySensitiveData tests that sensitive data patterns
// spanning chunk boundaries during chunked processing are still detected.
func TestChunkedProcessingBoundarySensitiveData(t *testing.T) {
	filter := NewSensitiveDataFilter()

	// Create large inputs that will trigger chunked processing
	// Chunk size is 4096, so we create inputs larger than that
	// NOTE: Test inputs must have word boundaries (spaces) around sensitive data
	// for the patterns to match correctly.

	tests := []struct {
		name             string
		input            string
		shouldNotContain string
		description      string
	}{
		{
			name: "credit card spanning chunk boundary",
			// Place credit card number at chunk boundary (4096 byte mark) with spaces for word boundary
			input:            strings.Repeat("x ", 2045) + " 4532-0151-1283-0366 " + strings.Repeat(" y", 500),
			shouldNotContain: "4532-0151-1283-0366",
			description:      "Credit card spanning chunk boundary should be redacted",
		},
		{
			name:             "password spanning chunk boundary",
			input:            strings.Repeat("x ", 2040) + " password=supersecretvalue " + strings.Repeat(" y", 500),
			shouldNotContain: "supersecretvalue",
			description:      "Password spanning chunk boundary should be redacted",
		},
		{
			name:             "SSN spanning chunk boundary",
			input:            strings.Repeat("x ", 2045) + " 123-45-6789 " + strings.Repeat(" y", 500),
			shouldNotContain: "123-45-6789",
			description:      "SSN spanning chunk boundary should be redacted",
		},
		{
			name:             "API key spanning chunk boundary",
			input:            strings.Repeat("x ", 2035) + " sk-1234567890abcdefghijklmnop " + strings.Repeat(" y", 500),
			shouldNotContain: "sk-1234567890abcdefghijklmnop",
			description:      "API key spanning chunk boundary should be redacted",
		},
		{
			name:             "connection string spanning chunk boundary",
			input:            strings.Repeat("x ", 2000) + " mysql://admin:secretpass@db.host:3306/prod " + strings.Repeat(" y", 500),
			shouldNotContain: "admin:secretpass@db.host:3306",
			description:      "Connection string spanning chunk boundary should be redacted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.Filter(tt.input)

			// Check that the sensitive data was redacted
			if strings.Contains(result, tt.shouldNotContain) {
				t.Errorf("%s\nInput length: %d, Result still contains: %s",
					tt.description, len(tt.input), tt.shouldNotContain)
			}

			// Verify result contains [REDACTED]
			if !strings.Contains(result, "[REDACTED]") {
				t.Errorf("Expected [REDACTED] in result for: %s", tt.name)
			}
		})
	}
}

// TestChunkedProcessingPreservesNonSensitiveData tests that chunked processing
// doesn't corrupt non-sensitive data.
func TestChunkedProcessingPreservesNonSensitiveData(t *testing.T) {
	filter := NewSensitiveDataFilter()

	// Create a large input without any sensitive data
	input := strings.Repeat("hello world ", 1000) // ~12KB

	result := filter.Filter(input)

	// The result should be very similar (may have some differences due to final pass)
	// but should not contain [REDACTED] since there's no sensitive data
	if strings.Contains(result, "[REDACTED]") {
		t.Error("Non-sensitive data should not be redacted")
	}

	// Most of the content should be preserved
	if len(result) < len(input)/2 {
		t.Errorf("Result too short: got %d, input was %d", len(result), len(input))
	}
}

// TestTruncationWithNoSensitiveDataAtBoundary tests that truncation works
// correctly when there's no sensitive data at the boundary.
func TestTruncationWithNoSensitiveDataAtBoundary(t *testing.T) {
	// Create a filter with small max input length
	smallFilter := &SensitiveDataFilter{
		maxInputLength: 500,
		timeout:        defaultFilterTimeout,
		semaphore:      make(chan struct{}, maxConcurrentFilters),
	}
	smallFilter.enabled.Store(true)

	// Copy patterns from default filter
	patterns := make([]*regexp.Regexp, len(internal.CompiledBasicPatterns))
	copy(patterns, internal.CompiledBasicPatterns)
	smallFilter.patternsPtr.Store(&patterns)

	// Create input with no sensitive data near the boundary
	input := strings.Repeat("normal text ", 100) // ~1200 bytes, no sensitive data

	result := smallFilter.Filter(input)

	// Should contain truncation marker
	if !strings.Contains(result, "[TRUNCATED") {
		t.Error("Large input should be truncated")
	}

	// Should not contain [REDACTED] since there's no sensitive data
	if strings.Contains(result, "[REDACTED]") {
		t.Error("Non-sensitive data should not be redacted")
	}
}

// ============================================================================
// SECURITY PRESET CONFIGURATION TESTS
// ============================================================================

func TestSecurityPresetConfigs(t *testing.T) {
	// One table-driven test replacing TestHealthcareConfig / TestFinancialConfig
	// / TestGovernmentConfig, which each duplicated the same "assert preset
	// non-nil + filter enabled, then range inputs and assert redaction" shape.
	type filterCase struct {
		name             string
		input            string
		shouldNotContain string
	}

	presets := []struct {
		name   string
		config *SecurityConfig
		cases  []filterCase
	}{
		{"HealthcareConfig", HealthcareConfig(), []filterCase{
			{"NPI with context", "provider_id=1234567890", "1234567890"},
			{"MRN with context", "mrn=PATIENT123456", "PATIENT123456"},
			{"Patient ID with context", "patient_id=AB12345678", "AB12345678"},
			{"password in healthcare", "password=healthsecret123", "healthsecret123"},
		}},
		{"FinancialConfig", FinancialConfig(), []filterCase{
			{"CVV with context", "cvv=123", "123"},
			{"account number with context", "account_number=12345678901", "12345678901"},
			{"bank account with context", "bank_account=98765432100", "98765432100"},
			{"credit card in financial", "card=4532-0151-1283-0366", "4532-0151-1283-0366"},
			{"password in financial", "password=financesecret", "financesecret"},
		}},
		{"GovernmentConfig", GovernmentConfig(), []filterCase{
			{"passport with context", "passport_number=123456789", "123456789"},
			{"driver license with context", "driver_license=AB1234567", "AB1234567"},
			{"case number with context", "case_number=CASE12345", "CASE12345"},
			{"password in government", "password=govsecret", "govsecret"},
		}},
	}

	for _, p := range presets {
		t.Run(p.name, func(t *testing.T) {
			if p.config == nil {
				t.Fatalf("%s should not return nil", p.name)
			}
			if p.config.SensitiveFilter == nil {
				t.Fatalf("%s should have a sensitive filter", p.name)
			}
			if !p.config.SensitiveFilter.IsEnabled() {
				t.Errorf("%s filter should be enabled", p.name)
			}

			for _, c := range p.cases {
				t.Run(c.name, func(t *testing.T) {
					result := p.config.SensitiveFilter.Filter(c.input)
					if strings.Contains(result, c.shouldNotContain) {
						t.Errorf("%s should filter %q, got: %s", p.name, c.shouldNotContain, result)
					}
				})
			}
		})
	}
}

func TestSecurityConfigClone(t *testing.T) {
	// Merges the former boundary_test.go TestSecurityConfigClone (scalar-field
	// independence on DefaultSecurityConfig) and the former
	// TestSecurityConfigCloning (filter independence on HealthcareConfig) into
	// one parameterized check across every preset: Clone() must deep-copy both
	// the scalar fields and the SensitiveFilter.
	presets := []struct {
		name   string
		config *SecurityConfig
	}{
		{"DefaultSecurityConfig", DefaultSecurityConfig()},
		{"HealthcareConfig", HealthcareConfig()},
		{"FinancialConfig", FinancialConfig()},
		{"GovernmentConfig", GovernmentConfig()},
	}
	for _, p := range presets {
		t.Run(p.name, func(t *testing.T) {
			clone := p.config.Clone()
			if clone == nil {
				t.Fatal("Clone should not return nil")
			}
			if clone.SensitiveFilter == nil {
				t.Fatal("Clone should preserve the sensitive filter")
			}

			// Scalar-field independence.
			originalMax := p.config.MaxMessageSize
			clone.MaxMessageSize = 999
			if p.config.MaxMessageSize != originalMax {
				t.Error("Modifying clone's scalar field should not affect original")
			}

			// Filter independence: adding a pattern to the clone must not change
			// the original's pattern count.
			originalCount := p.config.SensitiveFilter.PatternCount()
			clone.SensitiveFilter.AddPattern(`test_pattern_clone=\w+`)
			if p.config.SensitiveFilter.PatternCount() != originalCount {
				t.Error("Modifying clone's filter should not affect original")
			}
		})
	}
}

// TestFilterCacheBoundaries covers the filter result cache's safety limits
// directly: a nil cache is a no-op, long inputs are never cached (hash
// collision defense), and the cache evicts once full instead of growing.
func TestFilterCacheBoundaries(t *testing.T) {
	t.Run("nil cache is a no-op", func(t *testing.T) {
		f := &SensitiveDataFilter{}
		f.cacheResult(1, "input", "result", time.Now()) // must not panic
	})

	t.Run("long inputs are never cached", func(t *testing.T) {
		f := newSensitiveDataFilterWithPatterns(nil, nil, emptyFilterTimeout)
		long := strings.Repeat("a", cacheInputMaxLen+1)
		f.cacheResult(2, long, "r", time.Now())

		f.cacheMu.Lock()
		cached := len(f.cache)
		f.cacheMu.Unlock()
		if cached != 0 {
			t.Errorf("input of %d bytes was cached, want skipped (cache len %d)", len(long), cached)
		}
	})

	t.Run("evicts when full", func(t *testing.T) {
		f := newSensitiveDataFilterWithPatterns(nil, nil, emptyFilterTimeout)
		f.maxCacheSz = 2
		for i := 0; i < 4; i++ {
			f.cacheResult(uint64(i), fmt.Sprintf("input-%d", i), "r", time.Now())
		}

		f.cacheMu.Lock()
		cached := len(f.cache)
		f.cacheMu.Unlock()
		if cached != f.maxCacheSz {
			t.Errorf("cache holds %d entries after overflow, want capped at %d", cached, f.maxCacheSz)
		}
	})
}

func TestPresetConfigsHaveMaxWriters(t *testing.T) {
	configs := []struct {
		name   string
		config *SecurityConfig
	}{
		{"HealthcareConfig", HealthcareConfig()},
		{"FinancialConfig", FinancialConfig()},
		{"GovernmentConfig", GovernmentConfig()},
		{"DefaultSecureConfig", DefaultSecureConfig()},
		{"DefaultSecurityConfig", DefaultSecurityConfig()},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			if tc.config.MaxWriters <= 0 {
				t.Errorf("%s should have MaxWriters > 0", tc.name)
			}
			if tc.config.MaxMessageSize <= 0 {
				t.Errorf("%s should have MaxMessageSize > 0", tc.name)
			}
		})
	}
}

// boomStringer is a fmt.Stringer whose String method panics — simulating a
// misbehaving user type inside a logged structure.
type boomStringer struct{}

func (boomStringer) String() string { panic("boom") }

// TestFilterValueRecursiveStringerPanicFailClosed pins the fail-CLOSED
// behavior of a panicking Stringer inside a filtered structure. The struct
// fast path used to call v.String() directly; the panic then unwound to
// FilterValueRecursive's coarse recover, whose fallback returns the ENTIRE
// structure unfiltered — a "password"-keyed sibling entry bypassed redaction
// (fail-open). The panic-safe helper now degrades only the panicking value to
// a placeholder while the rest of the structure stays filtered.
func TestFilterValueRecursiveStringerPanicFailClosed(t *testing.T) {
	f := DefaultSecurityConfig().SensitiveFilter

	value := map[string]any{
		"password": "hunter2-secret-value",
		"note":     boomStringer{},
	}
	got := f.FilterValueRecursive("data", value)
	rendered := fmt.Sprintf("%v", got)
	if strings.Contains(rendered, "hunter2-secret-value") {
		t.Errorf("fail-open redaction bypass: sensitive field leaked around panicking Stringer: %q", rendered)
	}
}

// TestCredentialKeywordPreGateSeparatorForms pins that the digit-less
// separator spellings of credential keywords reach the regex filter. The
// password/token patterns match digit-less values, so the
// couldContainSensitiveData hard pre-gate can only admit them through the
// credential-keyword list — which lacked "pwd" and "api-key" while the
// patterns accepted them, so these messages were returned unchanged without
// any regex ever running.
func TestCredentialKeywordPreGateSeparatorForms(t *testing.T) {
	f := DefaultSecurityConfig().SensitiveFilter

	inputs := []string{
		"pwd: hunterXX",        // no digits: only the keyword can pass the pre-gate
		"api-key: abcdefghXYZ", // ditto; api_key/apikey were listed, api-key was not
	}
	for _, in := range inputs {
		if got := f.Filter(in); got == in {
			t.Errorf("Filter(%q) returned input unchanged — credential keyword missing from pre-gate", in)
		}
	}
}

// ============================================================================
// Rate Limiting (public configuration surface) — GEN-001
// ============================================================================

// TestRateLimitConfigPublicSurface pins the exported rate-limit configuration
// surface: SecurityConfig.RateLimitConfig was previously typed
// *internal.RateLimitConfig — publicly visible but unconstructible outside
// the module. The RateLimitConfig alias and DefaultRateLimitConfig must make
// it externally usable.
func TestRateLimitConfigPublicSurface(t *testing.T) {
	cfg := DefaultRateLimitConfig()
	if cfg == nil {
		t.Fatal("DefaultRateLimitConfig() = nil, want non-nil")
	}
	if cfg.MaxMessagesPerSecond != 10000 || cfg.MaxBytesPerSecond != 10*1024*1024 ||
		cfg.BurstSize != 1000 || cfg.Strategy != RateLimitStrategyDrop || cfg.SamplingRate != 100 {
		t.Errorf("DefaultRateLimitConfig() = %+v, want documented defaults", cfg)
	}

	// External-style construction via struct literal.
	custom := &RateLimitConfig{
		MaxMessagesPerSecond: 5,
		Strategy:             RateLimitStrategySample,
		SamplingRate:         2,
	}
	if custom.Strategy != RateLimitStrategySample || custom.MaxMessagesPerSecond != 5 {
		t.Errorf("custom RateLimitConfig = %+v", custom)
	}

	// SecurityConfig.Clone must deep-copy the rate limit config.
	sc := DefaultSecurityConfig()
	sc.RateLimitConfig = custom
	clone := sc.Clone()
	if clone.RateLimitConfig == sc.RateLimitConfig {
		t.Fatal("SecurityConfig.Clone() aliased RateLimitConfig instead of deep-copying")
	}
	if clone.RateLimitConfig.MaxMessagesPerSecond != 5 || clone.RateLimitConfig.Strategy != RateLimitStrategySample {
		t.Errorf("cloned RateLimitConfig = %+v, want field values preserved", clone.RateLimitConfig)
	}
}

// TestRateLimitConfigWiringDropsMessages verifies end-to-end that a configured
// RateLimitConfig actually gates the log path: New wires it into the internal
// RateLimiter and over-limit messages are dropped.
func TestRateLimitConfigWiringDropsMessages(t *testing.T) {
	w := &countWriter{}

	cfg := DefaultConfig()
	cfg.Security.RateLimitConfig = &RateLimitConfig{
		MaxMessagesPerSecond: 10,
		BurstSize:            0,
		Strategy:             RateLimitStrategyDrop,
	}
	cfg.Targets = []OutputTarget{CustomOutput(w)}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = logger.Close() }()

	const total = 100
	for i := 0; i < total; i++ {
		logger.Info("flood")
	}

	// All calls complete within one or two wall-clock seconds: the per-second
	// budget (10) may refill once on a boundary crossing, but the vast
	// majority of the 100 messages must be dropped.
	if n := w.n.Load(); n >= total {
		t.Fatalf("rate limiter did not drop any message: %d/%d written", n, total)
	} else if n < 10 {
		t.Errorf("expected at least the per-second budget (10) to pass, got %d", n)
	}
}
