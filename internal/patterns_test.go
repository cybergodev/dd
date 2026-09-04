package internal

import (
	"sync"
	"testing"
)

func TestInitPatterns(t *testing.T) {
	// Reset patterns for testing
	PatternsOnce = sync.Once{}
	CompiledFullPatterns = nil
	CompiledBasicPatterns = nil

	InitPatterns()

	if len(CompiledFullPatterns) == 0 {
		t.Error("InitPatterns should compile full patterns")
	}
	if len(CompiledBasicPatterns) == 0 {
		t.Error("InitPatterns should compile basic patterns")
	}
	if len(CompiledFullPatterns) < len(CompiledBasicPatterns) {
		t.Error("Full patterns should be >= basic patterns")
	}
	// Most patterns must compile: some regex features may legitimately fail,
	// but dropping below half would silently gut the filter.
	if len(CompiledFullPatterns) < len(AllPatterns)/2 {
		t.Errorf("Expected at least half of patterns to compile, got %d of %d",
			len(CompiledFullPatterns), len(AllPatterns))
	}

	// Call again - should be idempotent due to sync.Once
	count := len(CompiledFullPatterns)
	InitPatterns()
	if len(CompiledFullPatterns) != count {
		t.Error("InitPatterns should be idempotent")
	}
}

func TestIsSensitiveKey(t *testing.T) {
	tests := []struct {
		key       string
		sensitive bool
	}{
		// Empty and non-sensitive
		{"", false},
		{"name", false},
		{"id", false},
		{"description", false},
		{"title", false},

		// Exact match sensitive keywords
		{"password", true},
		{"PASSWORD", true},
		{"passwd", true},
		{"pwd", true},
		{"secret", true},
		{"token", true},
		{"bearer", true},
		{"auth", true},
		{"authorization", true},
		{"credential", true},
		{"credentials", true},

		// API keys
		{"api_key", true},
		{"apikey", true},
		{"api-key", true},
		{"access_key", true},
		{"accesskey", true},
		{"client_id", true},
		{"client_secret", true},

		// Secrets
		{"secret_key", true},
		{"private_key", true},
		{"private_key_id", true},

		// Session and tokens
		{"session_id", true},
		{"session_token", true},
		{"refresh_token", true},
		{"access_token", true},
		{"oauth_token", true},

		// PII
		{"credit_card", true},
		{"ssn", true},
		{"social_security", true},

		// Contact info
		{"phone", true},
		{"telephone", true},
		{"mobile", true},

		// Exact match only keywords
		{"db", true},
		{"url", true},
		{"uri", true},
		{"host", true},
		{"conn", true},
		{"dsn", true},

		// Substring matches (compound keys)
		{"user_password", true},
		{"admin_password_hash", true},
		{"api_key_secret", true},
		{"my_token_value", true},
		{"user_secret_data", true},
		{"db_password", true},
		{"redis_auth", true},

		// Case insensitive
		{"Secret_Key", true},
		{"API_KEY", true},
		{"UserToken", true},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := IsSensitiveKey(tt.key)
			if result != tt.sensitive {
				t.Errorf("IsSensitiveKey(%q) = %v, want %v", tt.key, result, tt.sensitive)
			}
		})
	}
}

func TestHasNestedQuantifiers(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		maxRange  int
		dangerous bool
	}{
		// Safe patterns
		{"simple literal", "hello", 10000, false},
		{"simple alternation", "a|b", 10000, false},
		{"simple quantifier", "a+", 10000, false},
		{"simple range", "a{1,10}", 10000, false},
		{"optional quantifier", "a?", 10000, false},
		{"character class", "[a-z]+", 10000, false},
		{"group without repetition", "(abc)", 10000, false},

		// Dangerous patterns - nested quantifiers
		{"nested plus on quantified group", "(a+)+", 10000, true},
		{"nested star on quantified group", "(a*)*", 10000, true},
		{"consecutive quantifiers", "a++", 10000, true},
		{"consecutive quantifiers with question", "a+?", 10000, true},
		{"quantified quantifier", "a{1,2}+", 10000, true},
		{"nested quantifier with alternation", "(a+|b+)+", 10000, true},

		// Open-ended ranges on groups
		{"open range on quantified group", "(a+){1,}", 10000, true},
		{"open range on alternation group", "(a+|b){0,}", 10000, true},

		// Excessive quantifier range
		{"excessive range", "a{1,20000}", 10000, true},
		{"excessive range upper", "a{5000,20000}", 10000, true},

		// Safe patterns with reasonable ranges
		{"reasonable range", "a{1,100}", 10000, false},
		{"reasonable range in group", "(abc){1,50}", 10000, false},

		// Edge cases
		{"empty pattern", "", 10000, false},
		{"single char", "a", 10000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasNestedQuantifiers(tt.pattern, tt.maxRange)
			if result != tt.dangerous {
				t.Errorf("HasNestedQuantifiers(%q) = %v, want %v", tt.pattern, result, tt.dangerous)
			}
		})
	}
}

func TestValidateQuantifierRange(t *testing.T) {
	tests := []struct {
		name     string
		rangeStr string
		maxRange int
		hasError bool
	}{
		// Valid ranges
		{"exact count", "5", 100, false},
		{"range", "1,10", 100, false},
		{"open ended", "5,", 100, false},
		{"at max", "100", 100, false},
		{"zero start", "0,50", 100, false},

		// Invalid ranges
		{"exceeds max", "200", 100, true},
		{"exceeds max in range", "50,200", 100, true},
		{"invalid format", "a,b", 100, true},
		{"empty", "", 100, true},
		{"too many parts", "1,2,3", 100, true},
		{"non-numeric", "abc", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuantifierRange(tt.rangeStr, tt.maxRange)
			if tt.hasError && err == nil {
				t.Errorf("ValidateQuantifierRange(%q) expected error, got nil", tt.rangeStr)
			}
			if !tt.hasError && err != nil {
				t.Errorf("ValidateQuantifierRange(%q) unexpected error: %v", tt.rangeStr, err)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		hasError bool
	}{
		{"0", 0, false},
		{"1", 1, false},
		{"123", 123, false},
		{"  456  ", 456, false}, // with whitespace
		{"", 0, true},
		{"abc", 0, true},
		{"12.5", 0, true},
		{"-10", -10, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseInt(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("ParseInt(%q) expected error, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("ParseInt(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("ParseInt(%q) = %d, want %d", tt.input, result, tt.expected)
				}
			}
		})
	}
}

// TestAllPatternsCompile asserts that every built-in pattern compiles.
// InitPatterns only warns when a pattern fails to compile — the pattern is
// then silently absent from every filter (weakening redaction with no other
// signal), so this test is the hard tripwire for a typo introduced into
// AllPatterns.
func TestAllPatternsCompile(t *testing.T) {
	InitPatterns()
	if len(CompiledFullPatterns) != len(AllPatterns) {
		t.Errorf("len(CompiledFullPatterns) = %d, want %d — built-in patterns failed to compile (see stderr warnings)", len(CompiledFullPatterns), len(AllPatterns))
	}
}
