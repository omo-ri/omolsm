package textproc

import (
	"unicode"
)

// Lang represents a supported language.
type Lang string

const (
	LangEnglish Lang = "english"
	LangRussian Lang = "russian"
	LangMixed   Lang = "mixed"
	LangUnknown Lang = "unknown"
)

// DetectLang performs heuristic language detection based on character distribution.
// Returns LangMixed if both Latin and Cyrillic characters are present in significant amounts.
func DetectLang(text string) Lang {
	var latinCount, cyrillicCount int

	for _, r := range text {
		if !unicode.IsLetter(r) {
			continue
		}
		if unicode.Is(unicode.Cyrillic, r) {
			cyrillicCount++
		} else if unicode.Is(unicode.Latin, r) {
			latinCount++
		}
	}

	total := latinCount + cyrillicCount
	if total == 0 {
		return LangUnknown
	}

	latinRatio := float64(latinCount) / float64(total)
	cyrillicRatio := float64(cyrillicCount) / float64(total)

	// If both scripts exceed 20%, it's a mixed document.
	if latinRatio > 0.2 && cyrillicRatio > 0.2 {
		return LangMixed
	}

	if cyrillicRatio > 0.5 {
		return LangRussian
	}
	return LangEnglish
}

// DetectTokenLang determines the language of a single token by its characters.
func DetectTokenLang(term string) Lang {
	for _, r := range term {
		if unicode.Is(unicode.Cyrillic, r) {
			return LangRussian
		}
	}
	return LangEnglish
}
