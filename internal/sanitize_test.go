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
	tests := []struct {
		name             string
		input            string
		shouldNotContain string
	}{
		// CSI (Control Sequence Introducer)
		{"CSI color red", "\x1b[31mhello\x1b[0m", "\x1b[31m"},
		{"CSI bold", "\x1b[1mbold\x1b[0m", "\x1b[1m"},
		{"CSI cursor", "\x1b[2J", "\x1b[2J"},
		{"CSI complex", "\x1b[?25h", "\x1b[?"},
		// OSC (Operating System Command)
		{"OSC title BEL", "\x1b]0;title\x07", "\x1b]0;"},
		{"OSC title ST", "\x1b]0;title\x1b\\", "\x1b]0;"},
		{"OSC hyperlink", "\x1b]8;;http://example.com\x1b\\link\x1b]8;;\x1b\\", "\x1b]8;"},
		// DCS (Device Control String)
		{"DCS with ST", "\x1bP0;0|1234\x1b\\", "\x1bP"},
		{"DCS with BEL", "\x1bPdata\x07", "\x1bP"},
		// APC (Application Program Command)
		{"APC with ST", "\x1b_Gi=1,a=q\x1b\\", "\x1b_"},
		{"APC iTerm2", "\x1b]133;A\x1b\\", "\x1b]133"},
		// PM (Privacy Message)
		{"PM with ST", "\x1b^privacy\x1b\\", "\x1b^"},
		{"PM with BEL", "\x1b^message\x07", "\x1b^"},
		// SOS (Start of String)
		{"SOS with ST", "\x1bXstring\x1b\\", "\x1bX"},
		{"SOS with BEL", "\x1bXalert\x07", "\x1bX"},
		// Mixed
		{"mixed", "normal\x1b[31mred\x1b[0mnormal", "\x1b[31m"},
		{"multiple sequences", "\x1b[31m\x1b]0;title\x07\x1b_APC\x1b\\", "\x1b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeControlChars(tt.input)
			if strings.Contains(result, tt.shouldNotContain) {
				t.Errorf("Result should not contain %q, got: %q", tt.shouldNotContain, result)
			}
		})
	}
}

func TestSanitizeANSIEscapePreservesContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"CSI preserves text", "before\x1b[31mcolored\x1b[0mafter", "beforecoloredafter"},
		{"OSC preserves text", "before\x1b]0;title\x07after", "beforetitleafter"},
		{"DCS preserves text", "before\x1bPdata\x1b\\after", "beforedataafter"},
		{"APC preserves text", "before\x1b_cmd\x1b\\after", "beforecmdafter"},
		{"PM preserves text", "before\x1b^msg\x1b\\after", "beforemsgafter"},
		{"SOS preserves text", "before\x1bXstr\x1b\\after", "beforestrafter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeControlChars(tt.input)
			// The result should contain the expected text (without the escape sequences)
			if !strings.Contains(result, "before") || !strings.Contains(result, "after") {
				t.Errorf("Result should preserve text content, got: %q", result)
			}
		})
	}
}

func TestSanitizeControlChars_Unicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Zero Width characters
		{"ZWSP removed", "hello\u200Bworld", "helloworld"},
		{"ZWNJ removed", "hello\u200Cworld", "helloworld"},
		{"ZWJ removed", "hello\u200Dworld", "helloworld"},
		// Directional marks
		{"LRM removed", "hello\u200Eworld", "helloworld"},
		{"RLM removed", "hello\u200Fworld", "helloworld"},
		// Line/Paragraph separators
		{"Line separator removed", "hello\u2028world", "helloworld"},
		{"Paragraph separator removed", "hello\u2029world", "helloworld"},
		// Bidirectional formatting
		{"LRE removed", "hello\u202Aworld", "helloworld"},
		{"RLE removed", "hello\u202Bworld", "helloworld"},
		{"PDF removed", "hello\u202Cworld", "helloworld"},
		{"LRO removed", "hello\u202Dworld", "helloworld"},
		{"RLO removed", "hello\u202Eworld", "helloworld"},
		// BOM
		{"BOM removed", "\uFEFFhello world", "hello world"},
		{"BOM middle removed", "hello\uFEFFworld", "helloworld"},
		// Multiple control chars
		{"multiple removed", "\u200B\u200E\u202Ehello\uFEFFworld\u200F", "helloworld"},
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

// TestSanitizeCombinedAttack tests combined attack patterns
func TestSanitizeCombinedAttack(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldClean bool
	}{
		{"ANSI + Unicode", "\x1b[31m\u202Efake\x1b[0m", true},
		{"CRLF + Unicode", "info\n\u202EERROR: fake", true},
		{"BOM + ANSI", "\uFEFF\x1b]0;title\x07log", true},
		{"Multiple zero-width", "a\u200Bb\u200Cc\u200Dd\u200Ee\u200Ff", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeControlChars(tt.input)
			// Check that control characters are removed
			if strings.Contains(result, "\x1b") {
				t.Errorf("Result should not contain ANSI escape: %q", result)
			}
			if strings.ContainsRune(result, '\u202E') {
				t.Errorf("Result should not contain RLO: %q", result)
			}
			if strings.ContainsRune(result, '\uFEFF') {
				t.Errorf("Result should not contain BOM: %q", result)
			}
		})
	}
}
