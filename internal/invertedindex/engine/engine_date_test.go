package engine_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"omolsm/internal/invertedindex/engine"
	"omolsm/internal/invertedindex/parser"
)

func date(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

// setupDateEngine creates an engine and indexes docs with date metadata.
func setupDateEngine(t *testing.T) *engine.Engine {
	t.Helper()
	dir := t.TempDir()

	type datedDoc struct {
		filename string
		content  string
		meta     engine.DateMeta
	}

	docs := []datedDoc{
		{
			"doc1.txt", "the quick brown fox",
			engine.DateMeta{
				Date:      date("2024-01-15"),
				StartDate: date("2024-01-01"),
				EndDate:   date("2024-06-30"),
			},
		},
		{
			"doc2.txt", "the lazy dog sleeps",
			engine.DateMeta{
				Date:      date("2024-03-10"),
				StartDate: date("2024-03-01"),
				EndDate:   date("2024-12-31"),
			},
		},
		{
			"doc3.txt", "cats and birds fly",
			engine.DateMeta{
				Date:      date("2023-06-20"),
				StartDate: date("2023-01-01"),
				EndDate:   nil, // forever valid
			},
		},
		{
			"doc4.txt", "fox and dog together",
			engine.DateMeta{
				Date:      date("2024-07-01"),
				StartDate: date("2024-07-01"),
				EndDate:   date("2024-07-31"),
			},
		},
		{
			"doc5.txt", "rainbow over the hill",
			engine.DateMeta{
				Date:      date("2025-01-01"),
				StartDate: date("2025-01-01"),
				EndDate:   nil, // forever valid
			},
		},
	}

	cfg := engine.DefaultConfig()
	cfg.Source.Dir = dir
	cfg.Source.Extensions = []string{".txt"}
	cfg.Language = "english"
	cfg.Index.Stemming = false
	cfg.Query.Stemming = false
	cfg.Index.StopWords = false
	cfg.Query.StopWords = false

	e, err := engine.NewEngineFromCfg(cfg)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })

	for _, d := range docs {
		path := filepath.Join(dir, d.filename)
		if err := os.WriteFile(path, []byte(d.content), 0644); err != nil {
			t.Fatal(err)
		}
		if err := e.IndexFileWithDates(path, d.meta); err != nil {
			t.Fatalf("index %s: %v", d.filename, err)
		}
	}

	return e
}

func sortedNames(r *engine.Result) []string {
	docs := r.Documents()
	names := make([]string, len(docs))
	for i, d := range docs {
		names[i] = d.Filename
	}
	sort.Strings(names)
	return names
}

func assertDateDocs(t *testing.T, label string, r *engine.Result, expected ...string) {
	t.Helper()
	got := sortedNames(r)
	sort.Strings(expected)
	if len(got) != len(expected) {
		t.Fatalf("%s: expected %v, got %v", label, expected, got)
	}
	for i := range got {
		if got[i] != expected[i] {
			t.Fatalf("%s: expected %v, got %v", label, expected, got)
		}
	}
}

func assertDateEmpty(t *testing.T, label string, r *engine.Result) {
	t.Helper()
	if !r.IsEmpty() {
		t.Fatalf("%s: expected empty, got %v", label, sortedNames(r))
	}
}

// ======================== Requirement A: DATE range ========================

func TestSearchDateRange(t *testing.T) {
	e := setupDateEngine(t)

	// All 2024 docs: doc1(Jan), doc2(Mar), doc4(Jul)
	r, err := e.SearchDateRange("2024-01-01", "2024-12-31")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "2024 all", r, "doc1.txt", "doc2.txt", "doc4.txt")

	// Only Q1 2024: doc1(Jan 15), doc2(Mar 10)
	r, err = e.SearchDateRange("2024-01-01", "2024-03-31")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "2024 Q1", r, "doc1.txt", "doc2.txt")

	// 2023: doc3 only
	r, err = e.SearchDateRange("2023-01-01", "2023-12-31")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "2023", r, "doc3.txt")

	// No docs in range
	r, err = e.SearchDateRange("2022-01-01", "2022-12-31")
	if err != nil {
		t.Fatal(err)
	}
	assertDateEmpty(t, "2022", r)
}

// ======================== Requirement B: VALID in range ========================

func TestSearchValidInRange(t *testing.T) {
	e := setupDateEngine(t)

	// Query: valid in Feb 2024
	// doc1: [2024-01-01, 2024-06-30] → overlaps ✓
	// doc2: [2024-03-01, 2024-12-31] → starts after Feb → ✗
	// doc3: [2023-01-01, nil]        → forever valid ✓
	// doc4: [2024-07-01, 2024-07-31] → starts after Feb → ✗
	// doc5: [2025-01-01, nil]        → starts after Feb → ✗
	r, err := e.SearchValidInRange("2024-02-01", "2024-02-28")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "valid Feb 2024", r, "doc1.txt", "doc3.txt")

	// Query: valid in Jul 2024
	// doc1: ends Jun 30 → ✗
	// doc2: [Mar, Dec] → overlaps ✓
	// doc3: forever ✓
	// doc4: [Jul 1, Jul 31] → overlaps ✓
	// doc5: starts 2025 → ✗
	r, err = e.SearchValidInRange("2024-07-01", "2024-07-31")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "valid Jul 2024", r, "doc2.txt", "doc3.txt", "doc4.txt")

	// Query: valid in 2026 — only forever-valid docs
	// doc3: forever ✓, doc5: forever ✓
	r, err = e.SearchValidInRange("2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "valid 2026", r, "doc3.txt", "doc5.txt")
}

// ======================== Requirement B: APPEARED in range ========================

func TestSearchAppearedInRange(t *testing.T) {
	e := setupDateEngine(t)

	// Appeared in 2024: doc1(Jan 1), doc2(Mar 1), doc4(Jul 1)
	r, err := e.SearchAppearedInRange("2024-01-01", "2024-12-31")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "appeared 2024", r, "doc1.txt", "doc2.txt", "doc4.txt")

	// Appeared in 2023: doc3
	r, err = e.SearchAppearedInRange("2023-01-01", "2023-12-31")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "appeared 2023", r, "doc3.txt")
}

// ======================== Combined: date + text ========================

func TestDateAndTextCombined(t *testing.T) {
	e := setupDateEngine(t)

	// fox AND DATE in 2024: fox in doc1, doc4. Both are 2024.
	foxResult := e.Search("fox")
	dateResult, err := e.SearchDateRange("2024-01-01", "2024-12-31")
	if err != nil {
		t.Fatal(err)
	}
	r := foxResult.AndResult(dateResult)
	assertDateDocs(t, "fox AND date 2024", r, "doc1.txt", "doc4.txt")

	// fox AND VALID in Jul 2024: fox in doc1(ends Jun→✗), doc4(Jul→✓)
	validResult, err := e.SearchValidInRange("2024-07-01", "2024-07-31")
	if err != nil {
		t.Fatal(err)
	}
	r = e.Search("fox").AndResult(validResult)
	assertDateDocs(t, "fox AND valid Jul 2024", r, "doc4.txt")
}

// ======================== Parser integration ========================

func TestParser_DateRange(t *testing.T) {
	e := setupDateEngine(t)

	r, err := parser.Execute(e, "DATE:[2024-01-01,2024-12-31]")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "parse DATE", r, "doc1.txt", "doc2.txt", "doc4.txt")
}

func TestParser_DateAndText(t *testing.T) {
	e := setupDateEngine(t)

	r, err := parser.Execute(e, "fox AND DATE:[2024-01-01,2024-12-31]")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "parse fox AND DATE", r, "doc1.txt", "doc4.txt")
}

func TestParser_ValidRange(t *testing.T) {
	e := setupDateEngine(t)

	r, err := parser.Execute(e, "VALID:[2024-07-01,2024-07-31]")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "parse VALID", r, "doc2.txt", "doc3.txt", "doc4.txt")
}

func TestParser_AppearedRange(t *testing.T) {
	e := setupDateEngine(t)

	r, err := parser.Execute(e, "APPEARED:[2024-01-01,2024-12-31]")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "parse APPEARED", r, "doc1.txt", "doc2.txt", "doc4.txt")
}

func TestParser_ComplexDateBoolean(t *testing.T) {
	e := setupDateEngine(t)

	// (fox OR dog) AND DATE:[2024-01-01,2024-06-30]
	// fox: doc1, doc4. dog: doc2, doc4. union: doc1, doc2, doc4
	// DATE 2024 H1: doc1(Jan), doc2(Mar). doc4 is Jul → out
	r, err := parser.Execute(e, "(fox OR dog) AND DATE:[2024-01-01,2024-06-30]")
	if err != nil {
		t.Fatal(err)
	}
	assertDateDocs(t, "parse complex", r, "doc1.txt", "doc2.txt")
}

func TestParser_DateErrors(t *testing.T) {
	e := setupDateEngine(t)

	badQueries := []string{
		"DATE:[]",
		"DATE:[2024-01-01]",
		"DATE:[bad,date]",
		"DATE:2024-01-01,2024-12-31",
	}

	for _, q := range badQueries {
		t.Run(q, func(t *testing.T) {
			_, err := parser.Execute(e, q)
			if err == nil {
				t.Fatalf("expected error for %q", q)
			}
		})
	}
}
