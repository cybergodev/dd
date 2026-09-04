package internal

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// ErrReservedName is returned when a path uses a Windows reserved device name.
var ErrReservedName = fmt.Errorf("reserved device name")

// ErrAlternateDataStream is returned when a path contains a Windows Alternate Data Stream.
var ErrAlternateDataStream = fmt.Errorf("alternate data stream not allowed")

// windowsReservedNames contains Windows reserved device names that cannot be used as filenames.
// These names are reserved across all versions of Windows and cannot be used even with extensions.
var windowsReservedNames = map[string]bool{
	"CON":  true,
	"PRN":  true,
	"AUX":  true,
	"NUL":  true,
	"COM1": true,
	"COM2": true,
	"COM3": true,
	"COM4": true,
	"COM5": true,
	"COM6": true,
	"COM7": true,
	"COM8": true,
	"COM9": true,
	"LPT1": true,
	"LPT2": true,
	"LPT3": true,
	"LPT4": true,
	"LPT5": true,
	"LPT6": true,
	"LPT7": true,
	"LPT8": true,
	"LPT9": true,
	// Also include common variants
	"CLOCK$":  true,
	"CONFIG$": true,
	"KEYBD$":  true,
	"SCREEN$": true,
}

// detectOverlongUTF8 checks for UTF-8 overlong encoding attacks.
// Overlong encodings use more bytes than necessary to represent a character,
// which can bypass pattern-based security checks.
//
// Examples of overlong encodings:
//   - 0xC0 0xAE represents '.' (should be single byte 0x2E)
//   - 0xC0 0xAF represents '/' (should be single byte 0x2F)
//   - 0xE0 0x80 0xAF represents '/' (should be single byte 0x2F)
//
// Returns true if overlong encoding is detected.
func detectOverlongUTF8(data []byte) bool {
	for i := 0; i < len(data); i++ {
		b := data[i]

		// Check for 2-byte sequence starting with 0xC0 or 0xC1
		// These are ALWAYS overlong encodings since characters 0x00-0x7F
		// should be single-byte, and 0x80-0xBF require continuation bytes
		if (b == 0xC0 || b == 0xC1) && i+1 < len(data) {
			// Check if next byte is a valid continuation byte (0x80-0xBF)
			if data[i+1]&0xC0 == 0x80 {
				return true
			}
		}

		// Check for 3-byte sequence starting with 0xE0
		// A valid 3-byte sequence starting with 0xE0 must have the second byte >= 0xA0
		// If second byte is 0x80-0x9F, it's an overlong encoding
		if b == 0xE0 && i+2 < len(data) {
			second := data[i+1]
			// Check for valid continuation bytes with overlong pattern
			if second >= 0x80 && second <= 0x9F && data[i+2]&0xC0 == 0x80 {
				return true
			}
		}

		// Check for 4-byte sequence starting with 0xF0
		// A valid 4-byte sequence starting with 0xF0 must have the second byte >= 0x90
		// If second byte is 0x80-0x8F, it's an overlong encoding
		if b == 0xF0 && i+3 < len(data) {
			second := data[i+1]
			// Check for valid continuation bytes with overlong pattern
			if second >= 0x80 && second <= 0x8F &&
				data[i+2]&0xC0 == 0x80 && data[i+3]&0xC0 == 0x80 {
				return true
			}
		}
	}
	return false
}

// ValidateAndSecurePath validates a file path and returns a cleaned absolute path.
// It performs security checks to prevent path traversal attacks and other vulnerabilities.
// All error parameters are caller-provided sentinel errors so that errors.Is() matching
// works correctly in the calling package.
func ValidateAndSecurePath(path string, maxPathLength int, emptyFilePathErr, nullByteErr, pathTooLongErr, pathTraversalErr, invalidPathErr, overlongErr error) (string, error) {
	if path == "" {
		return "", emptyFilePathErr
	}

	if strings.Contains(path, "\x00") {
		return "", nullByteErr
	}

	// Check for UTF-8 overlong encoding attacks
	// These can be used to encode path separators and traversal sequences
	// in non-canonical ways to bypass security checks
	if detectOverlongUTF8([]byte(path)) {
		return "", overlongErr
	}

	// Check for Windows Alternate Data Streams (ADS)
	// ADS can be used to hide malicious data: file.log:hidden.exe
	if err := validateNoADS(path); err != nil {
		return "", err
	}

	// Check for Windows reserved device names
	// These names are reserved and cannot be used as filenames
	if err := validateWindowsReservedName(path); err != nil {
		return "", err
	}

	if len(path) > maxPathLength {
		return "", fmt.Errorf("%w (max %d characters)", pathTooLongErr, maxPathLength)
	}

	// Check for path traversal in the ORIGINAL path before any normalization
	// This catches patterns like "../secret" or "logs/../../../etc/passwd"
	// We check both forward and backward slashes for cross-platform safety
	if strings.Contains(path, "..") {
		return "", pathTraversalErr
	}

	// Check for URL-encoded path traversal attempts
	// Attackers may use %2e%2e%2f (../), %2e%2e%5c (..\\), or mixed encodings
	decodedPath, err := url.PathUnescape(path)
	if err != nil {
		return "", fmt.Errorf("%w: invalid URL encoding", invalidPathErr)
	}
	// Check decoded path for traversal (handles double-encoding like %252e%252e)
	if decodedPath != path {
		if strings.Contains(decodedPath, "..") {
			return "", pathTraversalErr
		}
		// Recursively check for double/triple encoding
		fullyDecoded, innerErr := url.PathUnescape(decodedPath)
		for innerErr == nil && fullyDecoded != decodedPath {
			if strings.Contains(fullyDecoded, "..") {
				return "", pathTraversalErr
			}
			decodedPath = fullyDecoded
			fullyDecoded, innerErr = url.PathUnescape(decodedPath)
		}
	}

	// Convert to absolute path first to normalize
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w: %w", invalidPathErr, err)
	}

	// Clean the absolute path
	cleanPath := filepath.Clean(absPath)

	// Verify cleaned path length is still within limits
	if len(cleanPath) > maxPathLength {
		return "", fmt.Errorf("%w (cleaned path: max %d characters)", pathTooLongErr, maxPathLength)
	}

	// Additional Windows-specific checks
	// These checks prevent attacks like:
	// - UNC path injection: \\?\, \\.\, etc.
	// - Drive relative paths: C:..\, D:..\ etc.
	// - Reserved device names: CON, PRN, AUX, NUL, COM1-9, LPT1-9
	cleanPathLower := strings.ToLower(cleanPath)
	if strings.HasPrefix(cleanPathLower, "\\\\?\\") ||
		strings.HasPrefix(cleanPathLower, "\\\\.\\") {
		// Only allow if the original path was already using these prefixes
		// This prevents attackers from injecting UNC paths
		origLower := strings.ToLower(path)
		if !strings.HasPrefix(origLower, "\\\\?\\") &&
			!strings.HasPrefix(origLower, "\\\\.\\") {
			return "", pathTraversalErr
		}
	}

	// Note: Symlink checking is done in OpenFile — an lstat gate before the
	// open plus a SameFile recheck after it — so the check and the open
	// cannot be separated by a TOCTOU (time-of-check-time-of-use) window.
	return cleanPath, nil
}

// validateWindowsReservedName checks if the path uses a Windows reserved device name.
// These names (CON, PRN, AUX, NUL, COM1-9, LPT1-9) are reserved and cannot be used as filenames.
func validateWindowsReservedName(path string) error {
	// Get the base filename without extension
	base := filepath.Base(path)
	base = strings.ToUpper(base)

	// Remove extension if present
	if idx := strings.Index(base, "."); idx > 0 {
		base = base[:idx]
	}

	// Check if the base name is a reserved name
	if windowsReservedNames[base] {
		return ErrReservedName
	}

	return nil
}

// validateNoADS checks for Windows Alternate Data Stream patterns.
// ADS can be used to hide malicious data or bypass security checks.
// Example: file.log:hidden.exe or file.log:$DATA
func validateNoADS(path string) error {
	// Check for colon that indicates ADS (but not after a drive letter)
	// We need to be careful not to flag drive letters like C:
	colonIdx := strings.LastIndex(path, ":")
	if colonIdx <= 0 {
		return nil
	}

	// Check if this is a drive letter (single character followed by colon at position 1 or 2)
	// Examples: C:\path, C:/path, /c:/path (MSYS/Git Bash style)
	if colonIdx == 1 || (colonIdx == 2 && (path[0] == '/' || path[0] == '\\')) {
		// This might be a drive letter, check if followed by path separator
		if colonIdx+1 < len(path) {
			next := path[colonIdx+1]
			if next == '\\' || next == '/' {
				return nil // This is a drive letter, not ADS
			}
		}
	}

	// Check for URL scheme (http://, https://, etc.)
	if colonIdx > 0 && colonIdx+2 < len(path) && path[colonIdx+1] == '/' && path[colonIdx+2] == '/' {
		return nil // This is a URL scheme, not ADS
	}

	// If we get here with a colon, it's likely ADS
	return ErrAlternateDataStream
}

// ValidateTimeFormat validates a time format string.
// Returns nil if the format is valid or empty (empty uses default).
// Returns an error if the format cannot round-trip a time value.
//
// The check uses FIXED probe times rather than time.Now(): render-then-parse
// is date-sensitive for ambiguous layouts — "1_2" renders June 16 as "616"
// (unparseable two-digit month) but September 1 as "9 1" (parseable) — so a
// now()-based check accepted or rejected the same format depending on the
// calendar day, making Config.Validate flaky by date.
func ValidateTimeFormat(format string) error {
	if format == "" {
		return nil // Empty format is valid (will use default)
	}
	for _, probe := range timeFormatProbes {
		rendered := probe.Format(format)
		parsed, err := time.Parse(format, rendered)
		if err != nil {
			return fmt.Errorf("invalid time format %q: %w", format, err)
		}
		if parsed.Format(format) != rendered {
			return fmt.Errorf("invalid time format %q: not round-trippable", format)
		}
	}
	return nil
}

// timeFormatProbes are fixed times chosen so ambiguous layouts fail
// deterministically: a single-digit month with a two-digit day (June 16) and
// two-digit month/day (November 22), each carrying full clock and zone
// components so time-bearing layouts are exercised too.
var timeFormatProbes = []time.Time{
	time.Date(2026, time.June, 16, 15, 4, 5, 0, time.UTC),
	time.Date(2026, time.November, 22, 3, 4, 5, 0, time.UTC),
}

// DetectNullByteInjection checks for null byte injection attacks.
// Null bytes can be used to truncate strings in C-based systems or
// bypass validation checks that only examine data before the null byte.
func DetectNullByteInjection(data []byte) bool {
	for _, b := range data {
		if b == 0x00 {
			return true
		}
	}
	return false
}

// DetectLog4Shell checks for Log4Shell (CVE-2021-44228) and related
// JNDI injection attack patterns. These patterns can trigger remote
// code execution in vulnerable Log4j versions.
//
// Patterns detected:
//   - ${jndi:...} - JNDI lookup
//   - ${${lower:j}ndi:...} - Obfuscated JNDI
//   - ${${::-j}${::-n}${::-d}${::-i}:...} - Character bypass
//   - ${env:...}, ${sys:...} - Environment/system variable lookups
//   - Unicode escape sequences like \u006a for 'j'
func DetectLog4Shell(input string) bool {
	// Fast path: check for ${ which indicates potential Log4j pattern
	if !strings.Contains(input, "${") {
		return false
	}

	// Common Log4Shell patterns - must have closing brace to be valid
	inputLower := strings.ToLower(input)

	// Check for jndi: within ${...} pattern
	if strings.Contains(inputLower, "${jndi:") && strings.Contains(input, "}") {
		return true
	}

	// Check for nested lookups (obfuscation technique)
	if strings.Contains(input, "${${") && strings.Contains(input, "}}") {
		return true
	}

	// Check for env/sys lookups with closing brace
	if (strings.Contains(inputLower, "${env:") || strings.Contains(inputLower, "${sys:")) && strings.Contains(input, "}") {
		return true
	}

	// Check for obfuscation patterns like ${::- or ${lower:
	if strings.Contains(input, "${::-") || strings.Contains(inputLower, "${lower:") || strings.Contains(inputLower, "${upper:") {
		return true
	}

	// Check for Unicode escape sequences that could spell jndi, ldap, rmi, dns
	// Common escape sequences: \u006a (j), \u006e (n), \u0064 (d), \u0069 (i)
	if strings.Contains(inputLower, "\\u006a") || strings.Contains(inputLower, "\\u006e") ||
		strings.Contains(inputLower, "\\u0064") || strings.Contains(inputLower, "\\u0069") ||
		strings.Contains(inputLower, "\\u006c") || strings.Contains(inputLower, "\\u0072") {
		// Check if combined with ${ pattern
		if strings.Contains(input, "${") {
			return true
		}
	}

	// Check for default value patterns that could be used for obfuscation
	// ${::-j} uses default value syntax for character construction
	if strings.Contains(input, "${:") && strings.Contains(input, ":-") {
		return true
	}

	return false
}

// DetectHomographAttack checks for potential homograph attacks.
// Homograph attacks use visually similar characters from different
// Unicode scripts to impersonate legitimate domains or strings.
//
// This is a basic detection that checks for:
//   - Mixed Cyrillic/Latin characters (common spoofing)
//   - Mixed Greek/Latin characters
//   - Confusable characters in suspicious contexts
func DetectHomographAttack(s string) bool {
	if len(s) == 0 {
		return false
	}

	hasLatin := false
	hasCyrillic := false
	hasGreek := false

	for _, r := range s {
		// Basic Latin (A-Z, a-z)
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			hasLatin = true
		}
		// Cyrillic range (U+0400 to U+04FF)
		if r >= 0x0400 && r <= 0x04FF {
			hasCyrillic = true
		}
		// Greek range (U+0370 to U+03FF)
		if r >= 0x0370 && r <= 0x03FF {
			hasGreek = true
		}
	}

	// Mixed script is suspicious
	if (hasLatin && hasCyrillic) || (hasLatin && hasGreek) {
		return true
	}

	return false
}

// ValidateFieldKeyStrict validates a field key against strict naming rules.
// Field keys should:
//   - Not be empty
//   - Not exceed 256 characters
//   - Contain only alphanumeric characters, underscores, hyphens, and dots
//   - Not start with a digit
//   - Not contain path traversal sequences
//   - Not contain null bytes
//   - Not look like Log4Shell patterns
//   - Not contain overlong UTF-8 encodings
//   - Not contain mixed script characters (homograph attacks)
func ValidateFieldKeyStrict(key string) error {
	if key == "" {
		return fmt.Errorf("field key cannot be empty")
	}

	if len(key) > 256 {
		return fmt.Errorf("field key too long: %d characters (max 256)", len(key))
	}

	// Check for null bytes
	if DetectNullByteInjection([]byte(key)) {
		return fmt.Errorf("field key contains null byte")
	}

	// Check for overlong UTF-8 encoding
	if detectOverlongUTF8([]byte(key)) {
		return fmt.Errorf("field key contains overlong UTF-8 encoding")
	}

	// Check for path traversal
	if strings.Contains(key, "..") {
		return fmt.Errorf("field key contains path traversal sequence")
	}

	// Check for Log4Shell patterns
	if DetectLog4Shell(key) {
		return fmt.Errorf("field key contains suspicious pattern")
	}

	// Check for homograph attacks (mixed script characters)
	if DetectHomographAttack(key) {
		return fmt.Errorf("field key contains mixed script characters (potential homograph attack)")
	}

	// Check for valid characters
	for i, r := range key {
		// First character can't be a digit
		if i == 0 && r >= '0' && r <= '9' {
			return fmt.Errorf("field key cannot start with a digit")
		}

		// Only allow alphanumeric, underscore, hyphen, dot
		isLower := r >= 'a' && r <= 'z'
		isUpper := r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		if !isLower && !isUpper && !isDigit && r != '_' && r != '-' && r != '.' {
			return fmt.Errorf("field key contains invalid character: %q", r)
		}
	}

	return nil
}

// ValidateFieldKeyBasic validates a field key against basic naming rules.
// This is less strict than ValidateFieldKeyStrict and allows more characters.
func ValidateFieldKeyBasic(key string) error {
	if key == "" {
		return fmt.Errorf("field key cannot be empty")
	}

	if len(key) > 256 {
		return fmt.Errorf("field key too long: %d characters (max 256)", len(key))
	}

	// Check for null bytes
	if DetectNullByteInjection([]byte(key)) {
		return fmt.Errorf("field key contains null byte")
	}

	// Check for control characters
	for _, r := range key {
		if r < 32 {
			return fmt.Errorf("field key contains control character")
		}
	}

	return nil
}
