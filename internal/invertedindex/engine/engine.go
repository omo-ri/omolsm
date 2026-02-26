package engine

import (
	"fmt"
	"omolsm/internal/invertedindex/dictionary"
	"omolsm/internal/invertedindex/index"
	"omolsm/internal/invertedindex/textproc"
	"os"
	"path/filepath"
	"strings"

	"github.com/RoaringBitmap/roaring"
)

// DocInfo stores metadata for an indexed document.
type DocInfo struct {
	ID       uint32
	Filename string
}

// Engine orchestrates text processing, dictionary, and inverted index.
type Engine struct {
	conf          *Config
	pipeline      *textproc.Pipeline
	queryPipeline *textproc.Pipeline
	dict          dictionary.Dictionary
	idx           index.InvertedIndex
	docs          []DocInfo
	nextID        uint32
}

// NewEngine creates an engine with default config.
func NewEngine() *Engine {
	cfg := DefaultConfig()
	return newEngineFromConfig(cfg)
}

// NewEngineFromConfig creates an engine from a YAML config file.
func NewEngineFromConfig(path string) (*Engine, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	return newEngineFromConfig(cfg), nil
}

func newEngineFromConfig(cfg *Config) *Engine {
	return &Engine{
		conf:          cfg,
		pipeline:      buildPipeline(cfg, cfg.Index),
		queryPipeline: buildPipeline(cfg, cfg.Query),
		dict:          dictionary.NewMemDictionary(),
		idx:           index.NewMemIndex(),
	}
}

// buildPipeline constructs a textproc.Pipeline from config.
func buildPipeline(cfg *Config, pc PipelineConfig) *textproc.Pipeline {
	var opts []textproc.PipelineOption

	// Language.
	switch cfg.Language {
	case "english":
		opts = append(opts, textproc.WithLanguage(textproc.LangEnglish))
	case "russian":
		opts = append(opts, textproc.WithLanguage(textproc.LangRussian))
	case "mixed":
		opts = append(opts, textproc.WithLanguage(textproc.LangMixed))
	default:
		opts = append(opts, textproc.WithAutoDetect())
	}

	// Normalizer.
	if pc.Normalize {
		opts = append(opts, textproc.WithNormalizer(textproc.NewUnicodeNormalizer(
			textproc.WithStripAccents(pc.StripAccents),
			textproc.WithMinTermLength(pc.MinTermLen),
		)))
	} else {
		opts = append(opts, textproc.WithNormalizer(nil))
	}

	// Stop words.
	if pc.StopWords {
		f := textproc.NewDictStopWordFilter()
		if len(cfg.CustomStopWords.English) > 0 {
			f.AddCustomWords(textproc.LangEnglish, cfg.CustomStopWords.English)
		}
		if len(cfg.CustomStopWords.Russian) > 0 {
			f.AddCustomWords(textproc.LangRussian, cfg.CustomStopWords.Russian)
		}
		opts = append(opts, textproc.WithStopWordFilter(f))
	} else {
		opts = append(opts, textproc.WithStopWordFilter(nil))
	}

	// Stemming.
	if pc.Stemming {
		opts = append(opts, textproc.WithStemmer(textproc.NewSnowballStemmer()))
	} else {
		opts = append(opts, textproc.WithStemmer(nil))
	}

	return textproc.NewPipeline(opts...)
}

// IndexDir scans the configured source directory for matching files and indexes each one.
func (e *Engine) IndexDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !e.matchExtension(entry.Name()) {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		if err := e.IndexFile(filePath); err != nil {
			return fmt.Errorf("index file %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// IndexSource indexes files from the configured source.dir.
func (e *Engine) IndexSource() error {
	return e.IndexDir(e.conf.Source.Dir)
}

// IndexFile reads a single file and adds it to the index.
func (e *Engine) IndexFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	text := string(data)
	docID := e.nextID
	e.nextID++

	e.docs = append(e.docs, DocInfo{
		ID:       docID,
		Filename: filepath.Base(filePath),
	})

	result := e.pipeline.Process(text)
	featureIDs := e.dict.GetOrAddTerms(result.Terms)
	e.idx.Add(docID, featureIDs)

	return nil
}

// Search queries one or more terms (AND logic) and returns a chainable Result.
func (e *Engine) Search(terms ...string) *Result {
	if len(terms) == 0 {
		return newResult(roaring.New(), e)
	}

	featureIDs := e.resolveTerms(terms)
	if featureIDs == nil {
		return newResult(roaring.New(), e)
	}

	if len(featureIDs) == 1 {
		return newResult(e.idx.GetPostingList(featureIDs[0]), e)
	}

	return newResult(e.idx.And(featureIDs), e)
}

// Stats returns basic statistics about the index.
func (e *Engine) Stats() Stats {
	return Stats{
		DocCount:     uint32(len(e.docs)),
		TermCount:    e.dict.Size(),
		FeatureCount: e.idx.FeatureCount(),
	}
}

type Stats struct {
	DocCount     uint32
	TermCount    int
	FeatureCount int
}

func (s Stats) String() string {
	return fmt.Sprintf("docs: %d, terms: %d, features: %d", s.DocCount, s.TermCount, s.FeatureCount)
}

// GetDoc returns document info by docID.
func (e *Engine) GetDoc(docID uint32) (DocInfo, bool) {
	if int(docID) >= len(e.docs) {
		return DocInfo{}, false
	}
	return e.docs[docID], true
}

func (e *Engine) matchExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	for _, allowed := range e.conf.Source.Extensions {
		if ext == allowed {
			return true
		}
	}
	return false
}

func (e *Engine) resolveTerms(rawTerms []string) []uint32 {
	var featureIDs []uint32
	for _, raw := range rawTerms {
		result := e.queryPipeline.Process(raw)
		for _, term := range result.Terms {
			id, ok := e.dict.Get(term)
			if !ok {
				continue
			}
			featureIDs = append(featureIDs, id)
		}
	}
	if len(featureIDs) == 0 {
		return nil
	}
	return featureIDs
}

func (e *Engine) bitmapToDocs(bm interface{ ToArray() []uint32 }) []DocInfo {
	docIDs := bm.ToArray()
	if len(docIDs) == 0 {
		return nil
	}

	docs := make([]DocInfo, 0, len(docIDs))
	for _, id := range docIDs {
		if int(id) < len(e.docs) {
			docs = append(docs, e.docs[id])
		}
	}
	return docs
}
