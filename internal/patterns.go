package internal

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// PatternDefinition represents a regex pattern for sensitive data detection.
type PatternDefinition struct {
	Pattern string
	Basic   bool // Included in basic filter
}

// AllPatterns is the centralized registry of all security patterns.
var AllPatterns = []PatternDefinition{
	// Credit card and SSN patterns
	{`\b[0-9]{4}[- ]?[0-9]{4}[- ]?[0-9]{4}[- ]?[0-9]{3,7}\b`, true},
	{`\b[0-9]{3}-[0-9]{2}-[0-9]{4}\b`, true},
	{`(?i)((?:credit[_-]?card|card)[\s:=]+)[0-9]{13,19}\b`, true},
	// Credentials and secrets
	{`(?i)((?:password|passwd|pwd|secret)[\s:=]+)[^\s]{1,128}\b`, true},
	{`(?i)((?:token|api[_-]?key|bearer)[\s:=]+)[^\s]{1,256}\b`, true},
	{`\beyJ[A-Za-z0-9_-]{10,100}\.eyJ[A-Za-z0-9_-]{10,100}\.[A-Za-z0-9_-]{10,100}\b`, false},
	{`-----BEGIN[^-]{1,20}PRIVATE\s+KEY-----[A-Za-z0-9+/=\s]{1,4000}-----END[^-]{1,20}PRIVATE\s+KEY-----`, true},
	// API keys
	// Merged AWS Access Key patterns (AKIA for permanent, ASIA for temporary)
	{`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`, true},
	{`\bAIza[A-Za-z0-9_-]{35}\b`, false},
	{`\bsk-[A-Za-z0-9]{16,48}\b`, true},
	// Email - only in full filter mode to avoid false positives on user@host format
	{`\b[A-Za-z0-9._%+-]{1,64}@[A-Za-z0-9.-]{1,253}\.[A-Za-z]{2,6}\b`, false},
	// IP addresses
	{`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`, false},
	// IPv6 addresses (full and compressed formats)
	{`\b(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\b`, false},                               // Full IPv6
	{`\b(?:[0-9a-fA-F]{1,4}:){1,7}:\b`, false},                                            // Trailing ::
	{`\b::(?:[0-9a-fA-F]{1,4}:){0,5}[0-9a-fA-F]{1,4}\b`, false},                           // Leading ::
	{`\b(?:[0-9a-fA-F]{1,4}:){1,4}::(?:[0-9a-fA-F]{1,4}:){0,3}[0-9a-fA-F]{1,4}\b`, false}, // Mixed :: in middle
	{`\b(?:[0-9a-fA-F]{1,4}:){1,5}::[0-9a-fA-F]{1,4}\b`, false},                           // :: with 5 groups before
	{`\b(?:[0-9a-fA-F]{1,4}:){1,6}::\b`, false},                                           // :: with 6 groups before
	{`\b::(?:[0-9a-fA-F]{1,4}:){1,6}[0-9a-fA-F]{1,4}\b`, false},                           // :: with groups after
	{`\b(?:[0-9a-fA-F]{1,4}:){6}(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`, false},                  // IPv6 with IPv4 suffix
	// Database connection strings - preserve protocol name
	{`(?i)((?:mysql|postgresql|mongodb|redis|sqlite|cassandra|influx|cockroach|timescale|postgres)://)[^\s]{1,200}\b`, true},
	// JDBC connection strings - preserve jdbc:prefix
	{`(?i)((?:jdbc:)(?:mysql|postgresql|sqlserver|oracle|mongodb|redis|cassandra)://)[^\s]{1,200}\b`, false},
	{`(?i)((?:server|data source|host)[\s=:]+)[^\s;]{1,200}(?:;|\s|$)`, false},
	{`(?i)((?:oracle|tns|sid)[\s=:]+)[^\s]{1,100}\b`, false},
	{`(?i)(?:[\w.-]+:[\w.-]+@)(?:[\w.-]+|\([^\)]+\))(?::\d+)?(?:/[\w.-]+)?`, false},
	// Phone numbers - global patterns
	{`(?i)((?:phone|mobile|tel|telephone|cell|cellular|fax|contact|number)[\s:=]+)[\+]?[(]?\d{1,4}[)]?[-\s.]?\(?\d{1,4}\)?[-\s.]?\d{1,9}[-\s.]?\d{0,9}\b`, true},
	{`\+\d{1,3}[- ]?\d{6,14}\b`, true},                          // International: +XXXXXXXXXXXX (7-15 digits after +)
	{`\+[\d\s\-\(\)]{7,20}\b`, true},                            // International phone with + and formatting (7-20 chars total, bounded)
	{`\b00[1-9]\d{6,14}\b`, true},                               // 00 prefix international (8-16 digits total)
	{`\b(?:\(\d{3}\)\s?|\d{3}[-.\s])\d{3}[-.\s]?\d{4}\b`, true}, // NANP with required separator: (415) 555-2671 or 415-555-2671
	{`\b\d{3,5}[- ]\d{4,8}\b`, false},                           // Phone numbers with separators (7-13 digits total) - moved to full filter to avoid false positives on dates
	{`\b0\d{3,5}[- ]?\d{4,8}\b`, true},                          // Starting with 0 and separators (10+ digits total)

	// ===== Enterprise Patterns =====

	// Financial Services (PCI-DSS compliance)
	// SWIFT/BIC codes (8 or 11 characters: BBBBCCLLbbb)
	// BBBB = bank code (4 letters), CC = country code (2 letters), LL = location code (2 alphanumeric), bbb = branch code (optional 3 alphanumeric)
	// Context-aware pattern to reduce false positives - requires context keywords like "swift", "bic", "bank"
	{`(?i)(?:swift|bic|bank[_-]?code|iban)[\s:=]+[A-Z]{4}[A-Z]{2}[A-Z0-9]{2}(?:[A-Z0-9]{3})?\b`, true},
	// IBAN (International Bank Account Number) - generic pattern
	{`\b[A-Z]{2}[0-9]{2}[A-Z0-9]{4}[0-9]{7,30}\b`, false},
	// CVV/CVC codes with context
	{`(?i)(?:cvv|cvc|cv2|security[_-]?code|card[_-]?verification)[\s:=]+[0-9]{3,4}\b`, true},

	// Healthcare (HIPAA compliance)
	// ICD-10 Diagnosis codes with medical context (e.g., "diagnosis: A12.3", "icd10: S72.0")
	// Requires context keywords to reduce false positives from generic codes
	{`(?i)(?:icd[-_]?10?|diagnosis|diag|dx|diagnostic[_-]?code|clinical[_-]?code)[\s:=]+[A-Z][0-9]{2}(?:\.[0-9A-Z]{1,4})?\b`, true},
	// US National Provider Identifier (NPI) - 10 digits starting with 1 or 2
	// Context-aware pattern to reduce false positives from random 10-digit numbers
	{`(?i)(?:npi|national[_-]?provider[_-]?identifier|provider[_-]?id)[\s:=]+[12][0-9]{9}\b`, true},
	// Medical Record Numbers (MRN) with context
	{`(?i)(?:mrn|medical[_-]?record[_-]?number|patient[_-]?id|health[_-]?record)[\s:=]+[A-Za-z0-9]{6,20}\b`, true},
	// Health Insurance Claim Number (HICN) - Medicare format
	{`\b[0-9]{9}[A-Z]{1,2}\b`, false},

	// Government/Identity
	// US Passport numbers (9 digits, or 8 digits for older)
	{`(?i)(?:passport[_-]?number|passport[_-]?no|passport[_-]?id)[\s:=]+[0-9]{8,9}\b`, true},
	// US Driver's License with context (state-specific, generic)
	{`(?i)(?:driver[_-]?license|dl[_-]?number|license[_-]?number|drivers[_-]?license)[\s:=]+[A-Za-z0-9]{5,20}\b`, true},
	// US Tax ID / Employer Identification Number (EIN)
	{`\b[0-9]{2}-[0-9]{7}\b`, false},
	// UK National Insurance Number
	{`\b[A-CEGHJ-PR-TW-Z][A-CEGHJ-NPR-TW-Z][0-9]{6}[A-D]\b`, false},
	// Canadian Social Insurance Number (SIN) - with context to avoid false positives on phone numbers
	{`(?i)(?:sin|social[_-]?insurance[_-]?number|canadian[_-]?sin)[\s:=]+[0-9]{3}[- ]?[0-9]{3}[- ]?[0-9]{3}\b`, true},

	// Cloud Provider Tokens
	// GitHub tokens (merged: p=personal, o=oauth, u=user-to-server, s=server-to-server, r=refresh)
	{`\b(?:ghp_|gho_|ghu_|ghs_|ghr_)[A-Za-z0-9]{36}\b`, true},
	// Slack tokens
	{`\bxox[baprs]-[0-9]{10,13}-[0-9]{10,13}-[a-zA-Z0-9]{24}\b`, true},
	// Stripe keys (merged: sk=secret, rk=restricted, stk=connect token)
	{`\b(?:sk|rk|stk)_live_[0-9a-zA-Z]{24,64}\b`, true},
	// GCP Service Account (JSON key structure indicator)
	{`"private_key"\s*:\s*"[^"]{100,4000}"`, true}, // Bounded max
	// Azure Connection String
	{`(?i)(?:connection[_-]?string|connstr|azure[_-]?connection)[\s:=]+[^\s]{50,500}`, true},
	// Generic OAuth/Refresh tokens with context
	{`(?i)(?:refresh[_-]?token|access[_-]?token|auth[_-]?token|bearer)[\s:=]+[A-Za-z0-9_\-\.]{20,256}\b`, true}, // Bounded max

	// ===== Log4Shell and JNDI Injection Patterns =====
	// CVE-2021-44228 - Log4Shell vulnerability patterns
	{`\$\{jndi:[^}]{0,200}\}`, true},                  // Basic JNDI lookup (bounded)
	{`\$\{(?:lower|upper) *: *j[a-z]{0,10}\}`, false}, // Obfuscated JNDI (bounded)
	{`\$\{[^}]{0,100}jndi[^}]{0,100}\}`, true},        // Any JNDI in expression (bounded)
	// Suspicious protocols in logs (potential JNDI/RMI/LDAP injection)
	{`(?i)(?:ldap|ldaps|rmi|dns|iiop|corba)://[^\s]{1,200}`, false},

	// ===== Modern Authentication Tokens =====
	// Anthropic and OpenAI API keys (merged: sk-ant, sk-proj)
	{`\bsk-(?:ant|proj)-[A-Za-z0-9_-]{32,128}\b`, true},
	// GitLab Personal Access Tokens
	{`\bglpat-[A-Za-z0-9_-]{20,128}\b`, true}, // Bounded max
	// Google OAuth tokens
	{`\b(?:ya29\.|1//)[A-Za-z0-9_\-\.]{20,256}\b`, true}, // Bounded max
	// AWS STS Session Tokens
	{`\bFwoGZXIvYXdz[ A-Za-z0-9/+=]{40,256}\b`, false}, // Bounded max

	// ===== Message Queue and Streaming =====
	// RabbitMQ connection strings
	{`(?i)(?:amqp|amqps)://[^\s]{1,200}\b`, true},
	// NATS connection strings
	{`(?i)nats://[^\s]{1,200}\b`, false},
	// Kafka connection strings (bootstrap servers)
	{`(?i)(?:kafka|bootstrap[_-]?server)[\s:=]+[a-z0-9._-]+:\d{1,5}`, false},

	// ===== International Identifiers =====
	// Australia ABN (Australian Business Number) - requires separator to avoid matching generic 11-digit numbers
	{`\b\d{2}[- ]\d{3}[- ]?\d{3}[- ]?\d{3}\b`, false},
	// New Zealand IRD (Inland Revenue Department) Number
	{`\b\d{8,9}\b`, false},
	// Chile RUT (Rol Único Tributario)
	{`\b\d{1,2}\.\d{3}\.\d{3}-[\dKk]\b`, false},
	// Brazil CPF
	{`\b\d{3}\.\d{3}\.\d{3}-\d{2}\b`, false},
	// Mexico RFC (Registro Federal de Contribuyentes)
	{`\b[A-ZÑ&]{3,4}\d{6}[A-Z0-9]{3}\b`, false},

	// ===== Biometric and Identity =====
	// Fingerprint template identifiers
	{`(?i)(?:fingerprint[_-]?template|fp[_-]?id)[\s:=]+[A-Za-z0-9_-]{10,128}\b`, true}, // Bounded max
	// Face recognition template identifiers
	{`(?i)(?:face[_-]?template|face[_-]?id)[\s:=]+[A-Za-z0-9_-]{10,128}\b`, true}, // Bounded max
	// Biometric data indicators
	{`(?i)(?:biometric[_-]?data|bio[_-]?hash)[\s:=]+[A-Za-z0-9+/=]{20,256}\b`, true}, // Bounded max
}

// Pre-compiled regex cache to avoid repeated compilation.
var (
	CompiledFullPatterns  []*regexp.Regexp
	CompiledBasicPatterns []*regexp.Regexp
	PatternsOnce          sync.Once
)

// InitPatterns initializes the pre-compiled regex patterns.
// This is called once on first use to avoid startup overhead.
func InitPatterns() {
	PatternsOnce.Do(func() {
		CompiledFullPatterns = make([]*regexp.Regexp, 0, len(AllPatterns))
		CompiledBasicPatterns = make([]*regexp.Regexp, 0, len(AllPatterns))

		for _, pd := range AllPatterns {
			// Skip ReDoS check for built-in patterns (already validated)
			re, err := regexp.Compile(pd.Pattern)
			if err != nil {
				// Output warning in debug/test mode for built-in pattern compilation failures
				if os.Getenv("DD_DEBUG") != "" {
					fmt.Fprintf(os.Stderr, "dd: warning: failed to compile pattern %q: %v\n", pd.Pattern, err)
				}
				continue
			}
			CompiledFullPatterns = append(CompiledFullPatterns, re)
			if pd.Basic {
				CompiledBasicPatterns = append(CompiledBasicPatterns, re)
			}
		}
	})
}

// HasNestedQuantifiers checks for regex patterns with nested quantifiers
// that can cause exponential backtracking (ReDoS vulnerability).
// Returns true if dangerous patterns like (a+)+, a++, or a{1,10000} are found.
func HasNestedQuantifiers(pattern string, maxQuantifierRange int) bool {
	// Track consecutive quantifiers
	prevWasQuantifier := false

	// Track if the content inside a group ends with a quantifier
	// This helps detect (a+)+ patterns
	groupEndsWithQuantifier := make(map[int]bool)
	// Track if a group contains alternation with quantified parts
	groupHasQuantifiedAlternation := make(map[int]bool)
	depth := 0

	for i := 0; i < len(pattern); i++ {
		c := pattern[i]

		switch c {
		case '(':
			depth++
			prevWasQuantifier = false
			groupEndsWithQuantifier[depth] = false
			groupHasQuantifiedAlternation[depth] = false
		case ')':
			if depth > 0 {
				// Check if this group is followed by a repeating quantifier (+, *, {n,})
				// AND the group content ends with a quantifier or has quantified alternation
				if i+1 < len(pattern) && (groupEndsWithQuantifier[depth] || groupHasQuantifiedAlternation[depth]) {
					next := pattern[i+1]
					// Only + and * are dangerous when applied to a quantified group
					// ? is safe because it's optional (no repetition)
					if next == '+' || next == '*' {
						return true
					}
					if next == '{' {
						// An open-ended range {n,} applied to a quantified group is
						// equivalent to an unbounded quantifier (e.g. (a+){1,} ~ (a+)+),
						// which causes catastrophic backtracking. ValidateQuantifierRange
						// intentionally accepts {n,} (it only bounds the upper value), so
						// detect it explicitly here. Bounded ranges {n,m} are acceptable;
						// excessively large ones are caught separately when the '{' itself
						// is processed below (the case '{' branch).
						end := strings.Index(pattern[i+1:], "}")
						if end != -1 {
							rangeContent := pattern[i+2 : i+1+end]
							if isOpenEndedRange(rangeContent) {
								return true
							}
						}
					}
				}
				delete(groupEndsWithQuantifier, depth)
				delete(groupHasQuantifiedAlternation, depth)
				depth--
			}
			prevWasQuantifier = false
		case '|':
			// Alternation - if we have a quantifier before this, mark the group
			if depth > 0 && prevWasQuantifier {
				groupHasQuantifiedAlternation[depth] = true
			}
			prevWasQuantifier = false
		case '+', '*', '?':
			// Check for consecutive quantifiers (e.g., a++, a*?)
			if prevWasQuantifier {
				return true
			}
			// Mark that current depth ends with a quantifier
			if depth > 0 {
				groupEndsWithQuantifier[depth] = true
			}
			prevWasQuantifier = true
		case '{':
			// Find the closing brace
			end := strings.Index(pattern[i:], "}")
			if end != -1 {
				// Check for consecutive quantifier like a{1,2}+
				if prevWasQuantifier {
					return true
				}

				// Check for excessive quantifier range
				rangeContent := pattern[i+1 : i+end]
				if err := ValidateQuantifierRange(rangeContent, maxQuantifierRange); err != nil {
					return true
				}

				// Mark that current depth ends with a quantifier
				if depth > 0 {
					groupEndsWithQuantifier[depth] = true
				}
				prevWasQuantifier = true
				i += end
			}
		default:
			// Reset for non-special characters (but not for \, |, ^, $, ., [, ])
			if c != '\\' && c != '|' && c != '^' && c != '$' && c != '.' {
				prevWasQuantifier = false
			}
		}
	}

	return false
}

// ValidateQuantifierRange checks if a quantifier range is within safe limits.
func ValidateQuantifierRange(rangeStr string, maxQuantifierRange int) error {
	parts := strings.Split(rangeStr, ",")

	// Parse the maximum value
	var maxVal int
	var err error

	if len(parts) == 1 {
		// Exact count: {n}
		maxVal, err = ParseInt(parts[0])
	} else if len(parts) == 2 {
		// Range: {n,m} or {n,}
		if parts[1] == "" {
			// Open-ended range {n,} - dangerous, but handled elsewhere
			return nil
		}
		maxVal, err = ParseInt(parts[1])
	} else {
		return fmt.Errorf("invalid quantifier range")
	}

	if err != nil {
		return err
	}

	if maxVal > maxQuantifierRange {
		return fmt.Errorf("quantifier range %d exceeds maximum %d", maxVal, maxQuantifierRange)
	}

	return nil
}

// isOpenEndedRange reports whether a quantifier range body (the text between
// '{' and '}') is open-ended, i.e. of the form {n,} with no upper bound. Such a
// quantifier applied to a quantified group behaves like an unbounded repetition
// and can trigger catastrophic backtracking. ValidateQuantifierRange accepts
// {n,} (it only bounds the upper value), so callers that must reject unbounded
// repetition use this helper instead.
func isOpenEndedRange(rangeStr string) bool {
	parts := strings.Split(rangeStr, ",")
	return len(parts) == 2 && parts[1] == ""
}

// ParseInt safely parses an integer from a string.
func ParseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	return strconv.Atoi(s)
}

// SensitiveKeywords contains field names that indicate sensitive data.
// These keywords support both exact match and substring matching.
// For short keywords that may cause false positives (e.g., "db", "url"),
// use ExactMatchOnlyKeywords instead.
//
// Categories:
//   - Credentials: password, passwd, pwd, secret, token, bearer, auth, authorization, credential
//   - API Keys: api_key, apikey, api-key, access_key, accesskey, access-key, client_id, client_secret
//   - Secrets: secret_key, secretkey, secret-key, private_key, privatekey, private-key, private_key_id
//   - Tokens: session_id, session_token, refresh_token, access_token, oauth_token
//   - OAuth: consumer_key, consumer_secret
//   - PII: credit_card, creditcard, ssn, social_security
//   - Contact: phone, telephone, mobile, cell, cellular, tel, fax, contact
var SensitiveKeywords = map[string]struct{}{
	// Credentials
	"password":      {},
	"passwd":        {},
	"pwd":           {},
	"secret":        {},
	"token":         {},
	"bearer":        {},
	"auth":          {},
	"authorization": {},
	"credential":    {},
	"credentials":   {},

	// API Keys
	"api_key":       {},
	"apikey":        {},
	"api-key":       {},
	"access_key":    {},
	"accesskey":     {},
	"access-key":    {},
	"client_id":     {},
	"clientid":      {},
	"client_secret": {},

	// Secrets
	"secret_key":     {},
	"secretkey":      {},
	"secret-key":     {},
	"private_key":    {},
	"privatekey":     {},
	"private-key":    {},
	"private_key_id": {},

	// Session and Tokens
	"session_id":    {},
	"sessionid":     {},
	"session_token": {},
	"refresh_token": {},
	"refreshtoken":  {},
	"access_token":  {},
	"accesstoken":   {},
	"oauth_token":   {},
	"auth_token":    {},
	"authtoken":     {},

	// OAuth Consumer
	"consumer_key":    {},
	"consumer_secret": {},

	// Personal Identifiable Information (PII)
	"credit_card":     {},
	"creditcard":      {},
	"ssn":             {},
	"social_security": {},

	// Contact Information
	"phone":        {},
	"telephone":    {},
	"mobile":       {},
	"cell":         {},
	"cellular":     {},
	"tel":          {},
	"fax":          {},
	"contact":      {},
	"phonenumber":  {},
	"phone_number": {},

	// Database/Connection Strings (longer forms that are less likely to cause false positives)
	"connection": {},
	"database":   {},
	"hostname":   {},
	"endpoint":   {},
}

// ExactMatchOnlyKeywords contains keywords that should only match exactly.
// These are typically short words that could cause false positives with substring matching.
// For example, "db" should not match "mongodb", and "url" should not match "curl".
var ExactMatchOnlyKeywords = map[string]struct{}{
	// Short words that need exact matching to avoid false positives
	"conn": {},
	"dsn":  {},
	"db":   {},
	"host": {},
	"uri":  {},
	"url":  {},
}

// Pre-computed sorted keyword slices for efficient exact matching with binary search
var (
	// sortedSensitiveKeywords is a pre-sorted slice of SensitiveKeywords for binary search
	sortedSensitiveKeywords []string
	// sortedExactMatchKeywords is a pre-sorted slice of ExactMatchOnlyKeywords for binary search
	sortedExactMatchKeywords []string
	// substringCheckKeywords contains keywords sorted alphabetically for substring matching
	substringCheckKeywords []string
	// keywordInitOnce ensures keyword slices are initialized only once
	keywordInitOnce sync.Once
)

// initKeywordSlices initializes the pre-sorted keyword slices for efficient lookup.
// This is called once on first use.
func initKeywordSlices() {
	keywordInitOnce.Do(func() {
		// Initialize sorted slices for exact match
		sortedSensitiveKeywords = make([]string, 0, len(SensitiveKeywords))
		for k := range SensitiveKeywords {
			sortedSensitiveKeywords = append(sortedSensitiveKeywords, k)
		}
		sort.Strings(sortedSensitiveKeywords)

		sortedExactMatchKeywords = make([]string, 0, len(ExactMatchOnlyKeywords))
		for k := range ExactMatchOnlyKeywords {
			sortedExactMatchKeywords = append(sortedExactMatchKeywords, k)
		}
		sort.Strings(sortedExactMatchKeywords)

		// Initialize substring check keywords (same as sensitive keywords)
		substringCheckKeywords = make([]string, len(sortedSensitiveKeywords))
		copy(substringCheckKeywords, sortedSensitiveKeywords)
	})
}

// binarySearchString performs binary search on a sorted string slice.
// Returns true if the target is found.
func binarySearchString(sorted []string, target string) bool {
	lo, hi := 0, len(sorted)-1
	for lo <= hi {
		mid := lo + (hi-lo)/2
		if sorted[mid] == target {
			return true
		}
		if sorted[mid] < target {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return false
}

// IsSensitiveKey checks if a key indicates sensitive data.
// It uses both exact match and substring matching for comprehensive detection.
// Performance optimized with pre-sorted slices and binary search for exact matching,
// and efficient substring matching for compound keys.
func IsSensitiveKey(key string) bool {
	if key == "" {
		return false
	}

	// Ensure keyword slices are initialized
	initKeywordSlices()

	// Convert key to lowercase inline to avoid allocation
	keyLen := len(key)
	var lowerKey string

	if keyLen <= 64 {
		// Use stack-allocated buffer for short keys
		var lowerBuf [64]byte
		for i := 0; i < keyLen; i++ {
			c := key[i]
			if c >= 'A' && c <= 'Z' {
				c += 32 // ASCII lowercase conversion
			}
			lowerBuf[i] = c
		}
		lowerKey = string(lowerBuf[:keyLen])
	} else {
		// Use strings.ToLower for long keys
		lowerKey = strings.ToLower(key)
	}

	// Fast path: binary search for exact match in sensitive keywords
	if binarySearchString(sortedSensitiveKeywords, lowerKey) {
		return true
	}

	// Fast path: binary search for exact match in exact-match-only keywords
	if binarySearchString(sortedExactMatchKeywords, lowerKey) {
		return true
	}

	// Substring match for compound keys like "user_password", "api_key_secret", etc.
	// Use pre-sorted slice instead of map iteration for better cache locality
	for _, keyword := range substringCheckKeywords {
		if strings.Contains(lowerKey, keyword) {
			return true
		}
	}

	return false
}
