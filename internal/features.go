package internal

// This file implements per-pattern necessary-condition gating for the
// sensitive-data filter's regex sweep. On messages that merely "could contain"
// sensitive data (see couldContainSensitiveData — any digits, '@', "://",
// credential keyword, ...), the filter otherwise runs every registered pattern's
// MatchString, which pprof showed was ~95% of CPU on formatted logging.
//
// ScanMessageFeatures extracts cheap structural features from the input in a
// single pass, and each built-in pattern carries a PatternGate describing
// features any match must imply. A pattern whose gate rejects the features
// cannot match, so its regex scan is skipped entirely.
//
// SECURITY: a gate is only allowed to reject when the condition is *necessary*
// for a match — i.e. gate ⊇ matches. An overly strict gate would silently
// disable redaction (a security hole); an overly loose one only costs
// performance. When in doubt, leave a condition out. Pattern-gate soundness is
// enforced by TestPatternGateEquivalence (curated corpus + randomized inputs).

// MessageFeatures holds the structural features of a message, extracted in one
// pass by ScanMessageFeatures. It is consumed by PatternGate.Allows.
type MessageFeatures struct {
	DigitTotal     int    // total ASCII digit count
	DigitRun       int    // longest run of consecutive ASCII digits
	Dots           int    // '.' count
	Colons         int    // ':' count
	HasAt          bool   // contains '@'
	HasPlus        bool   // contains '+'
	HasScheme      bool   // contains "://" (URL / connection schemes)
	HasDollarBrace bool   // contains "${" (JNDI-style lookups)
	Keywords       uint64 // keyword group bits present (case-insensitive)
	Literals       uint64 // literal token group bits present (case-sensitive)
}

// PatternGate is a set of conditions that must ALL hold for the gated pattern
// to possibly match. The zero value never rejects (pattern always runs).
type PatternGate struct {
	MinDigits     int    // minimum total digit count required by a match
	MinDigitRun   int    // minimum longest digit run required by a match
	MinDots       int    // minimum '.' count required by a match
	MinColons     int    // minimum ':' count required by a match
	NeedAt        bool   // match requires '@'
	NeedPlus      bool   // match requires '+'
	NeedScheme    bool   // match requires "://"
	NeedDollarBrk bool   // match requires "${"
	Keywords      uint64 // match requires a keyword from one of these groups
	Literals      uint64 // match requires a literal token from one of these groups
}

// Allows reports whether input with the given features could still match the
// gated pattern. false means the pattern provably cannot match.
func (g PatternGate) Allows(f MessageFeatures) bool {
	// Fast rejection: a pattern requiring a keyword or literal token can never
	// match a message carrying none — one branch stands in for the full check
	// for the majority of patterns on token-free messages (profiling showed
	// the per-pattern Allows calls were the next-hot cost after the token scan).
	if (g.Keywords != 0 || g.Literals != 0) && f.Keywords == 0 && f.Literals == 0 {
		return false
	}
	if f.DigitTotal < g.MinDigits || f.DigitRun < g.MinDigitRun ||
		f.Dots < g.MinDots || f.Colons < g.MinColons {
		return false
	}
	if g.NeedAt && !f.HasAt ||
		g.NeedPlus && !f.HasPlus ||
		g.NeedScheme && !f.HasScheme ||
		g.NeedDollarBrk && !f.HasDollarBrace {
		return false
	}
	if g.Keywords != 0 && f.Keywords&g.Keywords == 0 {
		return false
	}
	if g.Literals != 0 && f.Literals&g.Literals == 0 {
		return false
	}
	return true
}

// Keyword group bits. Each group is a set of keywords such that every keyword
// alternation in the gated patterns that references the group has all of its
// alternatives implied by at least one keyword in the group (any-of semantics).
const (
	kwCredentials uint64 = 1 << iota // password/passwd/pwd/secret/token/bearer/...
	kwCard                           // card / credit card
	kwServerHost                     // server / data source / host / oracle / tns / sid
	kwPhone                          // phone/mobile/tel/... /number
	kwSwift                          // swift/bic/iban/bank code
	kwCvv                            // cvv/cvc/security code/card verification
	kwIcd                            // icd/diagnosis/diag/dx/clinical code
	kwNpi                            // npi/national provider identifier
	kwMrn                            // mrn/medical record/patient id/health record
	kwPassport                       // passport
	kwLicense                        // driver's license / dl number / license number
	kwSin                            // sin / social insurance number
	kwConn                           // connection string / connstr / azure
	kwKafka                          // kafka / bootstrap server
	kwBio                            // fingerprint / face / biometric / bio hash
)

// gateKeywords maps keyword-group bits to the keywords that set them. Keywords
// are matched case-insensitively as substrings.
var gateKeywords = []struct {
	bit     uint64
	keyword string
}{
	{kwCredentials, "password"}, {kwCredentials, "passwd"}, {kwCredentials, "pwd"},
	{kwCredentials, "secret"}, {kwCredentials, "token"}, {kwCredentials, "bearer"},
	{kwCredentials, "auth"}, {kwCredentials, "credential"},
	{kwCredentials, "api_key"}, {kwCredentials, "apikey"}, {kwCredentials, "api-key"},

	{kwCard, "card"},

	{kwServerHost, "server"}, {kwServerHost, "data source"}, {kwServerHost, "host"},
	{kwServerHost, "oracle"}, {kwServerHost, "tns"}, {kwServerHost, "sid"},

	{kwPhone, "phone"}, {kwPhone, "mobile"}, {kwPhone, "tel"}, {kwPhone, "telephone"},
	{kwPhone, "cell"}, {kwPhone, "cellular"}, {kwPhone, "fax"}, {kwPhone, "contact"},
	{kwPhone, "number"},

	{kwSwift, "swift"}, {kwSwift, "bic"}, {kwSwift, "iban"}, {kwSwift, "bank"},

	{kwCvv, "cvv"}, {kwCvv, "cvc"}, {kwCvv, "cv2"}, {kwCvv, "security"},
	{kwCvv, "verification"},

	{kwIcd, "icd"}, {kwIcd, "diagnosis"}, {kwIcd, "diag"}, {kwIcd, "dx"},
	{kwIcd, "clinical"},

	{kwNpi, "npi"}, {kwNpi, "national"}, {kwNpi, "provider"},

	{kwMrn, "mrn"}, {kwMrn, "medical"}, {kwMrn, "patient"}, {kwMrn, "health"},

	{kwPassport, "passport"},

	// SECURITY: "dl" covers the separator-less/hyphenated spellings of
	// dl[_-]?number (dlnumber, dl-number, dl_number) that the underscore-only
	// keywords miss — a missing form here silently disables that pattern's
	// redaction (see TestPatternGateEquivalence). Same for "insurance" below
	// (social-insurance-number) and "fp" in kwBio (fpid). Widening a gate
	// costs only performance, never soundness.
	{kwLicense, "driver"}, {kwLicense, "license"}, {kwLicense, "dl_number"},
	{kwLicense, "dl-number"}, {kwLicense, "dl"},

	{kwSin, "sin"}, {kwSin, "social_insurance"}, {kwSin, "canadian"},
	{kwSin, "insurance"},

	{kwConn, "connstr"}, {kwConn, "connection"}, {kwConn, "azure"},

	{kwKafka, "kafka"}, {kwKafka, "bootstrap"},

	{kwBio, "fingerprint"}, {kwBio, "fp_id"}, {kwBio, "fp-id"}, {kwBio, "face"},
	{kwBio, "biometric"}, {kwBio, "bio_hash"}, {kwBio, "biohash"}, {kwBio, "bio-hash"},
	{kwBio, "fp"},
}

// Literal token group bits. Each group's tokens are case-sensitive substrings
// that any match of the gated patterns must contain.
const (
	litAKIA    uint64 = 1 << iota // "AKIA" / "ASIA"
	litAIza                       // "AIza"
	litSk                         // "sk-"
	litGHP                        // "ghp_" and friends
	litXox                        // "xox"
	litLive                       // "_live_"
	litPrivKey                    // "private_key"
	litPem                        // "BEGIN"
	litJndi                       // "jndi"
	litYa29                       // "ya29." / "1//"
	litFwo                        // "FwoGZXIvYXdz"
	litEyJ                        // "eyJ"
	litGlpat                      // "glpat-"
)

// gateLiterals maps literal-group bits to their tokens.
var gateLiterals = []struct {
	bit   uint64
	token string
}{
	{litAKIA, "AKIA"}, {litAKIA, "ASIA"},
	{litAIza, "AIza"},
	{litSk, "sk-"},
	{litGHP, "ghp_"}, {litGHP, "gho_"}, {litGHP, "ghu_"}, {litGHP, "ghs_"}, {litGHP, "ghr_"},
	{litXox, "xox"},
	{litLive, "_live_"},
	{litPrivKey, "private_key"},
	{litPem, "BEGIN"},
	{litJndi, "jndi"},
	{litYa29, "ya29."}, {litYa29, "1//"},
	{litFwo, "FwoGZXIvYXdz"},
	{litEyJ, "eyJ"},
	{litGlpat, "glpat-"},
}

// keywordIndex indexes the gateKeywords entries for single-pass scanning;
// literalIndex does the same for gateLiterals (case-sensitively).
var (
	keywordIndex = NewSecondByteIndex(keywordWords(), true)
	literalIndex = NewSecondByteIndex(literalWords(), false)
)

func keywordWords() []string {
	words := make([]string, len(gateKeywords))
	for i, gk := range gateKeywords {
		words[i] = gk.keyword
	}
	return words
}

func literalWords() []string {
	words := make([]string, len(gateLiterals))
	for i, gl := range gateLiterals {
		words[i] = gl.token
	}
	return words
}

// upperASCII returns the uppercase form of an ASCII letter, or c unchanged.
func upperASCII(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 32
	}
	return c
}

// lowerASCII returns the lowercase form of an ASCII letter, or c unchanged.
func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

// ScanMessageFeatures extracts MessageFeatures from input in a single pass,
// followed by indexed keyword and literal scans. It is only invoked on inputs
// that passed the couldContainSensitiveData gate, where the alternative is the
// full regex sweep.
func ScanMessageFeatures(input string) MessageFeatures {
	var f MessageFeatures
	run := 0
	for i := 0; i < len(input); i++ {
		c := input[i]
		switch {
		case c >= '0' && c <= '9':
			f.DigitTotal++
			run++
			if run > f.DigitRun {
				f.DigitRun = run
			}
			continue
		case c == '.':
			f.Dots++
		case c == ':':
			f.Colons++
			if !f.HasScheme && i+2 < len(input) && input[i+1] == '/' && input[i+2] == '/' {
				f.HasScheme = true
			}
		case c == '@':
			f.HasAt = true
		case c == '+':
			f.HasPlus = true
		case c == '$':
			if !f.HasDollarBrace && i+1 < len(input) && input[i+1] == '{' {
				f.HasDollarBrace = true
			}
		}
		run = 0
	}
	f.Keywords, f.Literals = scanTokens(input)
	return f
}

// scanTokens returns the OR of keyword-group bits and literal-group bits
// present in input, in a single pass over the input: at each position only
// the tokens whose first TWO bytes can continue there are compared (keywords
// case-insensitively, literals case-sensitively; see SecondByteIndex). This
// replaces two separate passes — one indexed keyword scan plus sixteen
// strings.Contains calls for the literals — which together were the bulk of
// ScanMessageFeatures' cost.
func scanTokens(input string) (keywords, literals uint64) {
	n := len(input)
	// Every token is at least two bytes, so the last byte can never start one.
	for i := 0; i+1 < n; i++ {
		c := input[i]
		if cands := keywordIndex.Candidates(c, input[i+1]); cands != nil {
			for _, ci := range cands {
				kw := gateKeywords[ci].keyword
				end := i + len(kw)
				if end > n {
					continue
				}
				if matchKeywordAt(input, i, kw) {
					keywords |= gateKeywords[ci].bit
				}
			}
		}
		if cands := literalIndex.Candidates(c, input[i+1]); cands != nil {
			for _, li := range cands {
				token := gateLiterals[li].token
				end := i + len(token)
				if end > n {
					continue
				}
				if input[i:end] == token {
					literals |= gateLiterals[li].bit
				}
			}
		}
	}
	return keywords, literals
}

// matchKeywordAt reports whether input[pos:] starts with kw, comparing
// case-insensitively for ASCII letters. Both sides are lowercase keywords.
func matchKeywordAt(input string, pos int, kw string) bool {
	for j := 0; j < len(kw); j++ {
		if lowerASCII(input[pos+j]) != kw[j] {
			return false
		}
	}
	return true
}
