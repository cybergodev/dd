package internal

import (
	"bytes"
	"sync"
)

// HexChars is the shared hex-digit lookup table used by JSON string escaping
// (internal/json.go) and control-character sanitization (SanitizeControlChars).
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
	for i := range msgLen {
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

	// Slow path: single-pass replacement of control characters and removal of ANSI sequences.
	// Uses a pooled buffer that grows as needed, eliminating the two-pass approach.
	resultPtr := sanitizeBufferPool.Get().(*[]byte)
	result := (*resultPtr)[:0]

	for i := 0; i < msgLen; i++ {
		b := message[i]
		switch {
		case b == 0x00:
			// Null bytes are removed entirely for security (prevent log truncation)
			continue
		case b == 127:
			// DEL character is removed
			continue
		case b == 0x1b:
			// ESC character - skip the entire ANSI escape sequence
			i += skipAnsiSequenceString(message, i+1)
			continue
		case b == 0xEF && i+2 < msgLen:
			// Check for BOM: EF BB BF (U+FEFF)
			if message[i+1] == 0xBB && message[i+2] == 0xBF {
				i += 2
				continue
			}
			// Non-BOM 0xEF sequence: keep all 3 bytes as a unit
			// Consistent with 0xE2 handling and correct for invalid UTF-8
			result = append(result, b, message[i+1], message[i+2])
			i += 2
		case b == 0xE2 && i+2 < msgLen:
			// Check for Unicode control characters
			if isUnicodeControlSequence(message[i+1], message[i+2]) {
				i += 2
				continue
			}
			result = append(result, b, message[i+1], message[i+2])
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

// skipAnsiSequenceString skips an ANSI escape sequence in a string starting after the ESC character.
// It avoids the O(n) []byte(string) allocation by operating directly on the string.
func skipAnsiSequenceString(data string, start int) int {
	if start >= len(data) {
		return 0
	}

	const maxSequenceLen = 256

	b := data[start]

	// CSI (Control Sequence Introducer): ESC [
	if b == '[' {
		skip := 1
		for i := start + 1; i < len(data) && skip < maxSequenceLen; i++ {
			c := data[i]
			if c >= 0x40 && c <= 0x7E {
				return skip + 1
			}
			skip++
		}
		return skip
	}

	// OSC/DCS/APC/PM/SOS: terminated by BEL or ST (ESC \)
	if b == ']' || b == 'P' || b == '_' || b == '^' || b == 'X' {
		skip := 1
		for i := start + 1; i < len(data) && skip < maxSequenceLen; i++ {
			c := data[i]
			if c == 0x07 {
				return skip + 1
			}
			if c == 0x1b && i+1 < len(data) && data[i+1] == '\\' {
				return skip + 2
			}
			skip++
		}
		return skip
	}

	// Other single-character sequences
	if (b >= 0x20 && b <= 0x2F) || (b >= 0x30 && b <= 0x3F) || (b >= 0x40 && b <= 0x5F) {
		return 1
	}

	return 1
}
