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
		maxInputLength: MaxInputLength,
		timeout:        DefaultFilterTimeout,
	}
	filter.enabled.Store(true)

	// Optimized security patterns - ReDoS resistant with strict boundaries
	patterns := []string{
		// Credit card numbers (13-19 digits with optional separators)
		`\b[0-9]{4}[- ]?[0-9]{4}[- ]?[0-9]{4}[- ]?[0-9]{3,7}\b`,
		// SSN (strict format)
		`\b[0-9]{3}-[0-9]{2}-[0-9]{4}\b`,
		// Password/secret fields (length limited)
		`(?i)(?:password|passwd|pwd|secret)[\s:=]+[^\s]{1,32}\b`,
		// API keys and tokens (length limited)
		`(?i)(?:token|api[_-]?key|bearer)[\s:=]+[^\s]{1,128}\b`,
		// JWT tokens (strict three-part format)
		`\beyJ[A-Za-z0-9_-]{10,100}\.eyJ[A-Za-z0-9_-]{10,100}\.[A-Za-z0-9_-]{10,100}\b`,
		// Private keys (bounded content)
		`-----BEGIN[^-]{1,20}PRIVATE\s+KEY-----[A-Za-z0-9+/=\s]{1,4000}-----END[^-]{1,20}PRIVATE\s+KEY-----`,
		// AWS Access Keys
		`\bAKIA[0-9A-Z]{16}\b`,
		// Google API Keys
		`\bAIza[A-Za-z0-9_-]{35}\b`,
		// OpenAI API Keys
		`\bsk-[A-Za-z0-9]{20,48}\b`,
		// Email addresses (simplified but secure)
		`\b[A-Za-z0-9._%+-]{1,64}@[A-Za-z0-9.-]{1,253}\.[A-Za-z]{2,6}\b`,
		// IPv4 addresses
		`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`,
		// Database connection strings
		`(?i)(?:mysql|postgresql|mongodb)://[^\s]{1,200}\b`,
	}

	for _, pattern := range patterns {
		if err := filter.addPattern(pattern); err != nil {
			// Log error but continue - don't fail initialization for pattern issues
			continue
		}
	}

	return filter
}

func NewEmptySensitiveDataFilter() *SensitiveDataFilter {
	filter := &SensitiveDataFilter{
		patterns:       make([]*regexp.Regexp, 0),
		maxInputLength: MaxInputLength,
		timeout:        EmptyFilterTimeout,
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

	// Fast path: small inputs processed directly
	if inputLen < fastPathThreshold {
		return pattern.ReplaceAllString(input, "[REDACTED]")
	}

	// Medium size inputs: chunk processing to avoid timeout
	if inputLen < 10*fastPathThreshold {
		return f.filterInChunks(input, pattern)
	}

	// Large inputs: use timeout protection
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

		// Process large input in chunks
		output := f.filterInChunks(input, pattern)
		select {
		case done <- output:
		case <-ctx.Done():
			// Context cancelled, exit goroutine
			return
		}
	}()

	select {
	case output := <-done:
		return output
	case <-ctx.Done():
		return "[REDACTED]"
	}
}

// Process input in chunks to improve performance and avoid timeout
func (f *SensitiveDataFilter) filterInChunks(input string, pattern *regexp.Regexp) string {
	const chunkSize = 1024
	inputLen := len(input)

	if inputLen <= chunkSize {
		return pattern.ReplaceAllString(input, "[REDACTED]")
	}

	var result strings.Builder
	result.Grow(inputLen) // Pre-allocate capacity

	for i := 0; i < inputLen; i += chunkSize {
		end := min(i+chunkSize, inputLen)

		chunk := input[i:end]
		filtered := pattern.ReplaceAllString(chunk, "[REDACTED]")
		result.WriteString(filtered)
	}

	return result.String()
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
		maxInputLength: MaxInputLength, // Use consistent constant
		timeout:        DefaultFilterTimeout,
	}
	filter.enabled.Store(true)

	// Basic security patterns - most common sensitive data types
	patterns := []string{
		// Credit card numbers (improved pattern with separators)
		`\b[0-9]{4}[- ]?[0-9]{4}[- ]?[0-9]{4}[- ]?[0-9]{3,7}\b`,
		// SSN (strict format)
		`\b[0-9]{3}-[0-9]{2}-[0-9]{4}\b`,
		// Password fields (atomic groups, length limited)
		`(?i)(?:password|passwd|pwd)[\s:=]+[^\s]{1,32}\b`,
		// API keys and tokens (atomic groups, length limited)
		`(?i)(?:api[_-]?key|token|bearer)[\s:=]+[^\s]{1,128}\b`,
		// OpenAI API Keys
		`\bsk-[A-Za-z0-9]{16,48}\b`,
		// Private keys (bounded content, reduced size for basic filter)
		`-----BEGIN[^-]{1,20}PRIVATE\s+KEY-----[A-Za-z0-9+/=\s]{1,2000}-----END[^-]{1,20}PRIVATE\s+KEY-----`,
	}

	for _, pattern := range patterns {
		_ = filter.addPattern(pattern)
	}

	return filter
}

func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		MaxMessageSize:  MaxMessageSize,
		MaxWriters:      MaxWriterCount,
		SensitiveFilter: nil,
	}
}

func SecureSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		MaxMessageSize:  MaxMessageSize,
		MaxWriters:      MaxWriterCount,
		SensitiveFilter: NewSensitiveDataFilter(),
	}
}

const (
	fastPathThreshold = 100 // Fast path for small inputs in filterWithTimeout
)
