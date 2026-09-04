package dd

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"strconv"
	"strings"
	"sync"
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

	// IncludeSequence determines if monotonic sequence numbers are included
	// in the signed payload. This enables replay-attack detection during
	// verification (consumers must track observed sequence numbers to reject
	// duplicates); it does not by itself prevent replay.
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
// Returns an error if the secure random key generation fails (does not panic).
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

// signDataPool pools bytes.Buffer for sign/verify operations to reduce allocations.
// Used for building the data to be hashed. Security-critical: zeroed before return.
var signDataPool = sync.Pool{
	New: func() any {
		buf := &bytes.Buffer{}
		buf.Grow(256) // typical signed data size
		return buf
	},
}

// signResultPool pools signResult structs to reduce allocations in the hot signing path.
var signResultPool = sync.Pool{
	New: func() any {
		return &signResult{}
	},
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
// Use DefaultIntegrityConfigSafe() to generate a cryptographically secure key.
//
// SECURITY: after the key is copied into the signer, the caller's cfg.SecretKey
// bytes are zeroed so the key material exists in only one place. The passed
// config therefore cannot be reused to create a second signer — call
// IntegrityConfig.Clone() (or generate a fresh key) per signer.
//
// Returns errors:
//   - When SecretKey is less than 32 bytes
//   - When HashAlgorithm is not supported
//
// Example:
//
//	cfg, err := dd.DefaultIntegrityConfigSafe()
//	if err != nil { /* handle */ }
//	signer, err := dd.NewIntegritySigner(cfg)
func NewIntegritySigner(cfg IntegrityConfig) (*IntegritySigner, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if cfg.SignaturePrefix == "" {
		cfg.SignaturePrefix = "[SIG:"
	}

	// Copy the secret key to ensure we own the memory
	secretKey := make([]byte, len(cfg.SecretKey))
	copy(secretKey, cfg.SecretKey)

	// SECURITY: Zero the config's copy so only secretKey field holds the material.
	// Prevents key material from existing in two memory locations.
	for i := range cfg.SecretKey {
		cfg.SecretKey[i] = 0
	}
	cfg.SecretKey = nil

	return &IntegritySigner{
		config:    &cfg,
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

// signData computes the HMAC signature for the given data buffer content.
// It appends timestamp and sequence if configured, then computes the signature.
// Uses pooled buffer to avoid allocations.
func (s *IntegritySigner) signData(data *bytes.Buffer) *signResult {
	result := signResultPool.Get().(*signResult)
	result.timestamp = 0
	result.sequence = 0
	result.signature = result.signature[:0]

	if s.config.IncludeTimestamp {
		result.timestamp = time.Now().UnixNano()
		data.WriteString("|")
		data.WriteString(strconv.FormatInt(result.timestamp, 10))
	}

	if s.config.IncludeSequence {
		result.sequence = s.sequence.Add(1)
		data.WriteString("|")
		data.WriteString(strconv.FormatUint(result.sequence, 10))
	}

	// Create a fresh hasher and compute HMAC directly from buffer bytes
	hasher := s.newHasher()
	hasher.Write(data.Bytes())
	result.signature = hasher.Sum(result.signature)

	return result
}

// buildSignatureString constructs the signature output string from the sign
// result. It is pure with respect to the pools: it borrows nothing and does
// NOT recycle result — the caller owns result and releases it (see Sign/
// SignFields), keeping borrow and release lexically adjacent. The string is
// short-lived and already allocation-bound (String + base64), so a
// strings.Builder beats pooling here; this also stops the HMAC-payload pool
// (signDataPool) from being borrowed for an unrelated purpose.
func (s *IntegritySigner) buildSignatureString(result *signResult) string {
	var sb strings.Builder
	// prefix + worst-case ts (19 digits) + seq (20 digits) + separators +
	// base64 of a 32-byte HMAC (43 chars) + "]"
	sb.Grow(len(s.config.SignaturePrefix) + 88)

	sb.WriteString(s.config.SignaturePrefix)
	if s.config.IncludeTimestamp {
		sb.WriteString(strconv.FormatInt(result.timestamp, 10))
	}
	sb.WriteString(":")
	if s.config.IncludeSequence {
		sb.WriteString(strconv.FormatUint(result.sequence, 10))
	}
	sb.WriteString(":")
	sb.WriteString(base64.RawURLEncoding.EncodeToString(result.signature))
	sb.WriteString("]")
	return sb.String()
}

// putSignResult zeroes the signature bytes and recycles the signResult.
// Callers must invoke it exactly once per signResultPool.Get, via defer next
// to the Get.
func putSignResult(result *signResult) {
	for i := range result.signature {
		result.signature[i] = 0
	}
	result.signature = result.signature[:0]
	signResultPool.Put(result)
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

	data := signDataPool.Get().(*bytes.Buffer)
	data.Reset()
	data.WriteString(message)

	// SECURITY: Zero buffer on panic to prevent key material leak from pool
	defer func() {
		b := data.Bytes()
		for i := range b {
			b[i] = 0
		}
		data.Reset()
		signDataPool.Put(data)
	}()

	result := s.signData(data)
	defer putSignResult(result)

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

	data := signDataPool.Get().(*bytes.Buffer)
	data.Reset()
	data.WriteString(message)

	for _, f := range fields {
		data.WriteString("|")
		data.WriteString(f.Key)
		data.WriteString("=")
		fmt.Fprintf(data, "%v", f.Value)
	}

	// SECURITY: Zero buffer on panic to prevent key material leak from pool
	defer func() {
		b := data.Bytes()
		for i := range b {
			b[i] = 0
		}
		data.Reset()
		signDataPool.Put(data)
	}()

	result := s.signData(data)
	defer putSignResult(result)

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

	// SEC-003: scan for the closing ']' only AFTER the prefix. A custom
	// SignaturePrefix may itself contain ']' (Validate does not reject one),
	// and scanning from the prefix start would find that inner bracket first,
	// inverting the content slice bounds (entry[hi:lo]) and panicking —
	// reachable from the Sign→Verify round-trip itself. For prefixes without
	// ']' the first bracket after the prefix start is also the first after
	// the prefix end, so this is byte-identical to the old scan.
	prefixEnd := sigStart + len(s.config.SignaturePrefix)
	sigEnd := strings.Index(entry[prefixEnd:], "]")
	if sigEnd == -1 {
		return &LogIntegrity{
			Valid:   false,
			Message: entry,
		}, nil
	}

	// Extract the signature content (between prefix and ])
	sigContent := entry[prefixEnd : prefixEnd+sigEnd]
	message := entry[:sigStart]

	// Parse signature format: [SIG:ts:seq:sig]
	// Emitted forms (buildSignatureString always writes both colons):
	// ts:seq:sig, :seq:sig, ts::sig, or ::sig
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

	// Rebuild the signed payload and compare signatures (constant-time).
	// Try the message as extracted first; on mismatch, retry once with a single
	// trailing space stripped: AuditLogger.writeEvent (and other callers)
	// append the signature after a one-space separator, but Sign() signs the
	// message WITHOUT that separator. Without the retry, every audit-written
	// entry failed verification because the separator byte leaked into the
	// re-signed payload.
	if hmac.Equal(signature, s.expectedSignature(message, timestampStr, sequenceStr)) {
		return &LogIntegrity{
			Valid:     true,
			Message:   message,
			Timestamp: timestamp,
			Sequence:  sequence,
		}, nil
	}
	if trimmed, ok := strings.CutSuffix(message, " "); ok {
		if hmac.Equal(signature, s.expectedSignature(trimmed, timestampStr, sequenceStr)) {
			return &LogIntegrity{
				Valid:     true,
				Message:   trimmed,
				Timestamp: timestamp,
				Sequence:  sequence,
			}, nil
		}
	}

	return &LogIntegrity{
		Valid:   false,
		Message: message,
	}, nil
}

// expectedSignature rebuilds the signed payload for message using the timestamp
// and sequence strings exactly as they appeared inside the signature, and
// returns the HMAC that Sign() would have produced for that combination.
// Shared by Verify and verifyLegacy so both stay in lock-step with signData's
// payload format.
func (s *IntegritySigner) expectedSignature(message, timestampStr, sequenceStr string) []byte {
	data := signDataPool.Get().(*bytes.Buffer)
	data.Reset()
	data.WriteString(message)

	if s.config.IncludeTimestamp && timestampStr != "" {
		data.WriteString("|")
		data.WriteString(timestampStr)
	}
	if s.config.IncludeSequence && sequenceStr != "" {
		data.WriteString("|")
		data.WriteString(sequenceStr)
	}

	// SECURITY: Zero buffer on panic to prevent key material leak from pool
	defer func() {
		bufBytes := data.Bytes()
		for i := range bufBytes {
			bufBytes[i] = 0
		}
		data.Reset()
		signDataPool.Put(data)
	}()

	// Create a fresh hasher and recompute signature directly from buffer bytes
	hasher := s.newHasher()
	hasher.Write(data.Bytes())
	return hasher.Sum(nil)
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

	// For legacy signatures, we can only verify the message portion.
	// On mismatch, retry once with a single trailing space stripped — the same
	// separator handling as Verify (see the comment there).
	valid := hmac.Equal(signature, s.expectedSignature(message, "", ""))
	msg := message
	if !valid {
		if trimmed, ok := strings.CutSuffix(message, " "); ok {
			if hmac.Equal(signature, s.expectedSignature(trimmed, "", "")) {
				valid = true
				msg = trimmed
			}
		}
	}
	if !valid {
		return &LogIntegrity{
			Valid:   false,
			Message: message,
		}, nil
	}

	// Legacy signature valid but without timestamp/sequence verification
	return &LogIntegrity{
		Valid:   true,
		Message: msg,
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
	if c == nil {
		return []byte("null"), nil
	}
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
