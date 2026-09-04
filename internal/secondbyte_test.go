package internal

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
)

// startsWithPrefix reports whether s starts with prefix, compared
// case-insensitively when fold is set. s may be shorter than prefix.
func startsWithPrefix(s, prefix string, fold bool) bool {
	if len(s) < len(prefix) {
		return false
	}
	if fold {
		return strings.EqualFold(s[:len(prefix)], prefix)
	}
	return s[:len(prefix)] == prefix
}

// TestSecondByteIndexEquivalence checks the two properties the token scans
// rely on: Candidates never rejects a position where an indexed word starts
// (soundness), and every candidate it returns shares the two-byte prefix at
// that position (no spurious candidates). Verified over fixed and random
// inputs against direct prefix comparisons.
func TestSecondByteIndexEquivalence(t *testing.T) {
	words := []string{
		"password", "card", "cv2", "api_key", "dl", "bank", "xox",
		"AKIA", "AIza", "sk-", "_live_", "BEGIN", "FwoGZXIvYXdz",
	}
	inputs := []string{
		"", "a", "ab", "password", "PASSWORD", "PassWord",
		"my_password_here", "credit CARD: 1234", "cv2=123",
		"AKIAIOSFODNN7EXAMPLE", "AIzaSyA1234", "sk-abcdefgh",
		"sk_live_key", "-----BEGIN PRIVATE", "fingerprint fp_id",
		"FwoGZXIvYXdzEAabc", "begin at start", "z",
	}

	rng := rand.New(rand.NewSource(1))
	alphabet := []byte("abcdeklprstvwzAKXZ_-.2@9")
	for range 2000 {
		b := make([]byte, rng.Intn(24))
		for j := range b {
			b[j] = alphabet[rng.Intn(len(alphabet))]
		}
		inputs = append(inputs, string(b))
	}

	for _, fold := range []bool{true, false} {
		idx := NewSecondByteIndex(words, fold)
		for _, input := range inputs {
			// The scan loop never tests the final position (every indexed
			// word needs at least two bytes), mirroring the production scans.
			for i := 0; i+1 < len(input); i++ {
				cands := idx.Candidates(input[i], input[i+1])

				// Candidates is non-nil exactly when some word's two-byte
				// prefix occurs at offset i.
				prefixMatches := 0
				for _, w := range words {
					if startsWithPrefix(input[i:], w[:2], fold) {
						prefixMatches++
					}
				}
				if prefixMatches == 0 && cands != nil {
					t.Fatalf("fold=%v: Candidates(%q,%q)=%v at offset %d of %q, want nil (no two-byte prefix matches)",
						fold, input[i], input[i+1], cands, i, input)
				}
				if prefixMatches > 0 && cands == nil {
					t.Fatalf("fold=%v: Candidates(%q,%q)=nil at offset %d of %q, but a two-byte prefix matches",
						fold, input[i], input[i+1], i, input)
				}

				// Any word starting at i (full match) must be among the candidates.
				for wi, w := range words {
					if startsWithPrefix(input[i:], w, fold) && !slices.Contains(cands, wi) {
						t.Fatalf("fold=%v: word %q starts at offset %d of %q but is not a candidate",
							fold, w, i, input)
					}
				}
			}
		}
	}
}

// TestSecondByteIndexRejectsShortWords pins the constructor contract: the
// production scan loops assume every word needs two bytes, so a shorter word
// must fail loudly at build time rather than silently never matching.
func TestSecondByteIndexRejectsShortWords(t *testing.T) {
	for _, w := range []string{"", "a"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("NewSecondByteIndex accepted word %q, want panic", w)
				}
			}()
			NewSecondByteIndex([]string{w}, false)
		}()
	}
}

func TestSecondByteIndexLookup(t *testing.T) {
	idx := NewSecondByteIndex([]string{"ab", "ac"}, true)
	// A mask hit returns the whole first-byte candidate list ("ab" and "ac"
	// both start with 'a'); the caller's comparison picks the real match.
	if c := idx.Candidates('a', 'b'); !slices.Equal(c, []int{0, 1}) {
		t.Fatalf("Candidates('a','b') = %v, want [0 1]", c)
	}
	if c := idx.Candidates('A', 'C'); !slices.Equal(c, []int{0, 1}) {
		t.Fatalf("Candidates('A','C') = %v, want [0 1] (case-folded)", c)
	}
	// A mask miss rejects the position entirely.
	if c := idx.Candidates('a', 'z'); c != nil {
		t.Fatalf("Candidates('a','z') = %v, want nil", c)
	}
	if c := idx.Candidates('z', 'z'); c != nil {
		t.Fatalf("Candidates('z','z') = %v, want nil", c)
	}
}
