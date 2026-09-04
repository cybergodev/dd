package internal

import (
	"strings"
	"testing"
)

func TestSanitizeControlChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"normal", "hello world", "hello world"},
		{"newline", "hello\nworld", "hello\\nworld"},         // CRLF injection prevention: \n escaped to \\n
		{"tab", "hello\tworld", "hello\tworld"},              // Tab is allowed
		{"carriage return", "hello\rworld", "hello\\rworld"}, // CRLF injection prevention: \r escaped to \\r
		{"null byte", "hello\x00world", "helloworld"},
		{"del char", "hello\x7fworld", "helloworld"},
		{"control char", "hello\x01world", "hello\\x01world"},
		{"multiple control", "\x00\x01\x02", "\\x01\\x02"},
		{"CRLF injection", "info\nERROR: fake log", "info\\nERROR: fake log"},
		{"log forgery attempt", "user input\r\nERROR: system down", "user input\\r\\nERROR: system down"},
		// Truncated multi-byte sequence at end of input: nothing dangerous to
		// strip, the bytes pass through unchanged.
		{"truncated UTF-8 tail", "ab\xe2\x80", "ab\xe2\x80"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeControlChars(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeControlChars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeANSIEscape(t *testing.T) {
	// Exact-output table: the escape sequence is stripped whole - including
	// its payload for OSC/DCS/APC/PM/SOS sequences, which is by design (the
	// payload is attacker-controlled) - while text outside the sequence
	// survives byte-for-byte.
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// CSI (Control Sequence Introducer): ESC [ ... final-byte
		{"CSI color red", "before\x1b[31mcolored\x1b[0mafter", "beforecoloredafter"},
		{"CSI bold", "\x1b[1mbold\x1b[0m", "bold"},
		{"CSI cursor", "\x1b[2J", ""},
		{"CSI complex", "cursor \x1b[?25hshown", "cursor shown"},
		// OSC (Operating System Command): ESC ] ... BEL or ST - payload removed
		{"OSC title BEL", "before\x1b]0;title\x07after", "beforeafter"},
		{"OSC title ST", "before\x1b]0;title\x1b\\after", "beforeafter"},
		{"OSC hyperlink", "\x1b]8;;http://example.com\x1b\\link\x1b]8;;\x1b\\", "link"},
		{"OSC unterminated", "before\x1b]0;unterminated", "before"},
		// DCS (Device Control String)
		{"DCS with ST", "before\x1bP0;0|1234\x1b\\after", "beforeafter"},
		{"DCS with BEL", "before\x1bPdata\x07after", "beforeafter"},
		// APC (Application Program Command)
		{"APC with ST", "before\x1b_Gi=1,a=q\x1b\\after", "beforeafter"},
		{"APC iTerm2", "\x1b]133;A\x1b\\prompt", "prompt"},
		// PM (Privacy Message)
		{"PM with ST", "before\x1b^privacy\x1b\\after", "beforeafter"},
		{"PM with BEL", "before\x1b^message\x07after", "beforeafter"},
		// SOS (Start of String)
		{"SOS with ST", "before\x1bXstring\x1b\\after", "beforeafter"},
		{"SOS with BEL", "before\x1bXalert\x07after", "beforeafter"},
		// Mixed
		{"mixed", "normal\x1b[31mred\x1b[0mnormal", "normalrednormal"},
		{"multiple sequences", "\x1b[31m\x1b]0;title\x07\x1b_APC\x1b\\visible", "visible"},
		// Truncated / degenerate sequences at end of input
		{"truncated CSI at end", "text\x1b[31", "text"},
		{"lone ESC at end", "text\x1b", "text"},
		{"ESC swallows next byte", "a\x1bb", "a"},
		// Sequence-length cap: a parameter run longer than maxSequenceLen is
		// cut at the cap and the overflow bytes are kept as plain text.
		{"CSI longer than cap", "\x1b[" + strings.Repeat("0", 300), strings.Repeat("0", 45)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeControlChars(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeControlChars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// cp returns the UTF-8 encoding of a code point. Unicode-control tests build
// their inputs this way to keep the source ASCII while making the tested
// code point explicit.
func cp(r rune) string { return string(r) }

func TestSanitizeControlChars_Unicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Zero Width characters
		{"ZWSP removed", "hello" + cp(0x200B) + "world", "helloworld"},
		{"ZWNJ removed", "hello" + cp(0x200C) + "world", "helloworld"},
		{"ZWJ removed", "hello" + cp(0x200D) + "world", "helloworld"},
		// Directional marks
		{"LRM removed", "hello" + cp(0x200E) + "world", "helloworld"},
		{"RLM removed", "hello" + cp(0x200F) + "world", "helloworld"},
		// Line/Paragraph separators
		{"Line separator removed", "hello" + cp(0x2028) + "world", "helloworld"},
		{"Paragraph separator removed", "hello" + cp(0x2029) + "world", "helloworld"},
		// Bidirectional formatting
		{"LRE removed", "hello" + cp(0x202A) + "world", "helloworld"},
		{"RLE removed", "hello" + cp(0x202B) + "world", "helloworld"},
		{"PDF removed", "hello" + cp(0x202C) + "world", "helloworld"},
		{"LRO removed", "hello" + cp(0x202D) + "world", "helloworld"},
		{"RLO removed", "hello" + cp(0x202E) + "world", "helloworld"},
		// Word joiner / invisible operators / isolates (U+2060-U+206F)
		{"Word joiner removed", "hello" + cp(0x2060) + "world", "helloworld"},
		{"Invisible operator removed", "hello" + cp(0x2061) + "world", "helloworld"},
		{"Isolate removed", "hello" + cp(0x2066) + "world", "helloworld"},
		// Same UTF-8 lead bytes (E2 81 xx) but outside the dangerous range:
		// normal punctuation must survive.
		{"Fraction slash kept", "1" + cp(0x2044) + "2", "1" + cp(0x2044) + "2"},
		// BOM
		{"BOM removed", cp(0xFEFF) + "hello world", "hello world"},
		{"BOM middle removed", "hello" + cp(0xFEFF) + "world", "helloworld"},
		// Multiple control chars
		{"multiple removed", cp(0x200B) + cp(0x200E) + cp(0x202E) + "hello" + cp(0xFEFF) + "world" + cp(0x200F), "helloworld"},
		// Normal text preserved
		{"normal text", "hello world", "hello world"},
		{"normal unicode", "日本語テスト", "日本語テスト"},
		{"emoji", "😀🎉", "😀🎉"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeControlChars(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeControlChars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestSanitizeCombinedAttack verifies that combining evasion techniques
// (ANSI + bidi marks + BOM + zero-width) is no more effective than any one
// of them: each component is removed and the visible text remains.
func TestSanitizeCombinedAttack(t *testing.T) {
	rlo := cp(0x202E)
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"ANSI + Unicode", "\x1b[31m" + rlo + "fake\x1b[0m", "fake"},
		{"CRLF + Unicode", "info\n" + rlo + "ERROR: fake", "info\\nERROR: fake"},
		{"BOM + ANSI", cp(0xFEFF) + "\x1b]0;title\x07log", "log"},
		{"Multiple zero-width", "a" + cp(0x200B) + "b" + cp(0x200C) + "c" + cp(0x200D) + "d" + cp(0x200E) + "e" + cp(0x200F) + "f", "abcdef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeControlChars(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeControlChars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
