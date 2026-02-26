package textproc

import (
	"github.com/kljensen/snowball"
)

// Stemmer reduces words to their root/stem form.
type Stemmer interface {
	// Stem returns the stemmed form of a single term.
	Stem(term string, lang Lang) string
	// StemTokens applies stemming to a token stream.
	StemTokens(tokens []Token, lang Lang) []Token
}

// SnowballStemmer adapts the Snowball stemming library.
// For mixed-language documents, it detects the language of each individual token
// based on its characters, rather than relying solely on the document-level language.
type SnowballStemmer struct{}

func NewSnowballStemmer() *SnowballStemmer {
	return &SnowballStemmer{}
}

func (s *SnowballStemmer) Stem(term string, lang Lang) string {
	if lang == LangUnknown {
		return term
	}

	stemmed, err := snowball.Stem(term, string(lang), true)
	if err != nil {
		return term
	}
	return stemmed
}

func (s *SnowballStemmer) StemTokens(tokens []Token, lang Lang) []Token {
	result := make([]Token, 0, len(tokens))
	for _, tok := range tokens {
		tokenLang := DetectTokenLang(tok.Term)
		stemmed := s.Stem(tok.Term, tokenLang)
		if stemmed == "" {
			continue
		}
		result = append(result, Token{
			Term:     stemmed,
			Position: tok.Position,
		})
	}
	return result
}
