package textproc

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Normalizer transforms tokens into a canonical form.
type Normalizer interface {
	Normalize(tokens []Token) []Token
}

// UnicodeNormalizer applies:
//  1. Unicode NFC normalization (compose characters).
//  2. Lowercase conversion.
//  3. Optional accent/diacritical mark stripping (for Latin text).
//  4. Optional minimum term length filtering.
//  5. Removal of tokens that become empty after normalization.
type UnicodeNormalizer struct {
	stripAccents bool
	minTermLen   int
}

type NormalizerOption func(*UnicodeNormalizer)

func WithStripAccents(enabled ...bool) NormalizerOption {
	return func(n *UnicodeNormalizer) {
		if len(enabled) == 0 {
			n.stripAccents = true
		} else {
			n.stripAccents = enabled[0]
		}
	}
}

func WithMinTermLength(minLen int) NormalizerOption {
	return func(n *UnicodeNormalizer) {
		n.minTermLen = minLen
	}
}

func NewUnicodeNormalizer(opts ...NormalizerOption) *UnicodeNormalizer {
	n := &UnicodeNormalizer{
		minTermLen: 1,
	}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

func (n *UnicodeNormalizer) Normalize(tokens []Token) []Token {
	result := make([]Token, 0, len(tokens))

	for _, tok := range tokens {
		// Step 1: NFC normalization.
		normalized := norm.NFC.String(tok.Term)

		// Step 2: Lowercase.
		normalized = strings.ToLower(normalized)

		// Step 3: Optionally strip accents.
		if n.stripAccents {
			normalized = stripDiacritics(normalized)
		}

		// Step 4: Trim and skip empty.
		normalized = strings.TrimSpace(normalized)
		if normalized == "" {
			continue
		}

		// Step 5: Min term length filter.
		if len([]rune(normalized)) < n.minTermLen {
			continue
		}

		result = append(result, Token{
			Term:     normalized,
			Position: tok.Position,
		})
	}

	return result
}

// stripDiacritics removes combining diacritical marks from a string.
func stripDiacritics(s string) string {
	decomposed := norm.NFD.String(s)

	var b strings.Builder
	b.Grow(len(decomposed))

	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}

	return norm.NFC.String(b.String())
}
