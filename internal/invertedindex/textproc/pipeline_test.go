package textproc

import (
	"reflect"
	"testing"
)

// ---------------------------------------------------------------------------
// Language detection
// ---------------------------------------------------------------------------

func TestDetectLang(t *testing.T) {
	tests := []struct {
		name string
		text string
		want Lang
	}{
		{"english", "The quick brown fox jumps over the lazy dog", LangEnglish},
		{"russian", "Быстрая коричневая лиса прыгает через ленивую собаку", LangRussian},
		{"mixed mostly english", "Hello world и привет", LangMixed},
		{"mixed mostly russian", "Привет мир and hello", LangMixed},
		{"empty", "", LangUnknown},
		{"numbers only", "12345 67890", LangUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectLang(tt.text)
			if got != tt.want {
				t.Errorf("DetectLang(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tokenizer
// ---------------------------------------------------------------------------

func TestUnicodeTokenizer(t *testing.T) {
	tok := NewUnicodeTokenizer()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"simple english", "hello world", []string{"hello", "world"}},
		{"with punctuation", "hello, world! How are you?", []string{"hello", "world", "How", "are", "you"}},
		{"russian", "Привет мир", []string{"Привет", "мир"}},
		{"mixed", "Hello Привет world мир", []string{"Hello", "Привет", "world", "мир"}},
		{"numbers", "version 3 is here", []string{"version", "3", "is", "here"}},
		{"empty", "", nil},
		{"only punctuation", "!@#$%", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tok.Tokenize(tt.input)
			got := extractTerms(tokens)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Tokenize(%q) terms = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTokenizerPositions(t *testing.T) {
	tok := NewUnicodeTokenizer()
	tokens := tok.Tokenize("hello beautiful world")

	for i, tok := range tokens {
		if tok.Position != i {
			t.Errorf("token %q position = %d, want %d", tok.Term, tok.Position, i)
		}
	}
}

// ---------------------------------------------------------------------------
// Normalizer
// ---------------------------------------------------------------------------

func TestUnicodeNormalizer(t *testing.T) {
	norm := NewUnicodeNormalizer()

	tokens := []Token{
		{Term: "Hello", Position: 0},
		{Term: "WORLD", Position: 1},
		{Term: "Привет", Position: 2},
	}

	result := norm.Normalize(tokens)
	got := extractTerms(result)
	want := []string{"hello", "world", "привет"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Normalize = %v, want %v", got, want)
	}
}

func TestNormalizerStripAccents(t *testing.T) {
	norm := NewUnicodeNormalizer(
		WithStripAccents(true),
	)

	tokens := []Token{
		{Term: "café", Position: 0},
		{Term: "naïve", Position: 1},
		{Term: "résumé", Position: 2},
	}

	result := norm.Normalize(tokens)
	got := extractTerms(result)
	want := []string{"cafe", "naive", "resume"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Normalize with accents = %v, want %v", got, want)
	}
}

func TestNormalizerEmptyTokens(t *testing.T) {
	norm := NewUnicodeNormalizer()

	tokens := []Token{
		{Term: "", Position: 0},
		{Term: "hello", Position: 1},
		{Term: "  ", Position: 2},
	}

	result := norm.Normalize(tokens)
	if len(result) != 1 || result[0].Term != "hello" {
		t.Errorf("expected only 'hello', got %v", result)
	}
}

// ---------------------------------------------------------------------------
// Stop word filter
// ---------------------------------------------------------------------------

func TestStopWordFilter(t *testing.T) {
	f := NewDictStopWordFilter()

	t.Run("english", func(t *testing.T) {
		tokens := []Token{
			{Term: "the", Position: 0},
			{Term: "quick", Position: 1},
			{Term: "brown", Position: 2},
			{Term: "fox", Position: 3},
			{Term: "is", Position: 4},
			{Term: "a", Position: 5},
			{Term: "animal", Position: 6},
		}

		result := f.Filter(tokens, LangEnglish)
		got := extractTerms(result)
		want := []string{"quick", "brown", "fox", "animal"}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("Filter English = %v, want %v", got, want)
		}
	})

	t.Run("russian", func(t *testing.T) {
		tokens := []Token{
			{Term: "это", Position: 0},
			{Term: "быстрая", Position: 1},
			{Term: "лиса", Position: 2},
			{Term: "и", Position: 3},
			{Term: "собака", Position: 4},
		}

		result := f.Filter(tokens, LangRussian)
		got := extractTerms(result)
		want := []string{"быстрая", "лиса", "собака"}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("Filter Russian = %v, want %v", got, want)
		}
	})

	t.Run("unknown language passes through", func(t *testing.T) {
		tokens := []Token{{Term: "hello", Position: 0}}
		result := f.Filter(tokens, LangUnknown)
		if len(result) != 1 {
			t.Errorf("expected passthrough for unknown lang, got %v", result)
		}
	})
}

func TestStopWordFilterCustomWords(t *testing.T) {
	f := NewDictStopWordFilter()
	f.AddCustomWords(LangEnglish, []string{"custom", "words"})

	if !f.IsStopWord("custom", LangEnglish) {
		t.Error("expected 'custom' to be a stop word")
	}
	if !f.IsStopWord("the", LangEnglish) {
		t.Error("expected 'the' to still be a stop word")
	}
}

// ---------------------------------------------------------------------------
// Stemmer
// ---------------------------------------------------------------------------

func TestSnowballStemmer(t *testing.T) {
	s := NewSnowballStemmer()

	t.Run("english", func(t *testing.T) {
		tests := []struct {
			input, want string
		}{
			{"running", "run"},
			{"computers", "comput"},
			{"dogs", "dog"},
		}
		for _, tt := range tests {
			got := s.Stem(tt.input, LangEnglish)
			if got != tt.want {
				t.Errorf("Stem(%q, english) = %q, want %q", tt.input, got, tt.want)
			}
		}
	})

	t.Run("russian", func(t *testing.T) {
		tests := []struct {
			input, want string
		}{
			{"документов", "документ"},
			{"быстрая", "быстр"},
		}
		for _, tt := range tests {
			got := s.Stem(tt.input, LangRussian)
			if got != tt.want {
				t.Errorf("Stem(%q, russian) = %q, want %q", tt.input, got, tt.want)
			}
		}
	})

	t.Run("unknown language passthrough", func(t *testing.T) {
		got := s.Stem("hello", LangUnknown)
		if got != "hello" {
			t.Errorf("expected passthrough, got %q", got)
		}
	})
}

// ---------------------------------------------------------------------------
// Full pipeline
// ---------------------------------------------------------------------------

func TestPipelineEnglish(t *testing.T) {
	p := NewPipeline()

	result := p.Process("The quick brown foxes are jumping over the lazy dogs")

	if result.Lang != LangEnglish {
		t.Errorf("lang = %q, want english", result.Lang)
	}

	// "the", "are", "over" should be removed as stop words.
	// "foxes" → "fox", "jumping" → "jump", "dogs" → "dog", etc.
	terms := result.Terms
	assertContains(t, terms, "quick")
	assertContains(t, terms, "brown")
	assertContains(t, terms, "fox")
	assertContains(t, terms, "jump")
	assertContains(t, terms, "lazi")
	assertContains(t, terms, "dog")
	assertNotContains(t, terms, "the")
	assertNotContains(t, terms, "are")
	assertNotContains(t, terms, "over")
}

func TestPipelineRussian(t *testing.T) {
	p := NewPipeline()

	result := p.Process("Быстрая коричневая лиса прыгает через ленивую собаку")

	if result.Lang != LangRussian {
		t.Errorf("lang = %q, want russian", result.Lang)
	}

	terms := result.Terms
	assertContains(t, terms, "быстр")
	assertContains(t, terms, "лис")
	assertContains(t, terms, "собак")
}

func TestPipelineWithForcedLanguage(t *testing.T) {
	p := NewPipeline(WithLanguage(LangRussian))

	result := p.Process("тестовый текст")
	if result.Lang != LangRussian {
		t.Errorf("lang = %q, want russian", result.Lang)
	}
}

func TestPipelineWithDisabledStages(t *testing.T) {
	p := NewPipeline(
		WithStopWordFilter(nil),
		WithStemmer(nil),
	)

	result := p.Process("The quick brown fox")

	// Without stop word removal, "the" should be present.
	assertContains(t, result.Terms, "the")
	// Without stemming, "quick" stays as "quick" (not "quick" → some stem).
	assertContains(t, result.Terms, "quick")
}

func TestPipelineProcessWithLang(t *testing.T) {
	p := NewPipeline()

	result := p.ProcessWithLang("running dogs", LangEnglish)

	if result.Lang != LangEnglish {
		t.Errorf("lang = %q, want english", result.Lang)
	}
	assertContains(t, result.Terms, "run")
	assertContains(t, result.Terms, "dog")
}

func TestPipelineEmptyInput(t *testing.T) {
	p := NewPipeline()

	result := p.Process("")
	if len(result.Terms) != 0 {
		t.Errorf("expected empty terms, got %v", result.Terms)
	}
}

func TestPipelineDeduplication(t *testing.T) {
	p := NewPipeline()

	// "running" and "runs" both stem to "run".
	result := p.ProcessWithLang("running runs runner", LangEnglish)

	count := 0
	for _, term := range result.Terms {
		if term == "run" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'run' once in terms, got %d times in %v", count, result.Terms)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractTerms(tokens []Token) []string {
	if len(tokens) == 0 {
		return nil
	}
	terms := make([]string, len(tokens))
	for i, tok := range tokens {
		terms[i] = tok.Term
	}
	return terms
}

func assertContains(t *testing.T, terms []string, want string) {
	t.Helper()
	for _, term := range terms {
		if term == want {
			return
		}
	}
	t.Errorf("terms %v should contain %q", terms, want)
}

func assertNotContains(t *testing.T, terms []string, unwanted string) {
	t.Helper()
	for _, term := range terms {
		if term == unwanted {
			t.Errorf("terms %v should NOT contain %q", terms, unwanted)
			return
		}
	}
}
