package internal

import (
	"bytes"
	"strings"
	"sync"
)

// HexChars is a package-level constant for hex digit conversion.
// Avoids allocation in SanitizeControlChars hot path.
const HexChars = "0123456789abcdef"

// zeroBuffer securely zeroes the contents of a bytes.Buffer.
// Used before returning buffers to sync.Pool to prevent sensitive data retention.
func zeroBuffer(buf *bytes.Buffer) {
	b := buf.Bytes()
	for i := range b {
		b[i] = 0
	}
	buf.Reset()
}

// zeroSlice securely zeroes the contents of a byte slice pointer.
// Used before returning slices to sync.Pool to prevent sensitive data retention.
func zeroSlice(ptr *[]byte) {
	for i := range *ptr {
		(*ptr)[i] = 0
	}
	*ptr = (*ptr)[:0]
}

// sanitizeBufferPool pools byte slices for sanitization to reduce allocations.
// Initial capacity of 256 bytes covers most common messages.
var sanitizeBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 256)
		return &buf
	},
}

// SanitizeControlChars replaces dangerous control characters with visible escape sequences.
// This preserves debugging information while preventing log injection attacks.
//
// Allowed control characters: \t (tab) - passed through as-is
// Newlines and carriage returns: replaced with visible escape sequences (\n → \\n, \r → \\r)
// to prevent CRLF injection attacks while preserving debug information.
// Null bytes (\x00) and DEL (127) are removed entirely for security.
// Other control characters (0x01-0x1F except \t) are replaced with \xNN format.
// ANSI escape sequences (starting with ESC \x1b) are removed entirely for security.
// Unicode control characters (ZWSP, directional markers, BOM) are removed for security.
func SanitizeControlChars(message string) string {
	msgLen := len(message)
	if msgLen == 0 {
		return message
	}

	// Fast path: check if sanitization is needed using string indexing
	// Avoids []byte allocation when no sanitization is needed
	needsSanitization := false
	for i := 0; i < msgLen; i++ {
		b := message[i]
		// 0x1b is ESC character (start of ANSI escape sequences)
		// \n (0x0a) and \r (0x0d) are escaped to prevent CRLF injection
		if b == 0x00 || b == 0x1b || (b < 32 && b != '\t') || b == 127 {
			needsSanitization = true
			break
		}
		// Check for UTF-8 encoded Unicode control characters
		// These start with 0xE2 (for U+2000-U+20FF range) or 0xEF (for U+FEFF)
		if (b == 0xE2 || b == 0xEF) && i+2 < msgLen {
			needsSanitization = true
			break
		}
	}

	if !needsSanitization {
		return message
	}

	// Slow path: replace control characters and remove ANSI sequences
	// Now we need the byte slice for manipulation
	msgBytes := []byte(message)

	// First pass: calculate result size and identify sequences to remove
	resultSize := 0
	for i := 0; i < len(msgBytes); i++ {
		b := msgBytes[i]
		if b == 0x00 || b == 127 {
			// These are removed
			continue
		} else if b == 0x1b {
			// Skip ANSI escape sequence
			i += skipAnsiSequence(msgBytes, i+1)
			continue
		} else if b == 0xEF && i+2 < len(msgBytes) {
			// Check for BOM: EF BB BF (U+FEFF)
			if msgBytes[i+1] == 0xBB && msgBytes[i+2] == 0xBF {
				i += 2
				continue
			}
			resultSize++
		} else if b == 0xE2 && i+2 < len(msgBytes) {
			// Check for Unicode control characters in U+2000-U+20FF range
			// These are encoded as E2 80 XX or E2 82 XX in UTF-8
			if isUnicodeControlSequence(msgBytes[i+1], msgBytes[i+2]) {
				i += 2
				continue
			}
			resultSize += 3
		} else if b == '\n' || b == '\r' {
			// Escape newlines as visible \n and \r to prevent CRLF injection
			resultSize += 2 // \\n or \\r is 2 bytes
		} else if b < 32 && b != '\t' {
			resultSize += 4 // \xNN is 4 bytes
		} else {
			resultSize++
		}
	}

	// Get buffer from pool for result building
	resultPtr := sanitizeBufferPool.Get().(*[]byte)
	result := (*resultPtr)[:0]
	if cap(*resultPtr) < resultSize {
		// Pool buffer too small, allocate new one
		*resultPtr = make([]byte, 0, resultSize)
		result = (*resultPtr)[:0]
	}

	// Second pass: build the result
	for i := 0; i < len(msgBytes); i++ {
		b := msgBytes[i]
		switch {
		case b == 0x00:
			// Null bytes are removed entirely for security (prevent log truncation)
			continue
		case b == 127:
			// DEL character is removed
			continue
		case b == 0x1b:
			// ESC character - skip the entire ANSI escape sequence
			i += skipAnsiSequence(msgBytes, i+1)
			continue
		case b == 0xEF && i+2 < len(msgBytes):
			// Check for BOM: EF BB BF (U+FEFF)
			if msgBytes[i+1] == 0xBB && msgBytes[i+2] == 0xBF {
				i += 2
				continue
			}
			result = append(result, b)
		case b == 0xE2 && i+2 < len(msgBytes):
			// Check for Unicode control characters
			if isUnicodeControlSequence(msgBytes[i+1], msgBytes[i+2]) {
				i += 2
				continue
			}
			result = append(result, b, msgBytes[i+1], msgBytes[i+2])
			i += 2
		case b == '\n':
			// Escape newline as visible \n to prevent CRLF injection
			result = append(result, '\\', 'n')
		case b == '\r':
			// Escape carriage return as visible \r to prevent CRLF injection
			result = append(result, '\\', 'r')
		case b < 32 && b != '\t':
			// Replace other control characters with visible escape sequence \xNN
			result = append(result, '\\', 'x', HexChars[b>>4], HexChars[b&0x0f])
		default:
			result = append(result, b)
		}
	}

	// Convert to string before returning buffer to pool
	resultStr := string(result)

	// SECURITY: Zero buffer contents before returning to pool
	// This prevents sensitive data from remaining in pooled memory
	zeroSlice(resultPtr)
	sanitizeBufferPool.Put(resultPtr)

	return resultStr
}

// isUnicodeControlSequence checks if a UTF-8 sequence is a dangerous Unicode control character.
// The first byte (0xE2) is already checked, this checks bytes 2 and 3.
// Dangerous characters include:
//   - U+200B: Zero Width Space (ZWSP)
//   - U+200C: Zero Width Non-Joiner (ZWNJ)
//   - U+200D: Zero Width Joiner (ZWJ)
//   - U+200E: Left-to-Right Mark (LRM)
//   - U+200F: Right-to-Left Mark (RLM)
//   - U+2028: Line Separator
//   - U+2029: Paragraph Separator
//   - U+202A-E: Bidirectional formatting characters
//   - U+2060: Word Joiner (WJ)
//   - U+2061-64: Invisible operators
//   - U+2066-69: Isolate formatting characters
//   - U+206A-F: Deprecated formatting characters
func isUnicodeControlSequence(b2, b3 byte) bool {
	// E2 80 XX covers U+2000-U+20FF
	if b2 == 0x80 {
		switch b3 {
		case 0x8B: // U+200B Zero Width Space
			return true
		case 0x8C: // U+200C Zero Width Non-Joiner
			return true
		case 0x8D: // U+200D Zero Width Joiner
			return true
		case 0x8E: // U+200E Left-to-Right Mark
			return true
		case 0x8F: // U+200F Right-to-Left Mark
			return true
		case 0xA8: // U+2028 Line Separator (can cause log injection)
			return true
		case 0xA9: // U+2029 Paragraph Separator (can cause log injection)
			return true
		case 0xAA: // U+202A Left-to-Right Embedding
			return true
		case 0xAB: // U+202B Right-to-Left Embedding
			return true
		case 0xAC: // U+202C Pop Directional Formatting
			return true
		case 0xAD: // U+202D Left-to-Right Override
			return true
		case 0xAE: // U+202E Right-to-Left Override
			return true
		}
	}
	// E2 81 XX covers U+2040-U+207F
	if b2 == 0x81 {
		// U+2060-U+206F Word joiner and invisible formatting
		if b3 >= 0xA0 && b3 <= 0xAF {
			return true
		}
	}
	return false
}

// SanitizeUnicodeControlChars removes dangerous Unicode control characters from a string.
// This is a convenience function for cases where only Unicode control characters
// need to be removed without other sanitization.
func SanitizeUnicodeControlChars(s string) string {
	// Check if the string contains any potential Unicode control characters
	// This is a fast pre-check to avoid unnecessary work
	if !strings.ContainsAny(s, "\u0080\u0081\u0082\u0083\u0084\u0085\u0086\u0087\u0088\u0089\u008A\u008B\u008C\u008D\u008E\u008F\u0090\u0091\u0092\u0093\u0094\u0095\u0096\u0097\u0098\u0099\u009A\u009B\u009C\u009D\u009E\u009F\u034F\u115F\u1160\u2065\u200B\u200C\u200D\u200E\u200F\u2028\u2029\u202A\u202B\u202C\u202D\u202E\u2060\u2061\u2062\u2063\u2064\u2066\u2067\u2068\u2069\u206A\u206B\u206C\u206D\u206E\u206F\uFEFF") {
		return s
	}

	// Build the result without the control characters
	var result strings.Builder
	result.Grow(len(s))

	for _, r := range s {
		if !isUnicodeControlRune(r) {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// isUnicodeControlRune checks if a rune is a dangerous Unicode control character.
func isUnicodeControlRune(r rune) bool {
	// C0 Control Characters (U+0000-U+001F) are handled separately in SanitizeControlChars

	// C1 Control Characters (U+0080-U+009F)
	// These are the Latin-1 Supplement control characters
	if r >= 0x0080 && r <= 0x009F {
		return true
	}

	// Additional invisible/formatting characters
	switch r {
	case '\u034F': // Combining Grapheme Joiner (invisible)
		return true
	case '\u115F', '\u1160': // Hangul Jamo fillers (invisible)
		return true
	case '\u2065': // Deleted but may appear in attacks
		return true
	case '\u200B', '\u200C', '\u200D', '\u200E', '\u200F': // Zero-width and directional marks
		return true
	case '\u2028', '\u2029': // Line and paragraph separators
		return true
	case '\u202A', '\u202B', '\u202C', '\u202D', '\u202E': // Bidirectional formatting
		return true
	case '\u2060', '\u2061', '\u2062', '\u2063', '\u2064': // Invisible operators
		return true
	case '\u2066', '\u2067', '\u2068', '\u2069': // Isolate formatting
		return true
	case '\u206A', '\u206B', '\u206C', '\u206D', '\u206E', '\u206F': // Deprecated formatting
		return true
	case '\uFEFF': // BOM / Zero Width No-Break Space
		return true
	default:
		return false
	}
}

// skipAnsiSequence skips an ANSI escape sequence starting after the ESC character.
// Returns the number of bytes to skip (not including the ESC character itself).
// ANSI escape sequences follow these patterns:
//   - CSI: ESC [ ... final byte (0x40-0x7E)
//   - OSC: ESC ] ... BEL or ESC ] ... ST (ESC \)
//   - DCS: ESC P ... ST (Device Control String)
//   - APC: ESC _ ... ST (Application Program Command)
//   - PM: ESC ^ ... ST (Privacy Message)
//   - SOS: ESC X ... ST (Start of String)
//   - Other: ESC followed by a single intermediate/final byte
//
// SECURITY: Includes maximum length limit to prevent malformed sequences
// from causing excessive processing. Max sequence length is 256 bytes.
func skipAnsiSequence(data []byte, start int) int {
	if start >= len(data) {
		return 0
	}

	// SECURITY: Maximum ANSI sequence length to prevent abuse
	const maxSequenceLen = 256

	b := data[start]

	// CSI (Control Sequence Introducer): ESC [
	if b == '[' {
		skip := 1 // for the '['
		for i := start + 1; i < len(data) && skip < maxSequenceLen; i++ {
			c := data[i]
			// Parameter bytes: 0x30-0x3F
			// Intermediate bytes: 0x20-0x2F
			// Final byte: 0x40-0x7E
			if c >= 0x40 && c <= 0x7E {
				return skip + 1 // include the final byte
			}
			skip++
		}
		return skip // reached end of data or max length
	}

	// OSC (Operating System Command): ESC ]
	// DCS (Device Control String): ESC P
	// APC (Application Program Command): ESC _
	// PM (Privacy Message): ESC ^
	// SOS (Start of String): ESC X
	// All these use ST (String Terminator) or BEL to terminate
	if b == ']' || b == 'P' || b == '_' || b == '^' || b == 'X' {
		skip := 1 // for the initial character
		for i := start + 1; i < len(data) && skip < maxSequenceLen; i++ {
			c := data[i]
			if c == 0x07 { // BEL terminates these sequences
				return skip + 1
			}
			if c == 0x1b && i+1 < len(data) && data[i+1] == '\\' {
				// ST (String Terminator): ESC \ terminates these sequences
				return skip + 2
			}
			skip++
		}
		return skip // reached end of data or max length
	}

	// Other single-character sequences (ESC followed by one char)
	// Includes: ESC (, ESC ), ESC *, ESC +, ESC -, ESC ., ESC /, ESC #
	// These are typically 2-byte sequences
	if (b >= 0x20 && b <= 0x2F) || // Intermediate bytes
		(b >= 0x30 && b <= 0x3F) || // Parameter bytes range
		(b >= 0x40 && b <= 0x5F) { // Final byte range for non-CSI
		return 1
	}

	// Unknown sequence, just skip the ESC and the next byte
	return 1
}
