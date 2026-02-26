package parser

import (
	"omolsm/internal/invertedindex/engine"
	"os"
	"path/filepath"
	"testing"
)

// doc0: fox, dog, quick, brown
// doc1: cat, dog, fast, brown
// doc2: fox, bird
// doc3: cat, bird, snake
func setupEngine(t *testing.T) *engine.Engine {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"doc0.txt": "The quick brown fox jumps over the lazy dog",
		"doc1.txt": "A fast brown cat sleeps with the dog",
		"doc2.txt": "The fox and the bird flew over the hill",
		"doc3.txt": "The cat and the bird saw a snake",
	}

	for name, content := range files {
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	}

	e := engine.NewEngine()
	e.IndexDir(dir)
	return e
}

func TestSingleTerm(t *testing.T) {
	e := setupEngine(t)

	result, err := Execute(e, "fox")
	if err != nil {
		t.Fatal(err)
	}
	assertDocs(t, result, "doc0.txt", "doc2.txt")
}

func TestAnd(t *testing.T) {
	e := setupEngine(t)

	result, err := Execute(e, "fox AND dog")
	if err != nil {
		t.Fatal(err)
	}
	assertDocs(t, result, "doc0.txt")
	assertNotDoc(t, result, "doc2.txt")
}

func TestOr(t *testing.T) {
	e := setupEngine(t)

	result, err := Execute(e, "fox OR cat")
	if err != nil {
		t.Fatal(err)
	}
	assertDocs(t, result, "doc0.txt", "doc1.txt", "doc2.txt", "doc3.txt")
}

func TestAndNot(t *testing.T) {
	e := setupEngine(t)

	// dog AND NOT fox → doc1
	result, err := Execute(e, "dog AND NOT fox")
	if err != nil {
		t.Fatal(err)
	}
	assertDocs(t, result, "doc1.txt")
	assertNotDoc(t, result, "doc0.txt")
}

func TestParentheses(t *testing.T) {
	e := setupEngine(t)

	// fox AND (dog OR bird) → doc0 (fox+dog), doc2 (fox+bird)
	result, err := Execute(e, "fox AND (dog OR bird)")
	if err != nil {
		t.Fatal(err)
	}
	assertDocs(t, result, "doc0.txt", "doc2.txt")
}

func TestComplex(t *testing.T) {
	e := setupEngine(t)

	// (fox OR cat) AND NOT snake → doc0, doc1, doc2
	result, err := Execute(e, "(fox OR cat) AND NOT snake")
	if err != nil {
		t.Fatal(err)
	}
	assertNotDoc(t, result, "doc3.txt")
}

func TestPrecedence(t *testing.T) {
	e := setupEngine(t)

	// dog OR fox AND bird → dog OR (fox AND bird) = {0,1} OR {2} = {0,1,2}
	result, err := Execute(e, "dog OR fox AND bird")
	if err != nil {
		t.Fatal(err)
	}
	assertDocs(t, result, "doc0.txt", "doc1.txt", "doc2.txt")
}

func TestCaseInsensitiveOperators(t *testing.T) {
	e := setupEngine(t)

	result, err := Execute(e, "fox and dog")
	if err != nil {
		t.Fatal(err)
	}
	assertDocs(t, result, "doc0.txt")
}

func TestNonExistentTerm(t *testing.T) {
	e := setupEngine(t)

	result, err := Execute(e, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsEmpty() {
		t.Error("should be empty")
	}
}

// ---------------------------------------------------------------------------
// Error cases
// ---------------------------------------------------------------------------

func TestErrorEmpty(t *testing.T) {
	e := setupEngine(t)
	_, err := Execute(e, "")
	if err == nil {
		t.Error("should error on empty query")
	}
}

func TestErrorMissingParen(t *testing.T) {
	e := setupEngine(t)
	_, err := Execute(e, "(fox AND dog")
	if err == nil {
		t.Error("should error on missing paren")
	}
}

func TestErrorLeadingOperator(t *testing.T) {
	e := setupEngine(t)
	_, err := Execute(e, "AND fox")
	if err == nil {
		t.Error("should error on leading operator")
	}
}

func TestErrorTrailingOperator(t *testing.T) {
	e := setupEngine(t)
	_, err := Execute(e, "fox AND")
	if err == nil {
		t.Error("should error on trailing operator")
	}
}

// ---------------------------------------------------------------------------
// Tokenizer
// ---------------------------------------------------------------------------

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"fox", []string{"fox"}},
		{"fox AND dog", []string{"fox", "AND", "dog"}},
		{"(fox OR cat)", []string{"(", "fox", "OR", "cat", ")"}},
		{"(fox OR cat) AND NOT dog", []string{"(", "fox", "OR", "cat", ")", "AND", "NOT", "dog"}},
		{"  spaces   everywhere  ", []string{"spaces", "everywhere"}},
	}

	for _, tt := range tests {
		got := tokenize(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("tokenize(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func assertDocs(t *testing.T, result *engine.Result, want ...string) {
	t.Helper()
	docs := result.Documents()
	for _, w := range want {
		found := false
		for _, d := range docs {
			if d.Filename == w {
				found = true
				break
			}
		}
		if !found {
			names := make([]string, len(docs))
			for i, d := range docs {
				names[i] = d.Filename
			}
			t.Errorf("results %v should contain %s", names, w)
		}
	}
}

func assertNotDoc(t *testing.T, result *engine.Result, unwanted string) {
	t.Helper()
	for _, d := range result.Documents() {
		if d.Filename == unwanted {
			t.Errorf("results should NOT contain %s", unwanted)
			return
		}
	}
}
