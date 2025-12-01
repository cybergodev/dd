package dd

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SensitiveDataFilter struct {
	patterns       []*regexp.Regexp
	mu             sync.RWMutex
	maxInputLength int
	timeout        time.Duration
	enabled        atomic.Bool
}

func NewSensitiveDataFilter() *SensitiveDataFilter {
	filter := &SensitiveDataFilter{
		patterns:       make([]*regexp.Regexp, 0, 12),
		maxInputLength: defaultMaxInputLength,
		timeout:        defaultFilterTimeout,
	}
	filter.enabled.Store(true)

	patterns := []string{
		`\b[0-9]{13,19}\b`,
		`\b[0-9]{3}-[0-9]{2}-[0-9]{4}\b`,
		`(?i)(password|passwd|pwd|secret)[\s:=]+[^\s]{1,32}`,
		`(?i)(token|api[_-]?key|bearer)[\s:=]+[^\s]{1,256}`,
		`eyJ[A-Za-z0-9_-]{10,100}\.eyJ[A-Za-z0-9_-]{10,100}\.[A-Za-z0-9_-]{10,100}`,
		`-----BEGIN[^-]*PRIVATE\s+KEY-----[\s\S]*?-----END[^-]*PRIVATE\s+KEY-----`,
		`\bAKIA[0-9A-Z]{16}\b`,
		`\bAIza[A-Za-z0-9_-]{35}\b`,
		`\bsk-[A-Za-z0-9]{20,48}\b`,
		`\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`,
		`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,
		`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`,
		`\b[13][a-km-zA-HJ-NP-Z1-9]{25,34}\b`,
		`(?i)(mysql|postgresql|mongodb)://[^\s]{1,128}`,
	}

	for _, pattern := range patterns {
		_ = filter.addPattern(pattern)
	}

	return filter
}

func NewEmptySensitiveDataFilter() *SensitiveDataFilter {
	filter := &SensitiveDataFilter{
		patterns:       make([]*regexp.Regexp, 0),
		maxInputLength: emptyMaxInputLength,
		timeout:        emptyFilterTimeout,
	}
	filter.enabled.Store(true)
	return filter
}

func NewCustomSensitiveDataFilter(patterns ...string) (*SensitiveDataFilter, error) {
	filter := NewEmptySensitiveDataFilter()

	for _, pattern := range patterns {
		if err := filter.AddPattern(pattern); err != nil {
			return nil, err
		}
	}

	return filter, nil
}

func (f *SensitiveDataFilter) addPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPattern, err)
	}

	f.mu.Lock()
	f.patterns = append(f.patterns, re)
	f.mu.Unlock()

	return nil
}

func (f *SensitiveDataFilter) AddPattern(pattern string) error {
	return f.addPattern(pattern)
}

func (f *SensitiveDataFilter) AddPatterns(patterns ...string) error {
	for _, pattern := range patterns {
		if err := f.addPattern(pattern); err != nil {
			return err
		}
	}
	return nil
}

func (f *SensitiveDataFilter) ClearPatterns() {
	f.mu.Lock()
	f.patterns = make([]*regexp.Regexp, 0)
	f.mu.Unlock()
}

func (f *SensitiveDataFilter) PatternCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.patterns)
}

func (f *SensitiveDataFilter) Enable() {
	if f != nil {
		f.enabled.Store(true)
	}
}

func (f *SensitiveDataFilter) Disable() {
	if f != nil {
		f.enabled.Store(false)
	}
}

func (f *SensitiveDataFilter) IsEnabled() bool {
	if f == nil {
		return false
	}
	return f.enabled.Load()
}

func (f *SensitiveDataFilter) Clone() *SensitiveDataFilter {
	if f == nil {
		return nil
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	clone := &SensitiveDataFilter{
		patterns:       make([]*regexp.Regexp, len(f.patterns)),
		maxInputLength: f.maxInputLength,
		timeout:        f.timeout,
	}
	clone.enabled.Store(f.enabled.Load())
	copy(clone.patterns, f.patterns)

	return clone
}

func (f *SensitiveDataFilter) Filter(input string) string {
	if f == nil || !f.enabled.Load() {
		return input
	}

	inputLen := len(input)
	if inputLen == 0 {
		return input
	}

	if f.maxInputLength > 0 && inputLen > f.maxInputLength {
		input = input[:f.maxInputLength] + "... [TRUNCATED FOR SECURITY]"
	}

	f.mu.RLock()
	patternCount := len(f.patterns)
	if patternCount == 0 {
		f.mu.RUnlock()
		return input
	}

	patterns := make([]*regexp.Regexp, patternCount)
	copy(patterns, f.patterns)
	timeout := f.timeout
	f.mu.RUnlock()

	result := input
	for i := range patternCount {
		result = f.filterWithTimeout(result, patterns[i], timeout)
	}

	return result
}

func (f *SensitiveDataFilter) filterWithTimeout(input string, pattern *regexp.Regexp, timeout time.Duration) string {
	inputLen := len(input)
	if inputLen < fastPathThreshold {
		return pattern.ReplaceAllString(input, "[REDACTED]")
	}

	done := make(chan string, 1)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case done <- "[REDACTED]":
				default:
				}
			}
		}()
		output := pattern.ReplaceAllString(input, "[REDACTED]")
		select {
		case done <- output:
		case <-ctx.Done():
		}
	}()

	select {
	case output := <-done:
		return output
	case <-ctx.Done():
		return "[REDACTED]"
	}
}

func (f *SensitiveDataFilter) FilterValue(value any) any {
	if f == nil || !f.enabled.Load() {
		return value
	}
	if str, ok := value.(string); ok {
		return f.Filter(str)
	}
	return value
}

var sensitiveKeywords = []string{
	"password", "passwd", "pwd",
	"secret", "token", "bearer",
	"api_key", "apikey", "api-key",
	"access_key", "accesskey", "access-key",
	"secret_key", "secretkey", "secret-key",
	"private_key", "privatekey", "private-key",
	"auth", "authorization",
	"credit_card", "creditcard",
	"ssn", "social_security",
}

func isSensitiveKey(key string) bool {
	lowerKey := strings.ToLower(key)
	for _, keyword := range sensitiveKeywords {
		if lowerKey == keyword || strings.Contains(lowerKey, keyword) {
			return true
		}
	}
	return false
}

func (f *SensitiveDataFilter) FilterFieldValue(key string, value any) any {
	if f == nil || !f.enabled.Load() {
		return value
	}

	str, ok := value.(string)
	if !ok {
		return value
	}

	if isSensitiveKey(key) {
		return "[REDACTED]"
	}

	return f.Filter(str)
}

type SecurityConfig struct {
	MaxMessageSize  int
	MaxWriters      int
	SensitiveFilter *SensitiveDataFilter
}

func NewBasicSensitiveDataFilter() *SensitiveDataFilter {
	filter := &SensitiveDataFilter{
		patterns:       make([]*regexp.Regexp, 0, 6),
		maxInputLength: basicMaxInputLength,
		timeout:        defaultFilterTimeout,
	}
	filter.enabled.Store(true)

	patterns := []string{
		`\b[0-9]{13,19}\b`,
		`\b[0-9]{3}-[0-9]{2}-[0-9]{4}\b`,
		`(?i)(password|passwd|pwd)[\s:=]+[^\s]{1,32}`,
		`(?i)(api[_-]?key|token|bearer)[\s:=]+[^\s]{1,256}`,
		`\bsk-[A-Za-z0-9]{16,48}\b`,
		`-----BEGIN[^-]*PRIVATE\s+KEY-----[\s\S]*?-----END[^-]*PRIVATE\s+KEY-----`,
	}

	for _, pattern := range patterns {
		_ = filter.addPattern(pattern)
	}

	return filter
}

func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		MaxMessageSize:  defaultMaxMessageSize,
		MaxWriters:      defaultMaxWriters,
		SensitiveFilter: nil,
	}
}

func SecureSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		MaxMessageSize:  defaultMaxMessageSize,
		MaxWriters:      defaultMaxWriters,
		SensitiveFilter: NewSensitiveDataFilter(),
	}
}

const (
	truncatedSuffix       = "... [TRUNCATED]"
	invalidKeyName        = "invalid_key"
	defaultMaxMessageSize = 5 * 1024 * 1024 // 5MB
	defaultMaxWriters     = 100
	defaultMaxInputLength = 256 * 1024 // 256KB
	defaultFilterTimeout  = 50 * time.Millisecond
	basicMaxInputLength   = 64 * 1024   // 64KB
	emptyMaxInputLength   = 1024 * 1024 // 1MB
	emptyFilterTimeout    = 100 * time.Millisecond
	fastPathThreshold     = 100 // Fast path for small inputs in filterWithTimeout
)

func sanitizeFieldKey(key string) string {
	keyLen := len(key)
	if keyLen == 0 {
		return invalidKeyName
	}

	if keyLen > maxFieldKeyLength {
		key = key[:maxFieldKeyLength]
		keyLen = maxFieldKeyLength
	}

	// Fast path: check if sanitization is needed
	hasInvalid := false
	for i := range keyLen {
		if !isValidKeyChar(key[i]) {
			hasInvalid = true
			break
		}
	}

	if !hasInvalid {
		return key
	}

	// Slow path: remove invalid characters
	var sb strings.Builder
	sb.Grow(keyLen)

	for i := range keyLen {
		c := key[i]
		if isValidKeyChar(c) {
			sb.WriteByte(c)
		}
	}

	if sb.Len() == 0 {
		return invalidKeyName
	}

	return sb.String()
}

func isValidKeyChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.'
}
