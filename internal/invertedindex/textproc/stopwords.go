package textproc

// StopWordFilter removes common words that carry little semantic meaning.
type StopWordFilter interface {
	// IsStopWord returns true if the term should be filtered out.
	IsStopWord(term string, lang Lang) bool
	// Filter removes stop words from a token stream.
	Filter(tokens []Token, lang Lang) []Token
}

// DictStopWordFilter uses per-language dictionaries of stop words.
type DictStopWordFilter struct {
	dicts map[Lang]map[string]struct{}
}

func NewDictStopWordFilter() *DictStopWordFilter {
	f := &DictStopWordFilter{
		dicts: make(map[Lang]map[string]struct{}),
	}
	f.dicts[LangEnglish] = toSet(englishStopWords)
	f.dicts[LangRussian] = toSet(russianStopWords)
	return f
}

// AddCustomWords allows adding extra stop words for a specific language.
func (f *DictStopWordFilter) AddCustomWords(lang Lang, words []string) {
	dict, ok := f.dicts[lang]
	if !ok {
		dict = make(map[string]struct{})
		f.dicts[lang] = dict
	}
	for _, w := range words {
		dict[w] = struct{}{}
	}
}

func (f *DictStopWordFilter) IsStopWord(term string, lang Lang) bool {
	dict, ok := f.dicts[lang]
	if !ok {
		return false
	}
	_, found := dict[term]
	return found
}

func (f *DictStopWordFilter) Filter(tokens []Token, lang Lang) []Token {
	result := make([]Token, 0, len(tokens))
	for _, tok := range tokens {
		tokenLang := DetectTokenLang(tok.Term)
		if !f.IsStopWord(tok.Term, tokenLang) {
			result = append(result, tok)
		}
	}
	return result
}

func toSet(words []string) map[string]struct{} {
	s := make(map[string]struct{}, len(words))
	for _, w := range words {
		s[w] = struct{}{}
	}
	return s
}

// ---------------------------------------------------------------------------
// Built-in stop word lists
// ---------------------------------------------------------------------------

// Source: NLTK English stop words (most common subset).
var englishStopWords = []string{
	"a", "an", "the", "and", "or", "but", "not", "no",
	"is", "are", "was", "were", "am", "be", "been", "being",
	"have", "has", "had", "having",
	"do", "does", "did", "doing",
	"will", "would", "shall", "should", "can", "could", "may", "might", "must",
	"i", "me", "my", "myself", "we", "our", "ours", "ourselves",
	"you", "your", "yours", "yourself", "yourselves",
	"he", "him", "his", "himself",
	"she", "her", "hers", "herself",
	"it", "its", "itself",
	"they", "them", "their", "theirs", "themselves",
	"what", "which", "who", "whom", "this", "that", "these", "those",
	"if", "then", "else", "when", "where", "why", "how",
	"all", "each", "every", "both", "few", "more", "most",
	"other", "some", "such", "only", "own", "same", "so", "than", "too", "very",
	"in", "on", "at", "to", "for", "of", "with", "by", "from",
	"up", "about", "into", "over", "after", "before", "between", "under", "through",
	"during", "above", "below", "out", "off", "again", "further",
	"here", "there", "once",
	"just", "also", "now",
}

// Source: Common Russian stop words (MyStem / NLTK Russian).
var russianStopWords = []string{
	"и", "в", "во", "не", "что", "он", "на", "я", "с", "со",
	"как", "а", "то", "все", "она", "так", "его", "но", "да", "ты",
	"к", "у", "же", "вы", "за", "бы", "по", "только", "её", "ее",
	"мне", "было", "вот", "от", "меня", "ещё", "еще", "нет", "о", "из",
	"ему", "теперь", "когда", "даже", "ну", "вдруг", "ли", "если", "уже", "или",
	"ни", "быть", "был", "него", "до", "вас", "нибудь", "опять", "уж",
	"вам", "ведь", "там", "потом", "себя", "ничего", "ей", "может", "они",
	"тут", "где", "есть", "надо", "ней", "для", "мы", "тебя", "их", "чем",
	"была", "сам", "чтоб", "без", "будто", "чего", "раз", "тоже", "себе",
	"под", "будет", "ж", "тогда", "кто", "этот", "того", "потому", "этого",
	"какой", "совсем", "ним", "здесь", "этом", "один", "почти", "мой", "тем",
	"чтобы", "нее", "сейчас", "были", "куда", "зачем", "всех", "никогда",
	"можно", "при", "наконец", "два", "об", "другой", "хоть", "после",
	"над", "больше", "тот", "через", "эти", "нас", "про", "всего",
	"них", "какая", "много", "разве", "три", "эту", "моя", "свою",
	"этой", "перед", "иногда", "лучше", "чуть", "том", "нельзя",
	"такой", "им", "более", "всегда", "конечно", "всю", "между",
	"это", "мой", "свой", "весь",
}
