package textproc

import (
	"strings"
	"unicode"
)

type Token struct {
	Term     string
	Position int
}

// Tokenizer splits raw text into a stream of tokens.
type Tokenizer interface {
	Tokenize(text string) []Token
}

// UnicodeTokenizer splits text on non-letter/non-digit boundaries.
// It handles mixed-language text (Latin + Cyrillic) correctly by treating
// each contiguous run of letters/digits as a single token.
type UnicodeTokenizer struct{}

func NewUnicodeTokenizer() *UnicodeTokenizer {
	return &UnicodeTokenizer{}
}

func (t *UnicodeTokenizer) Tokenize(text string) []Token {
	var tokens []Token
	pos := 0

	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	for _, field := range fields {
		if field == "" {
			continue
		}
		tokens = append(tokens, Token{
			Term:     field,
			Position: pos,
		})
		pos++
	}

	return tokens
}
