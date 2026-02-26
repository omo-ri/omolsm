package engine

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestEnv creates a temp dir with test docs + a YAML config pointing to it.
// Returns the config file path.
func setupTestEnv(t *testing.T, yamlOverride string) string {
	t.Helper()
	dir := t.TempDir()

	// Create documents directory.
	docDir := filepath.Join(dir, "documents")
	os.MkdirAll(docDir, 0755)

	files := map[string]string{
		"doc0.txt": "The quick brown fox jumps over the lazy dog",
		"doc1.txt": "A fast brown cat sleeps on the warm mat with the dog",
		"doc2.txt": "The fox and the bird flew over the hill",
		"doc3.txt": "The cat and the bird saw a snake in the garden",
		"doc4.txt": "Быстрая коричневая лиса прыгает через ленивую собаку",
		"skip.csv": "this,should,be,ignored",
	}
	for name, content := range files {
		os.WriteFile(filepath.Join(docDir, name), []byte(content), 0644)
	}

	// Default YAML if no override.
	if yamlOverride == "" {
		yamlOverride = `
source:
  dir: "` + docDir + `"
  encoding: "utf-8"
  extensions:
    - ".txt"

language: "auto"

index:
  stop_words: true
  stemming: true
  strip_accents: false
  normalize: true
  min_term_length: 1

query:
  stop_words: false
  stemming: true
  strip_accents: false
  normalize: true
  min_term_length: 1
`
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(yamlOverride), 0644)

	return cfgPath
}

func loadAndIndex(t *testing.T, cfgPath string) *Engine {
	t.Helper()
	e, err := NewEngineFromConfig(cfgPath)
	if err != nil {
		t.Fatalf("NewEngineFromConfig: %v", err)
	}
	if err := e.IndexSource(); err != nil {
		t.Fatalf("IndexSource: %v", err)
	}
	return e
}

// ---------------------------------------------------------------------------
// Basic indexing
// ---------------------------------------------------------------------------

func TestConfigIndexing(t *testing.T) {
	cfgPath := setupTestEnv(t, "")
	e := loadAndIndex(t, cfgPath)

	stats := e.Stats()
	if stats.DocCount != 5 {
		t.Errorf("DocCount = %d, want 5 (csv should be skipped)", stats.DocCount)
	}
	if stats.TermCount == 0 {
		t.Error("TermCount should be > 0")
	}
}

// ---------------------------------------------------------------------------
// Search + chaining
// ---------------------------------------------------------------------------

func TestConfigSearch(t *testing.T) {
	cfgPath := setupTestEnv(t, "")
	e := loadAndIndex(t, cfgPath)

	docs := e.Search("fox").Documents()
	assertHasFile(t, docs, "doc0.txt")
	assertHasFile(t, docs, "doc2.txt")
}

func TestConfigSearchMultiTerm(t *testing.T) {
	cfgPath := setupTestEnv(t, "")
	e := loadAndIndex(t, cfgPath)

	docs := e.Search("fox", "dog").Documents()
	assertHasFile(t, docs, "doc0.txt")
	assertNotHasFile(t, docs, "doc2.txt")
}

func TestConfigAnd(t *testing.T) {
	cfgPath := setupTestEnv(t, "")
	e := loadAndIndex(t, cfgPath)

	docs := e.Search("fox").And("dog").Documents()
	assertHasFile(t, docs, "doc0.txt")
	assertNotHasFile(t, docs, "doc2.txt")
}

func TestConfigOr(t *testing.T) {
	cfgPath := setupTestEnv(t, "")
	e := loadAndIndex(t, cfgPath)

	docs := e.Search("fox").Or("cat").Documents()
	assertHasFile(t, docs, "doc0.txt")
	assertHasFile(t, docs, "doc1.txt")
	assertHasFile(t, docs, "doc2.txt")
	assertHasFile(t, docs, "doc3.txt")
}

func TestConfigNot(t *testing.T) {
	cfgPath := setupTestEnv(t, "")
	e := loadAndIndex(t, cfgPath)

	docs := e.Search("dog").Not("fox").Documents()
	assertHasFile(t, docs, "doc1.txt")
	assertNotHasFile(t, docs, "doc0.txt")
}

func TestConfigChainComplex(t *testing.T) {
	cfgPath := setupTestEnv(t, "")
	e := loadAndIndex(t, cfgPath)

	// (fox OR cat) AND NOT snake
	foxOrCat := e.Search("fox").Or("cat")
	docs := foxOrCat.NotResult(e.Search("snake")).Documents()
	assertNotHasFile(t, docs, "doc3.txt")
}

func TestConfigAndResult(t *testing.T) {
	cfgPath := setupTestEnv(t, "")
	e := loadAndIndex(t, cfgPath)

	// fox AND (dog OR bird)
	dogOrBird := e.Search("dog").Or("bird")
	docs := e.Search("fox").AndResult(dogOrBird).Documents()
	assertHasFile(t, docs, "doc0.txt")
	assertHasFile(t, docs, "doc2.txt")
}

// ---------------------------------------------------------------------------
// Russian
// ---------------------------------------------------------------------------

func TestConfigRussian(t *testing.T) {
	cfgPath := setupTestEnv(t, "")
	e := loadAndIndex(t, cfgPath)

	docs := e.Search("лиса").Documents()
	assertHasFile(t, docs, "doc4.txt")
}

// ---------------------------------------------------------------------------
// Config: stop words disabled on index side
// ---------------------------------------------------------------------------

func TestConfigNoStopWordsOnIndex(t *testing.T) {
	dir := t.TempDir()
	docDir := filepath.Join(dir, "documents")
	os.MkdirAll(docDir, 0755)
	os.WriteFile(filepath.Join(docDir, "doc0.txt"), []byte("the dog is here"), 0644)

	yaml := `
source:
  dir: "` + docDir + `"
  extensions: [".txt"]
language: "auto"
index:
  stop_words: false
  stemming: true
  strip_accents: false
  normalize: true
  min_term_length: 1
query:
  stop_words: false
  stemming: true
  strip_accents: false
  normalize: true
  min_term_length: 1
`
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	e := loadAndIndex(t, cfgPath)

	// "the" should be searchable since stop words are disabled.
	docs := e.Search("the").Documents()
	if len(docs) == 0 {
		t.Error("'the' should be indexed when stop_words is false")
	}
}

// ---------------------------------------------------------------------------
// Config: stemming disabled
// ---------------------------------------------------------------------------

func TestConfigNoStemming(t *testing.T) {
	dir := t.TempDir()
	docDir := filepath.Join(dir, "documents")
	os.MkdirAll(docDir, 0755)
	os.WriteFile(filepath.Join(docDir, "doc0.txt"), []byte("running dogs"), 0644)

	yaml := `
source:
  dir: "` + docDir + `"
  extensions: [".txt"]
language: "english"
index:
  stop_words: true
  stemming: false
  strip_accents: false
  normalize: true
  min_term_length: 1
query:
  stop_words: false
  stemming: false
  strip_accents: false
  normalize: true
  min_term_length: 1
`
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	e := loadAndIndex(t, cfgPath)

	// "running" should match exactly, "run" should NOT match.
	docs := e.Search("running").Documents()
	if len(docs) == 0 {
		t.Error("'running' should match when stemming is off")
	}
	docs = e.Search("run").Documents()
	if len(docs) != 0 {
		t.Error("'run' should NOT match when stemming is off")
	}
}

// ---------------------------------------------------------------------------
// Config: min_term_length
// ---------------------------------------------------------------------------

func TestConfigMinTermLength(t *testing.T) {
	dir := t.TempDir()
	docDir := filepath.Join(dir, "documents")
	os.MkdirAll(docDir, 0755)
	os.WriteFile(filepath.Join(docDir, "doc0.txt"), []byte("I am a go developer"), 0644)

	yaml := `
source:
  dir: "` + docDir + `"
  extensions: [".txt"]
language: "english"
index:
  stop_words: false
  stemming: false
  strip_accents: false
  normalize: true
  min_term_length: 3
query:
  stop_words: false
  stemming: false
  strip_accents: false
  normalize: true
  min_term_length: 1
`
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	e := loadAndIndex(t, cfgPath)

	// "go" (2 chars) should be filtered out, "developer" should exist.
	docs := e.Search("go").Documents()
	if len(docs) != 0 {
		t.Error("'go' should be filtered by min_term_length=3")
	}
	docs = e.Search("developer").Documents()
	if len(docs) == 0 {
		t.Error("'developer' should be indexed")
	}
}

// ---------------------------------------------------------------------------
// Config: validation errors
// ---------------------------------------------------------------------------

func TestConfigValidationStemMismatch(t *testing.T) {
	dir := t.TempDir()
	yaml := `
source:
  dir: "` + dir + `"
  extensions: [".txt"]
language: "auto"
index:
  stop_words: true
  stemming: true
  strip_accents: false
  normalize: true
query:
  stop_words: false
  stemming: false
  strip_accents: false
  normalize: true
`
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	_, err := NewEngineFromConfig(cfgPath)
	if err == nil {
		t.Error("should reject mismatched stemming config")
	}
}

func TestConfigValidationBadLanguage(t *testing.T) {
	dir := t.TempDir()
	yaml := `
source:
  dir: "` + dir + `"
  extensions: [".txt"]
language: "chinese"
index:
  stop_words: true
  stemming: true
  strip_accents: false
  normalize: true
query:
  stop_words: false
  stemming: true
  strip_accents: false
  normalize: true
`
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(yaml), 0644)

	_, err := NewEngineFromConfig(cfgPath)
	if err == nil {
		t.Error("should reject unsupported language")
	}
}

func TestConfigFileNotFound(t *testing.T) {
	_, err := NewEngineFromConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Error("should error on missing file")
	}
}

// ---------------------------------------------------------------------------
// Immutability
// ---------------------------------------------------------------------------

func TestConfigImmutability(t *testing.T) {
	cfgPath := setupTestEnv(t, "")
	e := loadAndIndex(t, cfgPath)

	foxResult := e.Search("fox")
	original := foxResult.Count()

	_ = foxResult.And("nonexistent")

	if foxResult.Count() != original {
		t.Error("original result was mutated")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func assertHasFile(t *testing.T, docs []DocInfo, want string) {
	t.Helper()
	for _, d := range docs {
		if d.Filename == want {
			return
		}
	}
	names := make([]string, len(docs))
	for i, d := range docs {
		names[i] = d.Filename
	}
	t.Errorf("results %v should contain %s", names, want)
}

func assertNotHasFile(t *testing.T, docs []DocInfo, unwanted string) {
	t.Helper()
	for _, d := range docs {
		if d.Filename == unwanted {
			t.Errorf("results should NOT contain %s", unwanted)
			return
		}
	}
}
