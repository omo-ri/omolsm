package kgram

import "strings"

// Generate returns all k-grams for a term with boundary markers.
//
// Example (k=2): "hello" → ["$h", "he", "el", "ll", "lo", "o$"]
func Generate(term string, k int) []string {
	padded := "$" + term + "$"
	runes := []rune(padded)
	if len(runes) < k {
		return []string{padded}
	}
	grams := make([]string, 0, len(runes)-k+1)
	for i := 0; i <= len(runes)-k; i++ {
		grams = append(grams, string(runes[i:i+k]))
	}
	return grams
}

// ExtractFromPattern extracts k-grams from a wildcard pattern.
// It splits on '*' and generates k-grams from each contiguous segment,
// adding boundary markers only at the true start and end of the pattern.
//
// Example (k=2): "he*o" → ["$h", "he", "o$"]
// Example (k=2): "*ello" → ["el", "ll", "lo", "o$"]
// Example (k=2): "fox*"  → ["$f", "fo", "ox"]
func ExtractFromPattern(pattern string, k int) []string {
	parts := strings.Split(pattern, "*")
	seen := make(map[string]struct{})
	var grams []string

	for i, part := range parts {
		if part == "" {
			continue
		}

		// Add boundary markers only at true start/end of the whole pattern.
		padded := part
		if i == 0 {
			padded = "$" + padded
		}
		if i == len(parts)-1 {
			padded = padded + "$"
		}

		runes := []rune(padded)
		for j := 0; j <= len(runes)-k; j++ {
			gram := string(runes[j : j+k])
			if _, dup := seen[gram]; !dup {
				seen[gram] = struct{}{}
				grams = append(grams, gram)
			}
		}
	}
	return grams
}

// MatchPattern reports whether term matches a wildcard pattern.
// '*' matches zero or more characters; no other wildcards are supported.
func MatchPattern(term, pattern string) bool {
	return matchWildcard([]rune(term), []rune(pattern))
}

func matchWildcard(s, p []rune) bool {
	if len(p) == 0 {
		return len(s) == 0
	}

	if p[0] == '*' {
		// Skip consecutive wildcards.
		for len(p) > 0 && p[0] == '*' {
			p = p[1:]
		}
		// Try matching the remainder of p at every position in s.
		for i := 0; i <= len(s); i++ {
			if matchWildcard(s[i:], p) {
				return true
			}
		}
		return false
	}

	if len(s) == 0 || s[0] != p[0] {
		return false
	}
	return matchWildcard(s[1:], p[1:])
}
