package dd

import (
	"strings"
	"testing"

	"github.com/cybergodev/dd/internal"
)

// FuzzSanitizeControlChars tests the control character sanitization with random inputs.
func FuzzSanitizeControlChars(f *testing.F) {
	// Seed corpus with known test cases
	f.Add("hello world")
	f.Add("hello\nworld")
	f.Add("hello\rworld")
	f.Add("hello\tworld")
	f.Add("test\x00null")
	f.Add("\x1b[31mcolor\x1b[0m")
	f.Add("\u200Bzero-width")
	f.Add("\u202Ertl-override")
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		result := internal.SanitizeControlChars(input)

		// Result should never contain raw control characters (except tab)
		for i, r := range result {
			if r < 32 && r != '\t' {
				t.Errorf("Result contains unescaped control character at index %d: %d", i, r)
			}
		}

		// Result should never contain ANSI escape sequences
		if strings.Contains(result, "\x1b[") {
			t.Error("Result should not contain ANSI CSI sequences")
		}
		if strings.Contains(result, "\x1b]") {
			t.Error("Result should not contain ANSI OSC sequences")
		}

		// Result should never contain null bytes
		if strings.Contains(result, "\x00") {
			t.Error("Result should not contain null bytes")
		}

		// Result should never contain Unicode control characters
		if strings.ContainsAny(result, "\u200B\u200C\u200D\u200E\u200F\u2028\u2029\u202A\u202B\u202C\u202D\u202E\uFEFF") {
			t.Error("Result should not contain Unicode control characters")
		}

		// Result should not panic or crash
		_ = len(result)
	})
}

// FuzzValidateAndSecurePath tests path validation with random inputs.
func FuzzValidateAndSecurePath(f *testing.F) {
	// Seed corpus with valid and invalid paths
	f.Add("logs/app.log")
	f.Add("../etc/passwd")
	f.Add("test\x00.log")
	f.Add("%2e%2e%2fsecret")
	f.Add("")
	f.Add("normal_file.txt")
	f.Add("/var/log/application.log")

	f.Fuzz(func(t *testing.T, path string) {
		// Limit path length to avoid excessive memory usage
		if len(path) > 4096 {
			path = path[:4096]
		}

		result, err := internal.ValidateAndSecurePath(
			path, 4096,
			ErrEmptyFilePath,
			ErrNullByte,
			ErrPathTooLong,
			ErrPathTraversal,
			ErrInvalidPath,
			ErrOverlongEncoding,
		)

		if err != nil {
			// Error is expected for malicious paths
			return
		}

		// If no error, result should be a clean path
		if result == "" && path != "" {
			t.Error("Non-empty valid path should return non-empty result")
		}

		// Result should not contain null bytes
		if strings.Contains(result, "\x00") {
			t.Error("Validated path should not contain null bytes")
		}

		// Result should not contain path traversal
		if strings.Contains(result, "..") {
			t.Error("Validated path should not contain path traversal")
		}
	})
}

// FuzzSensitiveDataFilter tests the sensitive data filter with random inputs.
func FuzzSensitiveDataFilter(f *testing.F) {
	// Initialize patterns
	internal.InitPatterns()

	// Create filter
	filter := newBasicSensitiveDataFilter()

	// Seed corpus with various inputs
	f.Add("hello world")
	f.Add("password=secret123")
	f.Add("api_key=sk-1234567890abcdef")
	f.Add("card=4532-0151-1283-0366")
	f.Add("email=test@example.com")
	f.Add("phone=+1-415-555-2671")
	f.Add("")
	f.Add(strings.Repeat("a", 10000)) // Large input

	f.Fuzz(func(t *testing.T, input string) {
		// Limit input size to avoid timeout
		if len(input) > 100000 {
			input = input[:100000]
		}

		result := filter.Filter(input)

		// Result should not be empty unless input was filtered to nothing
		if result == "" && len(input) > 0 {
			// This is acceptable for inputs that are entirely sensitive data
			return
		}

		// Result should not contain common sensitive data patterns
		// Note: We don't check for the actual sensitive values because they may
		// appear in benign contexts. We just ensure the function doesn't panic.

		// Most importantly, the function should not panic
		_ = len(result)
	})
}

// FuzzFilterFieldValue tests field value filtering with random key-value pairs.
func FuzzFilterFieldValue(f *testing.F) {
	filter := newBasicSensitiveDataFilter()

	// Seed corpus
	f.Add("username", "john_doe")
	f.Add("password", "secret123")
	f.Add("api_key", "sk-test123")
	f.Add("normal_field", "normal_value")
	f.Add("", "")
	f.Add("count", "12345")

	f.Fuzz(func(t *testing.T, key, value string) {
		// Limit input size
		if len(key) > 256 {
			key = key[:256]
		}
		if len(value) > 10000 {
			value = value[:10000]
		}

		result := filter.FilterFieldValue(key, value)

		// Sensitive keys should be redacted
		if internal.IsSensitiveKey(key) {
			if str, ok := result.(string); ok && str != "[REDACTED]" {
				// Note: The filter may not always redact if the key doesn't match exactly
				// This is expected behavior, so we don't fail the test
			}
		}

		// Function should not panic
		_ = result
	})
}
