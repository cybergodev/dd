// Field validation for structured logging keys.
package dd

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/cybergodev/dd/internal"
)

// FieldValidationMode determines how field key validation is performed.
type FieldValidationMode int

const (
	// FieldValidationNone disables field key validation (default).
	// All field keys are accepted without any checks.
	FieldValidationNone FieldValidationMode = iota

	// FieldValidationWarn logs a warning for field keys that don't match
	// the configured naming convention, but still accepts them.
	FieldValidationWarn

	// FieldValidationStrict rejects field keys that don't match the configured
	// naming convention by returning an error from the logging method.
	// Note: For performance reasons, validation errors are logged rather than
	// returned from logging methods, as they don't return errors.
	FieldValidationStrict
)

// FieldNamingConvention specifies the expected naming convention for field keys.
type FieldNamingConvention int

const (
	// NamingConventionAny accepts any valid field key (default).
	NamingConventionAny FieldNamingConvention = iota

	// NamingConventionSnakeCase expects field keys in snake_case format.
	// Example: user_id, first_name, created_at
	NamingConventionSnakeCase

	// NamingConventionCamelCase expects field keys in camelCase format.
	// Example: userId, firstName, createdAt
	NamingConventionCamelCase

	// NamingConventionPascalCase expects field keys in PascalCase format.
	// Example: UserId, FirstName, CreatedAt
	NamingConventionPascalCase

	// NamingConventionKebabCase expects field keys in kebab-case format.
	// Example: user-id, first-name, created-at
	NamingConventionKebabCase
)

// String returns the string representation of the validation mode.
func (m FieldValidationMode) String() string {
	switch m {
	case FieldValidationNone:
		return "none"
	case FieldValidationWarn:
		return "warn"
	case FieldValidationStrict:
		return "strict"
	default:
		return "unknown"
	}
}

// String returns the string representation of the naming convention.
func (c FieldNamingConvention) String() string {
	switch c {
	case NamingConventionAny:
		return "any"
	case NamingConventionSnakeCase:
		return "snake_case"
	case NamingConventionCamelCase:
		return "camelCase"
	case NamingConventionPascalCase:
		return "PascalCase"
	case NamingConventionKebabCase:
		return "kebab-case"
	default:
		return "unknown"
	}
}

// FieldValidationConfig configures field key validation.
type FieldValidationConfig struct {
	// Mode determines how validation failures are handled.
	Mode FieldValidationMode

	// Convention specifies the expected naming convention for field keys.
	Convention FieldNamingConvention

	// AllowCommonAbbreviations allows common abbreviations like ID, URL, HTTP
	// even when they don't strictly match the naming convention.
	AllowCommonAbbreviations bool

	// EnableSecurityValidation enables strict security validation including
	// Log4Shell detection, homograph attack detection, and overlong UTF-8 checks.
	// Default: true when Mode is not FieldValidationNone
	EnableSecurityValidation bool
}

// DefaultFieldValidationConfig returns the default field validation configuration
// which disables validation.
func DefaultFieldValidationConfig() *FieldValidationConfig {
	return &FieldValidationConfig{
		Mode:                     FieldValidationNone,
		Convention:               NamingConventionAny,
		AllowCommonAbbreviations: true,
		EnableSecurityValidation: true,
	}
}

// StrictSnakeCaseConfig returns a config for strict snake_case validation.
func StrictSnakeCaseConfig() *FieldValidationConfig {
	return &FieldValidationConfig{
		Mode:                     FieldValidationStrict,
		Convention:               NamingConventionSnakeCase,
		AllowCommonAbbreviations: true,
		EnableSecurityValidation: true,
	}
}

// StrictCamelCaseConfig returns a config for strict camelCase validation.
func StrictCamelCaseConfig() *FieldValidationConfig {
	return &FieldValidationConfig{
		Mode:                     FieldValidationStrict,
		Convention:               NamingConventionCamelCase,
		AllowCommonAbbreviations: true,
		EnableSecurityValidation: true,
	}
}

// ValidateFieldKey validates a field key against the configured naming convention.
// Returns an error describing the validation failure, or nil if valid.
// Security validation is always performed when Mode is not FieldValidationNone.
//
// Returns errors:
//   - Empty key error: when the key is an empty string
//   - Security validation errors: Log4Shell detection, homograph attack, overlong UTF-8
//   - Convention mismatch: when the key doesn't match the configured naming convention
func (c *FieldValidationConfig) ValidateFieldKey(key string) error {
	if c == nil || c.Mode == FieldValidationNone {
		return nil
	}

	if key == "" {
		return fmt.Errorf("field key cannot be empty")
	}

	// Always run security validation first when enabled
	if c.EnableSecurityValidation {
		if err := internal.ValidateFieldKeyStrict(key); err != nil {
			return err
		}
	}

	// Skip naming convention check if Any convention is specified
	if c.Convention == NamingConventionAny {
		return nil
	}

	// Check if it's a common abbreviation
	if c.AllowCommonAbbreviations && isCommonAbbreviation(key) {
		return nil
	}

	switch c.Convention {
	case NamingConventionSnakeCase:
		if !isValidSnakeCase(key) {
			return fmt.Errorf("field key %q does not match snake_case convention", key)
		}
	case NamingConventionCamelCase:
		if !isValidCamelCase(key) {
			return fmt.Errorf("field key %q does not match camelCase convention", key)
		}
	case NamingConventionPascalCase:
		if !isValidPascalCase(key) {
			return fmt.Errorf("field key %q does not match PascalCase convention", key)
		}
	case NamingConventionKebabCase:
		if !isValidKebabCase(key) {
			return fmt.Errorf("field key %q does not match kebab-case convention", key)
		}
	}

	return nil
}

// commonSuffixes contains suffixes that indicate a common abbreviation pattern.
// Pre-computed to avoid allocation on every call to isCommonAbbreviation.
var commonSuffixes = []string{"_id", "_url", "_uri", "_ip", "_api"}

// Common abbreviations that are allowed regardless of naming convention.
// Stored in lowercase; isCommonAbbreviation normalizes the lookup.
var commonAbbreviations = map[string]bool{
	"id": true, "url": true, "uri": true, "http": true, "https": true,
	"api": true, "json": true, "xml": true, "html": true, "sql": true,
	"ip": true, "tcp": true, "udp": true, "ssl": true, "tls": true,
	"jwt": true, "oauth": true,
}

func isCommonAbbreviation(key string) bool {
	// Case-insensitive exact match
	if commonAbbreviations[strings.ToLower(key)] {
		return true
	}

	// Check if key ends with a common abbreviation suffix
	lowerKey := strings.ToLower(key)
	for _, suffix := range commonSuffixes {
		if strings.HasSuffix(lowerKey, suffix) {
			prefix := key[:len(key)-len(suffix)]
			if len(prefix) > 0 && isValidPrefix(prefix) {
				return true
			}
		}
	}

	return false
}

// isValidPrefix checks that the prefix before a suffix is a valid identifier
// (lowercase letters, digits, underscores for snake_case; no consecutive underscores).
func isValidPrefix(s string) bool {
	if len(s) == 0 {
		return false
	}
	hasUnderscore := false
	for i, r := range s {
		if r == '_' {
			if hasUnderscore {
				return false
			}
			hasUnderscore = true
		} else {
			hasUnderscore = false
			if !unicode.IsLower(r) && !unicode.IsDigit(r) {
				return false
			}
		}
		if i == 0 && unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isValidSnakeCase(s string) bool {
	if len(s) == 0 {
		return false
	}

	// Must not start or end with underscore
	if s[0] == '_' || s[len(s)-1] == '_' {
		return false
	}

	// Must not have consecutive underscores
	hasUnderscore := false
	for i, r := range s {
		if r == '_' {
			if hasUnderscore {
				return false // Consecutive underscores
			}
			hasUnderscore = true
		} else {
			hasUnderscore = false
			// Must be lowercase letter or digit
			if !unicode.IsLower(r) && !unicode.IsDigit(r) {
				return false
			}
		}
		// First character must be a letter
		if i == 0 && unicode.IsDigit(r) {
			return false
		}
	}

	return true
}

func isValidCamelCase(s string) bool {
	if len(s) == 0 {
		return false
	}

	// First character must be lowercase letter
	firstRune := rune(s[0])
	if !unicode.IsLower(firstRune) {
		return false
	}

	// Must contain only letters and digits
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}

	return true
}

func isValidPascalCase(s string) bool {
	if len(s) == 0 {
		return false
	}

	// First character must be uppercase letter
	firstRune := rune(s[0])
	if !unicode.IsUpper(firstRune) {
		return false
	}

	// Must contain only letters and digits
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}

	return true
}

func isValidKebabCase(s string) bool {
	if len(s) == 0 {
		return false
	}

	// Must not start or end with hyphen
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}

	// Must not have consecutive hyphens
	hasHyphen := false
	for i, r := range s {
		if r == '-' {
			if hasHyphen {
				return false // Consecutive hyphens
			}
			hasHyphen = true
		} else {
			hasHyphen = false
			// Must be lowercase letter or digit
			if !unicode.IsLower(r) && !unicode.IsDigit(r) {
				return false
			}
		}
		// First character must be a letter
		if i == 0 && unicode.IsDigit(r) {
			return false
		}
	}

	return true
}
