package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"omolsm/internal/invertedindex/dictionary"
	"omolsm/internal/invertedindex/index"
	"omolsm/internal/invertedindex/textproc"

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

// NewEngine creates an engine with default config (memory backend).
func NewEngine() *Engine {
	cfg := DefaultConfig()
	e, _ := newEngineFromConfig(cfg)
	return e
}

// NewEngineFromConfigPath creates an engine from a YAML config file.
func NewEngineFromConfigPath(path string) (*Engine, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	return newEngineFromConfig(cfg)
}

func NewEngineFromCfg(cfg *Config) (*Engine, error) {
	return newEngineFromConfig(cfg)
}

func newEngineFromConfig(cfg *Config) (*Engine, error) {
	dict, idx, err := buildStorage(cfg)
	if err != nil {
		return nil, fmt.Errorf("build storage: %w", err)
	}

	return &Engine{
		conf:          cfg,
		pipeline:      buildPipeline(cfg, cfg.Index),
		queryPipeline: buildPipeline(cfg, cfg.Query),
		dict:          dict,
		idx:           idx,
	}, nil
}

// buildStorage creates the dictionary and inverted index based on config.
func buildStorage(cfg *Config) (dictionary.Dictionary, index.InvertedIndex, error) {
	backend := cfg.Storage.Backend
	if backend == "" {
		backend = "memory"
	}

	switch backend {
	case "memory":
		return dictionary.NewMemDictionary(), index.NewMemIndex(), nil

	case "lsm":
		dataDir := cfg.Storage.DataDir
		if dataDir == "" {
			dataDir = ".lsm-data"
		}

		dictDir := filepath.Join(dataDir, "dictionary")
		idxDir := filepath.Join(dataDir, "index")

		confOpts := cfg.Storage.ToLSMConfigOptions()

		dict, err := dictionary.NewLSMDictionaryFromConfig(dictDir, confOpts...)
		if err != nil {
			return nil, nil, fmt.Errorf("create lsm dictionary: %w", err)
		}

		idx, err := index.NewLSMIndexFromConfig(
			idxDir,
			[]index.LSMIndexOption{index.WithBlockSize(uint32(cfg.Storage.BlockSize))},
			confOpts...,
		)
		if err != nil {
			dict.Close()
			return nil, nil, fmt.Errorf("create lsm index: %w", err)
		}

		return dict, idx, nil

	default:
		return nil, nil, fmt.Errorf("unknown storage backend: %q", backend)
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

// Config returns the engine configuration.
func (e *Engine) Config() *Config {
	return e.conf
}

func (e *Engine) LSMStatsReport() string {
	if e.conf.Storage.Backend != "lsm" {
		return ""
	}

	var sb strings.Builder

	if lsmIdx, ok := e.idx.(*index.LSMIndex); ok {
		s := lsmIdx.GetTreeStats()
		sb.WriteString("  [Index LSM]\n")
		sb.WriteString(fmt.Sprintf("    Flushes      : %d\n", s.FlushCount))
		sb.WriteString(fmt.Sprintf("    Compactions  : %d\n", s.CompactCount))
		sb.WriteString(fmt.Sprintf("    Bytes written: %d\n", s.BytesWritten))
		sb.WriteString(fmt.Sprintf("    Bytes read   : %d\n", s.BytesRead))
		for i, cnt := range lsmIdx.SSTPerLevel() {
			if cnt > 0 {
				sb.WriteString(fmt.Sprintf("    Level %d SSTs : %d\n", i, cnt))
			}
		}
	}

	if lsmDict, ok := e.dict.(*dictionary.LSMDictionary); ok {
		s := lsmDict.GetTreeStats()
		sb.WriteString("  [Dictionary LSM]\n")
		sb.WriteString(fmt.Sprintf("    Flushes      : %d\n", s.FlushCount))
		sb.WriteString(fmt.Sprintf("    Compactions  : %d\n", s.CompactCount))
		sb.WriteString(fmt.Sprintf("    Bytes written: %d\n", s.BytesWritten))
		sb.WriteString(fmt.Sprintf("    Bytes read   : %d\n", s.BytesRead))
		for i, cnt := range lsmDict.SSTPerLevel() {
			if cnt > 0 {
				sb.WriteString(fmt.Sprintf("    Level %d SSTs : %d\n", i, cnt))
			}
		}
	}

	return sb.String()
}

// Close releases all resources (LSM temp directories, SST readers, etc).
func (e *Engine) Close() {
	if c, ok := e.dict.(interface{ Close() }); ok {
		c.Close()
	}
	if c, ok := e.idx.(interface{ Close() }); ok {
		c.Close()
	}
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
