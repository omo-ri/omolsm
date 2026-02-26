package textproc

// ProcessResult contains the output of the text processing pipeline.
type ProcessResult struct {
	Lang   Lang     // Detected or specified language.
	Terms  []string // Final list of processed terms (deduplicated, stemmed).
	Tokens []Token  // Full token stream with positions (for phrase queries, etc.).
}

// Pipeline orchestrates the text processing stages.
// Each stage is optional and can be replaced with a custom implementation.
type Pipeline struct {
	tokenizer  Tokenizer
	normalizer Normalizer
	stopFilter StopWordFilter
	stemmer    Stemmer
	langDetect bool // If true, auto-detect language; otherwise use forcedLang.
	forcedLang Lang
}

// PipelineOption configures a Pipeline.
type PipelineOption func(*Pipeline)

// NewPipeline creates a Pipeline with sensible defaults:
//   - UnicodeTokenizer
//   - UnicodeNormalizer (lowercase, NFC, no accent stripping)
//   - DictStopWordFilter (built-in English + Russian)
//   - SnowballStemmer
//   - Auto language detection enabled
func NewPipeline(opts ...PipelineOption) *Pipeline {
	p := &Pipeline{
		tokenizer:  NewUnicodeTokenizer(),
		normalizer: NewUnicodeNormalizer(),
		stopFilter: NewDictStopWordFilter(),
		stemmer:    NewSnowballStemmer(),
		langDetect: true,
	}

	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Process runs the full pipeline on raw text and returns the result.
func (p *Pipeline) Process(text string) *ProcessResult {
	// 1. Detect language.
	lang := p.forcedLang
	if p.langDetect {
		lang = DetectLang(text)
	}
	if lang == "" || lang == LangUnknown {
		lang = LangEnglish // Fallback.
	}

	// 2. Tokenize.
	tokens := p.tokenizer.Tokenize(text)

	// 3. Normalize.
	if p.normalizer != nil {
		tokens = p.normalizer.Normalize(tokens)
	}

	// 4. Remove stop words.
	if p.stopFilter != nil {
		tokens = p.stopFilter.Filter(tokens, lang)
	}

	// 5. Stem.
	if p.stemmer != nil {
		tokens = p.stemmer.StemTokens(tokens, lang)
	}

	// 6. Extract unique terms.
	terms := uniqueTerms(tokens)

	return &ProcessResult{
		Lang:   lang,
		Terms:  terms,
		Tokens: tokens,
	}
}

// ProcessWithLang runs the pipeline with a specific language (skips detection).
func (p *Pipeline) ProcessWithLang(text string, lang Lang) *ProcessResult {
	tokens := p.tokenizer.Tokenize(text)

	if p.normalizer != nil {
		tokens = p.normalizer.Normalize(tokens)
	}
	if p.stopFilter != nil {
		tokens = p.stopFilter.Filter(tokens, lang)
	}
	if p.stemmer != nil {
		tokens = p.stemmer.StemTokens(tokens, lang)
	}

	return &ProcessResult{
		Lang:   lang,
		Terms:  uniqueTerms(tokens),
		Tokens: tokens,
	}
}

// uniqueTerms extracts deduplicated terms from tokens while preserving order.
func uniqueTerms(tokens []Token) []string {
	seen := make(map[string]struct{}, len(tokens))
	terms := make([]string, 0, len(tokens))

	for _, tok := range tokens {
		if _, ok := seen[tok.Term]; ok {
			continue
		}
		seen[tok.Term] = struct{}{}
		terms = append(terms, tok.Term)
	}
	return terms
}

// ---------------------------------------------------------------------------
// Pipeline options
// ---------------------------------------------------------------------------

// WithTokenizer replaces the default tokenizer.
func WithTokenizer(t Tokenizer) PipelineOption {
	return func(p *Pipeline) { p.tokenizer = t }
}

// WithNormalizer replaces the default normalizer. Pass nil to disable.
func WithNormalizer(n Normalizer) PipelineOption {
	return func(p *Pipeline) { p.normalizer = n }
}

// WithStopWordFilter replaces the default stop word filter. Pass nil to disable.
func WithStopWordFilter(f StopWordFilter) PipelineOption {
	return func(p *Pipeline) { p.stopFilter = f }
}

// WithStemmer replaces the default stemmer. Pass nil to disable.
func WithStemmer(s Stemmer) PipelineOption {
	return func(p *Pipeline) { p.stemmer = s }
}

// WithLanguage forces a specific language and disables auto-detection.
func WithLanguage(lang Lang) PipelineOption {
	return func(p *Pipeline) {
		p.forcedLang = lang
		p.langDetect = false
	}
}

// WithAutoDetect enables automatic language detection (default).
func WithAutoDetect() PipelineOption {
	return func(p *Pipeline) {
		p.langDetect = true
		p.forcedLang = ""
	}
}
