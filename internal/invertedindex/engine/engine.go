package engine

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"omolsm/internal/invertedindex/dateindex"
	"omolsm/internal/invertedindex/dictionary"
	"omolsm/internal/invertedindex/index"
	"omolsm/internal/invertedindex/kgram"
	"omolsm/internal/invertedindex/textproc"

	"github.com/RoaringBitmap/roaring"
	"gopkg.in/yaml.v3"
)

// DateMeta carries optional date metadata for a document.
type DateMeta struct {
	Date      *time.Time // Requirement A: single date attribute
	StartDate *time.Time // Requirement B: validity start
	EndDate   *time.Time // Requirement B: validity end (nil = forever valid)
}

// DocInfo stores metadata for an indexed document.
type DocInfo struct {
	ID        uint32
	Filename  string
	Date      *time.Time // Requirement A
	StartDate *time.Time // Requirement B
	EndDate   *time.Time // Requirement B (nil = forever valid)
}

// Engine orchestrates text processing, dictionary, and inverted index.
type Engine struct {
	conf          *Config
	pipeline      *textproc.Pipeline
	queryPipeline *textproc.Pipeline
	dict          dictionary.Dictionary
	idx           index.InvertedIndex
	kgramIdx      kgram.Index
	docs          []DocInfo
	nextID        uint32

	// Date indexes
	dateIdx      *dateindex.DateIndex // Requirement A: single date
	startDateIdx *dateindex.DateIndex // Requirement B: start date
	endDateIdx   *dateindex.DateIndex // Requirement B: end date
	openEndDocs  *roaring.Bitmap      // docIDs with nil EndDate (forever valid)
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

	kgramIdx, err := buildKgramIndex(cfg)
	if err != nil {
		return nil, fmt.Errorf("build kgram index: %w", err)
	}

	return &Engine{
		conf:          cfg,
		pipeline:      buildPipeline(cfg, cfg.Index),
		queryPipeline: buildPipeline(cfg, cfg.Query),
		dict:          dict,
		idx:           idx,
		kgramIdx:      kgramIdx,
		dateIdx:       dateindex.New(),
		startDateIdx:  dateindex.New(),
		endDateIdx:    dateindex.New(),
		openEndDocs:   roaring.New(),
	}, nil
}

// buildKgramIndex creates a k-gram index matching the configured backend.
func buildKgramIndex(cfg *Config) (kgram.Index, error) {
	const k = 2
	backend := cfg.Storage.Backend
	if backend == "" || backend == "memory" {
		return kgram.NewMemKgramIndex(k), nil
	}
	// lsm backend
	dataDir := cfg.Storage.DataDir
	if dataDir == "" {
		dataDir = ".lsm-data"
	}
	kgramDir := filepath.Join(dataDir, "kgram")
	return kgram.NewLSMKgramIndex(kgramDir, k, cfg.Storage.ToLSMConfigOptions()...)
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
// If a metadata.yaml file exists in the directory, it will be used to provide date metadata.
func (e *Engine) IndexDir(dir string) error {
	metaMap := loadMetadataFile(dir)

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
		meta := metaMap[entry.Name()]
		if err := e.IndexFileWithDates(filePath, meta); err != nil {
			return fmt.Errorf("index file %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// IndexSource indexes files from the configured source.dir.
func (e *Engine) IndexSource() error {
	return e.IndexDir(e.conf.Source.Dir)
}

// metadataEntry is the YAML structure for per-file date metadata.
type metadataEntry struct {
	Date      string `yaml:"date"`       // "YYYY-MM-DD"
	StartDate string `yaml:"start_date"` // "YYYY-MM-DD"
	EndDate   string `yaml:"end_date"`   // "YYYY-MM-DD" or empty = forever valid
}

// loadMetadataFile loads metadata.yaml from dir, returning a map of filename → DateMeta.
// Returns an empty map if the file doesn't exist.
func loadMetadataFile(dir string) map[string]DateMeta {
	result := make(map[string]DateMeta)

	data, err := os.ReadFile(filepath.Join(dir, "metadata.yaml"))
	if err != nil {
		return result
	}

	var raw map[string]metadataEntry
	if err := yaml.Unmarshal(data, &raw); err != nil {
		log.Printf("Warning: failed to parse metadata.yaml: %v", err)
		return result
	}

	for filename, entry := range raw {
		var meta DateMeta
		if entry.Date != "" {
			if t, err := time.Parse("2006-01-02", entry.Date); err == nil {
				meta.Date = &t
			}
		}
		if entry.StartDate != "" {
			if t, err := time.Parse("2006-01-02", entry.StartDate); err == nil {
				meta.StartDate = &t
			}
		}
		if entry.EndDate != "" {
			if t, err := time.Parse("2006-01-02", entry.EndDate); err == nil {
				meta.EndDate = &t
			}
		}
		result[filename] = meta
	}

	return result
}

// IndexFile reads a single file and adds it to the index (no date metadata).
func (e *Engine) IndexFile(filePath string) error {
	return e.IndexFileWithDates(filePath, DateMeta{})
}

// IndexFileWithDates reads a single file and adds it to the index with date metadata.
func (e *Engine) IndexFileWithDates(filePath string, meta DateMeta) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	text := string(data)
	docID := e.nextID
	e.nextID++

	doc := DocInfo{
		ID:        docID,
		Filename:  filepath.Base(filePath),
		Date:      meta.Date,
		StartDate: meta.StartDate,
		EndDate:   meta.EndDate,
	}
	e.docs = append(e.docs, doc)

	// Text index
	result := e.pipeline.Process(text)
	featureIDs := e.dict.GetOrAddTerms(result.Terms)
	e.idx.Add(docID, featureIDs)

	for i, term := range result.Terms {
		e.kgramIdx.AddTerm(featureIDs[i], term)
	}

	// Date indexes
	if meta.Date != nil {
		e.dateIdx.Add(dateindex.DayFromTime(*meta.Date), docID)
	}
	if meta.StartDate != nil {
		e.startDateIdx.Add(dateindex.DayFromTime(*meta.StartDate), docID)
	}
	if meta.EndDate != nil {
		e.endDateIdx.Add(dateindex.DayFromTime(*meta.EndDate), docID)
	} else if meta.StartDate != nil {
		// Only track open-end if the document participates in the dual-date model
		e.openEndDocs.Add(docID)
	}

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

// SearchPrefix returns documents containing any term that starts with the given prefix.
// The prefix is lowercased before lookup; stemming is NOT applied.
// Example: SearchPrefix("fox") matches "fox", "foxes", "foxhound", …
func (e *Engine) SearchPrefix(prefix string) *Result {
	prefix = strings.ToLower(prefix)
	ids, err := e.dict.ScanPrefix(prefix)
	if err != nil || len(ids) == 0 {
		return newResult(roaring.New(), e)
	}
	bm := roaring.New()
	for _, id := range ids {
		bm.Or(e.idx.GetPostingList(id))
	}
	return newResult(bm, e)
}

// SearchWildcard returns documents containing any term matching the wildcard pattern.
// '*' matches zero or more characters. Uses the k-gram index for candidate lookup,
// then post-filters with exact pattern matching.
// Example: SearchWildcard("he*o") matches "hello", "hero", "hetero", …
func (e *Engine) SearchWildcard(pattern string) *Result {
	pattern = strings.ToLower(pattern)
	featureIDs := e.kgramIdx.Search(pattern)
	if len(featureIDs) == 0 {
		return newResult(roaring.New(), e)
	}
	bm := roaring.New()
	for _, id := range featureIDs {
		bm.Or(e.idx.GetPostingList(id))
	}
	return newResult(bm, e)
}

// SearchDateRange returns docs whose Date is in [from, to]. (Requirement A)
func (e *Engine) SearchDateRange(from, to string) (*Result, error) {
	fromDay, err := dateindex.DayFromString(from)
	if err != nil {
		return nil, fmt.Errorf("bad 'from' date %q: %w", from, err)
	}
	toDay, err := dateindex.DayFromString(to)
	if err != nil {
		return nil, fmt.Errorf("bad 'to' date %q: %w", to, err)
	}
	return newResult(e.dateIdx.Range(fromDay, toDay), e), nil
}

// SearchValidInRange returns docs valid at any point in [from, to]. (Requirement B)
// A document is valid if: startDate <= qTo AND (endDate >= qFrom OR endDate is nil).
func (e *Engine) SearchValidInRange(from, to string) (*Result, error) {
	fromDay, err := dateindex.DayFromString(from)
	if err != nil {
		return nil, fmt.Errorf("bad 'from' date %q: %w", from, err)
	}
	toDay, err := dateindex.DayFromString(to)
	if err != nil {
		return nil, fmt.Errorf("bad 'to' date %q: %w", to, err)
	}
	startOK := e.startDateIdx.LeBitmap(toDay) // startDate <= qTo
	endOK := e.endDateIdx.GeBitmap(fromDay)   // endDate >= qFrom
	endOK.Or(e.openEndDocs)                   // ...or endDate is nil
	startOK.And(endOK)
	return newResult(startOK, e), nil
}

// SearchAppearedInRange returns docs whose StartDate is in [from, to]. (Requirement B)
func (e *Engine) SearchAppearedInRange(from, to string) (*Result, error) {
	fromDay, err := dateindex.DayFromString(from)
	if err != nil {
		return nil, fmt.Errorf("bad 'from' date %q: %w", from, err)
	}
	toDay, err := dateindex.DayFromString(to)
	if err != nil {
		return nil, fmt.Errorf("bad 'to' date %q: %w", to, err)
	}
	return newResult(e.startDateIdx.Range(fromDay, toDay), e), nil
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
	if c, ok := e.kgramIdx.(interface{ Close() }); ok {
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
