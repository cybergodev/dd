package internal

// This file implements the two-byte pre-rejection index shared by the
// sensitive-data feature scan (features.go) and the credential-keyword
// pre-gate in the root package (security.go). Both scans walk every byte of
// every message; pprof showed the per-position candidate comparisons were the
// dominant CPU cost of the logging hot path once patterns were gated.
//
// The index turns "which words could start at this position" into one mask
// test: Candidates(first, second) returns nil unless some indexed word begins
// with exactly those two bytes, so the common position (a letter followed by
// an unrelated letter) rejects the whole candidate list without touching it.
//
// SECURITY: the mask is only allowed to REJECT positions where no indexed
// word's two-byte prefix can match. It never selects or filters candidates
// beyond the prefix, so callers' existing full-word comparison remains the
// authority — a mask bug can only skip work, never a match... provided every
// word is at least two bytes long (panics in NewSecondByteIndex otherwise).

// SecondByteIndex indexes a fixed word list for single-pass substring
// scanning. Words must be non-empty and at least two bytes long.
type SecondByteIndex struct {
	words    []string
	caseFold bool
	// byFirst maps a first byte to the indices of words starting with it.
	// For caseFold indexes, both cases of each word's first byte are
	// registered (whatever case the word itself uses), and both the mask
	// bits and the input's second byte are lowercased so mixed-case input
	// folds onto the words.
	byFirst [256][]int
	// secondMask[first] has bit b set iff some word starting with first has
	// (case-folded) byte b as its second byte.
	secondMask [256]mask256
}

// mask256 is a 256-bit set keyed by byte value.
type mask256 [4]uint64

func (m *mask256) set(c byte)      { m[c>>6] |= 1 << (c & 63) }
func (m *mask256) has(c byte) bool { return m[c>>6]&(1<<(c&63)) != 0 }

// NewSecondByteIndex builds the index for words. It panics on an empty or
// one-byte word: the scan loop never tests the final input position, which is
// sound only when every word needs at least two bytes.
func NewSecondByteIndex(words []string, caseFold bool) *SecondByteIndex {
	x := &SecondByteIndex{words: words, caseFold: caseFold}
	for i, w := range words {
		if len(w) < 2 {
			panic("internal: NewSecondByteIndex word shorter than 2 bytes: " + w)
		}
		second := w[1]
		if caseFold {
			second = lowerASCII(second)
		}
		for _, first := range firstByteVariants(w[0], caseFold) {
			x.byFirst[first] = append(x.byFirst[first], i)
			x.secondMask[first].set(second)
		}
	}
	return x
}

// firstByteVariants returns the byte positions a word's first byte must be
// registered under: both ASCII cases when folding, the byte itself otherwise.
func firstByteVariants(first byte, caseFold bool) []byte {
	variants := []byte{first}
	if caseFold {
		if lower := lowerASCII(first); lower != first {
			variants = append(variants, lower)
		}
		if upper := upperASCII(first); upper != first {
			variants = append(variants, upper)
		}
	}
	return variants
}

// Candidates returns the words indexed under first, or nil when no indexed
// word's two-byte prefix can match (first, second) — raw input bytes; a
// caseFold index folds them itself. Returned words share the first byte but
// may differ in the second; the caller's full-word comparison remains the
// authority for which candidate, if any, actually matches.
func (x *SecondByteIndex) Candidates(first, second byte) []int {
	idx := x.byFirst[first]
	if len(idx) == 0 {
		return nil
	}
	if x.caseFold {
		second = lowerASCII(second)
	}
	if !x.secondMask[first].has(second) {
		return nil
	}
	return idx
}
