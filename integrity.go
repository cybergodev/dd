package dd

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// HashAlgorithm defines the hash algorithm for integrity verification.
type HashAlgorithm int

const (
	// HashAlgorithmSHA256 uses SHA-256 for HMAC signatures.
	HashAlgorithmSHA256 HashAlgorithm = iota
)

// String returns the string representation of the hash algorithm.
func (a HashAlgorithm) String() string {
	switch a {
	case HashAlgorithmSHA256:
		return "SHA256"
	default:
		return "Unknown"
	}
}

// IntegrityConfig configures log integrity verification.
type IntegrityConfig struct {
	// SecretKey is the secret key for HMAC signatures.
	// Must be at least 32 bytes for SHA-256.
	// IMPORTANT: Keep this key secure and rotate periodically.
	SecretKey []byte

	// HashAlgorithm is the hash algorithm to use.
	// Default: SHA256
	HashAlgorithm HashAlgorithm

	// IncludeTimestamp determines if timestamps are included in signatures.
	IncludeTimestamp bool

	// IncludeSequence determines if sequence numbers are included.
	// This provides replay attack protection.
	IncludeSequence bool

	// SignaturePrefix is the prefix for signatures in log output.
	// Default: "[SIG:"
	SignaturePrefix string
}

// Validate validates the IntegrityConfig and returns an error if any field is invalid.
func (c IntegrityConfig) Validate() error {
	if len(c.SecretKey) < 32 {
		return fmt.Errorf("secret key must be at least 32 bytes, got %d", len(c.SecretKey))
	}
	switch c.HashAlgorithm {
	case HashAlgorithmSHA256:
		// Supported
	default:
		return fmt.Errorf("unsupported hash algorithm: %v", c.HashAlgorithm)
	}
	return nil
}

// DefaultIntegrityConfigSafe returns an IntegrityConfig with sensible defaults.
// Unlike DefaultIntegrityConfig, this function returns an error instead of panicking
// if the secure random key generation fails.
// This is the recommended function for production environments.
//
// Example:
//
//	cfg, err := dd.DefaultIntegrityConfigSafe()
//	if err != nil {
//	    // Handle error gracefully
//	    log.Fatal(err)
//	}
func DefaultIntegrityConfigSafe() (IntegrityConfig, error) {
	// Generate a cryptographically secure random key
	defaultKey := make([]byte, 32)
	if _, err := rand.Read(defaultKey); err != nil {
		return IntegrityConfig{}, fmt.Errorf("failed to generate secure random key: %w", err)
	}

	return IntegrityConfig{
		SecretKey:        defaultKey,
		HashAlgorithm:    HashAlgorithmSHA256,
		IncludeTimestamp: true,
		IncludeSequence:  true,
		SignaturePrefix:  "[SIG:",
	}, nil
}

// IntegritySigner signs log entries for integrity verification.
// It creates a new HMAC hasher for each Sign/Verify operation to ensure
// correct key management without pool-related complications.
type IntegritySigner struct {
	config    *IntegrityConfig
	secretKey []byte // Store key for creating new hashers
	sequence  atomic.Uint64
}

// NewIntegritySigner creates a new IntegritySigner with the given configuration.
// If no configuration is provided, a secure default configuration is generated.
// Returns an error if the default configuration cannot be generated (extremely rare,
// indicates system entropy exhaustion).
func NewIntegritySigner(cfg ...IntegrityConfig) (*IntegritySigner, error) {
	var config IntegrityConfig
	if len(cfg) > 0 {
		config = cfg[0]
	} else {
		var err error
		config, err = DefaultIntegrityConfigSafe()
		if err != nil {
			return nil, fmt.Errorf("failed to create default integrity config: %w", err)
		}
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	if config.SignaturePrefix == "" {
		config.SignaturePrefix = "[SIG:"
	}

	// Copy the secret key to ensure we own the memory
	secretKey := make([]byte, len(config.SecretKey))
	copy(secretKey, config.SecretKey)

	return &IntegritySigner{
		config:    &config,
		secretKey: secretKey,
	}, nil
}

// newHasher creates a new HMAC-SHA256 hasher configured with the secret key.
// Each operation gets a fresh hasher to avoid pool-related key management issues
// where hmac.Reset() restores the original creation key rather than a nil-key state.
// The allocation cost is negligible compared to the crypto operation itself.
func (s *IntegritySigner) newHasher() hash.Hash {
	return hmac.New(sha256.New, s.secretKey)
}

// signResult contains the components needed to build a signature string
type signResult struct {
	timestamp int64
	sequence  uint64
	signature []byte
}

// signData computes the HMAC signature for the given data builder content.
// It appends timestamp and sequence if configured, then computes the signature.
func (s *IntegritySigner) signData(data *strings.Builder) signResult {
	var timestamp int64
	var sequence uint64

	if s.config.IncludeTimestamp {
		timestamp = time.Now().UnixNano()
		data.WriteString("|")
		data.WriteString(strconv.FormatInt(timestamp, 10))
	}

	if s.config.IncludeSequence {
		sequence = s.sequence.Add(1)
		data.WriteString("|")
		data.WriteString(strconv.FormatUint(sequence, 10))
	}

	// Create a fresh hasher and compute HMAC
	hasher := s.newHasher()
	hasher.Write([]byte(data.String()))
	signature := hasher.Sum(nil)

	return signResult{
		timestamp: timestamp,
		sequence:  sequence,
		signature: signature,
	}
}

// buildSignatureString constructs the signature output string from the sign result.
func (s *IntegritySigner) buildSignatureString(result signResult) string {
	encodedSig := base64.RawURLEncoding.EncodeToString(result.signature)

	var sigBuilder strings.Builder
	sigBuilder.WriteString(s.config.SignaturePrefix)

	if s.config.IncludeTimestamp {
		sigBuilder.WriteString(strconv.FormatInt(result.timestamp, 10))
	}
	sigBuilder.WriteString(":")
	if s.config.IncludeSequence {
		sigBuilder.WriteString(strconv.FormatUint(result.sequence, 10))
	}
	sigBuilder.WriteString(":")
	sigBuilder.WriteString(encodedSig)
	sigBuilder.WriteString("]")

	return sigBuilder.String()
}

// Sign generates an HMAC signature for a log message.
// The signature includes the message, timestamp, and sequence number (if configured).
// Returns the signature string that should be appended to the log entry.
// This method is thread-safe and can be called concurrently.
//
// Signature format: [SIG:timestamp:sequence:signature] where timestamp and sequence
// are included only if configured. This allows proper verification of all signed data.
func (s *IntegritySigner) Sign(message string) string {
	if s == nil {
		return ""
	}

	var data strings.Builder
	data.WriteString(message)

	result := s.signData(&data)
	return s.buildSignatureString(result)
}

// SignFields generates an HMAC signature for a message with fields.
// Fields are included in the signature for additional integrity.
// This method is thread-safe and can be called concurrently.
//
// Signature format: [SIG:timestamp:sequence:signature] where timestamp and sequence
// are included only if configured. This allows proper verification of all signed data.
func (s *IntegritySigner) SignFields(message string, fields []Field) string {
	if s == nil {
		return ""
	}

	var data strings.Builder
	data.WriteString(message)

	for _, f := range fields {
		data.WriteString("|")
		data.WriteString(f.Key)
		data.WriteString("=")
		fmt.Fprintf(&data, "%v", f.Value)
	}

	result := s.signData(&data)
	return s.buildSignatureString(result)
}

// LogIntegrity contains the result of integrity verification.
type LogIntegrity struct {
	// Valid indicates if the signature is valid.
	Valid bool
	// Timestamp is the timestamp from the log entry (if included).
	Timestamp time.Time
	// Sequence is the sequence number (if included).
	Sequence uint64
	// Message is the extracted message without signature.
	Message string
}

// Verify verifies the integrity of a log entry.
// It validates that the signature matches the message, timestamp, and sequence (if configured).
// Returns the verification result and any error.
// This method is thread-safe and can be called concurrently.
func (s *IntegritySigner) Verify(entry string) (*LogIntegrity, error) {
	if s == nil {
		return nil, fmt.Errorf("signer is nil")
	}

	// Find signature prefix
	sigStart := strings.LastIndex(entry, s.config.SignaturePrefix)
	if sigStart == -1 {
		return &LogIntegrity{
			Valid:   false,
			Message: entry,
		}, nil
	}

	sigEnd := strings.Index(entry[sigStart:], "]")
	if sigEnd == -1 {
		return &LogIntegrity{
			Valid:   false,
			Message: entry,
		}, nil
	}

	// Extract the signature content (between prefix and ])
	sigContent := entry[sigStart+len(s.config.SignaturePrefix) : sigStart+sigEnd]
	message := entry[:sigStart]

	// Parse signature format: [SIG:ts:seq:sig]
	// Format can be: ts:seq:sig, :seq:sig, ts::sig, or :::sig
	parts := strings.SplitN(sigContent, ":", 3)
	if len(parts) != 3 {
		// Try legacy format (just base64 signature without metadata)
		return s.verifyLegacy(message, sigContent)
	}

	timestampStr := parts[0]
	sequenceStr := parts[1]
	signatureStr := parts[2]

	// Decode signature
	signature, err := base64.RawURLEncoding.DecodeString(signatureStr)
	if err != nil {
		return &LogIntegrity{
			Valid:   false,
			Message: message,
		}, nil
	}

	// Parse timestamp and sequence
	var timestamp time.Time
	var sequence uint64

	if s.config.IncludeTimestamp && timestampStr != "" {
		ts, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			return &LogIntegrity{
				Valid:   false,
				Message: message,
			}, nil
		}
		timestamp = time.Unix(0, ts)
	}

	if s.config.IncludeSequence && sequenceStr != "" {
		seq, err := strconv.ParseUint(sequenceStr, 10, 64)
		if err != nil {
			return &LogIntegrity{
				Valid:   false,
				Message: message,
			}, nil
		}
		sequence = seq
	}

	// Rebuild the signed data with the same format as Sign()
	var data strings.Builder
	data.WriteString(message)

	if s.config.IncludeTimestamp && timestampStr != "" {
		data.WriteString("|")
		data.WriteString(timestampStr)
	}

	if s.config.IncludeSequence && sequenceStr != "" {
		data.WriteString("|")
		data.WriteString(sequenceStr)
	}

	// Create a fresh hasher and recompute signature
	hasher := s.newHasher()
	hasher.Write([]byte(data.String()))
	expectedSig := hasher.Sum(nil)

	// Compare signatures (constant-time comparison)
	if !hmac.Equal(signature, expectedSig) {
		return &LogIntegrity{
			Valid:   false,
			Message: message,
		}, nil
	}

	return &LogIntegrity{
		Valid:     true,
		Message:   message,
		Timestamp: timestamp,
		Sequence:  sequence,
	}, nil
}

// verifyLegacy handles verification of legacy signature format (just base64 without metadata).
// This provides backward compatibility with signatures created before the format change.
func (s *IntegritySigner) verifyLegacy(message, sigStr string) (*LogIntegrity, error) {
	// Decode signature
	signature, err := base64.RawURLEncoding.DecodeString(sigStr)
	if err != nil {
		return &LogIntegrity{
			Valid:   false,
			Message: message,
		}, nil
	}

	// For legacy signatures, we can only verify the message portion
	// Create a fresh hasher and recompute signature
	hasher := s.newHasher()
	hasher.Write([]byte(message))
	expectedSig := hasher.Sum(nil)

	// Compare signatures (constant-time comparison)
	// Note: We compare the full expected signature, not a truncated version.
	// Truncating the expected signature to match the provided signature length
	// would significantly weaken the HMAC verification.
	if !hmac.Equal(signature, expectedSig) {
		return &LogIntegrity{
			Valid:   false,
			Message: message,
		}, nil
	}

	// Legacy signature valid but without timestamp/sequence verification
	return &LogIntegrity{
		Valid:   true,
		Message: message,
	}, nil
}

// GetSequence returns the current sequence number.
func (s *IntegritySigner) GetSequence() uint64 {
	if s == nil {
		return 0
	}
	return s.sequence.Load()
}

// ResetSequence resets the sequence counter to 0.
func (s *IntegritySigner) ResetSequence() {
	if s != nil {
		s.sequence.Store(0)
	}
}

// Clone creates a copy of the IntegrityConfig.
func (c *IntegrityConfig) Clone() IntegrityConfig {
	if c == nil {
		return IntegrityConfig{}
	}

	copiedKey := make([]byte, len(c.SecretKey))
	copy(copiedKey, c.SecretKey)

	return IntegrityConfig{
		SecretKey:        copiedKey,
		HashAlgorithm:    c.HashAlgorithm,
		IncludeTimestamp: c.IncludeTimestamp,
		IncludeSequence:  c.IncludeSequence,
		SignaturePrefix:  c.SignaturePrefix,
	}
}

// MarshalJSON implements json.Marshaler for IntegrityConfig.
// Note: SecretKey is intentionally not marshaled for security reasons.
func (c *IntegrityConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"hashAlgorithm":    c.HashAlgorithm.String(),
		"includeTimestamp": c.IncludeTimestamp,
		"includeSequence":  c.IncludeSequence,
		"signaturePrefix":  c.SignaturePrefix,
		"secretKeyLength":  len(c.SecretKey),
	})
}

// IntegrityStats holds integrity signer statistics.
type IntegrityStats struct {
	Sequence         uint64 // Current sequence number
	Algorithm        string // Hash algorithm name
	IncludeTimestamp bool   // Whether timestamps are included
	IncludeSequence  bool   // Whether sequence numbers are included
}

// Stats returns current integrity signer statistics.
func (s *IntegritySigner) Stats() IntegrityStats {
	if s == nil {
		return IntegrityStats{}
	}

	return IntegrityStats{
		Sequence:         s.sequence.Load(),
		Algorithm:        s.config.HashAlgorithm.String(),
		IncludeTimestamp: s.config.IncludeTimestamp,
		IncludeSequence:  s.config.IncludeSequence,
	}
}
