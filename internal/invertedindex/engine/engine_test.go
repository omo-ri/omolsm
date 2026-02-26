package engine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"omolsm/internal/invertedindex/engine"
	"omolsm/internal/invertedindex/parser"
)

// ---------------------------------------------------------------------------
// Helper: 创建临时文档目录，写入测试文件
// ---------------------------------------------------------------------------

// testDoc 定义一个测试文档
type testDoc struct {
	filename string
	content  string
}

// 标准测试文档集
var standardDocs = []testDoc{
	{"doc1.txt", "the quick brown fox jumps over the lazy dog"},
	{"doc2.txt", "the quick brown cat sleeps on the mat"},
	{"doc3.txt", "a lazy fox and a lazy dog sit together"},
	{"doc4.txt", "birds fly over the rainbow"},
	{"doc5.txt", "the dog chases the cat around the house"},
}

func setupDocs(t *testing.T, docs []testDoc) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range docs {
		path := filepath.Join(dir, d.filename)
		if err := os.WriteFile(path, []byte(d.content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// setupEngine 创建引擎并索引文档, 支持 memory 和 lsm 两种 backend.
func setupEngine(t *testing.T, backend string, docs []testDoc) *engine.Engine {
	t.Helper()
	docDir := setupDocs(t, docs)

	cfg := engine.DefaultConfig()
	cfg.Source.Dir = docDir
	cfg.Source.Extensions = []string{".txt"}
	cfg.Language = "english"
	cfg.Storage.Backend = backend
	cfg.Index.Stemming = false // 关掉 stemming 让测试结果可预测
	cfg.Query.Stemming = false
	cfg.Index.StopWords = false // 关掉停用词方便测试 "the" 等词
	cfg.Query.StopWords = false

	if backend == "lsm" {
		lsmDir := t.TempDir()
		cfg.Storage.DataDir = lsmDir
		cfg.Storage.SSTSize = 1024 // 小阈值触发 flush/compaction
		cfg.Storage.SSTNumPerLevel = 3
		cfg.Storage.SSTBlockSize = 256
		cfg.Storage.MaxLevel = 3
	}

	e, err := engine.NewEngineFromCfg(cfg)
	if err != nil {
		// 如果没有 NewEngineFromCfg, 用下面的方式
		t.Fatalf("create engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })

	if err := e.IndexSource(); err != nil {
		t.Fatalf("index source: %v", err)
	}

	return e
}

// docNames 从 Result 提取文件名并排序, 方便比较
func docNames(r *engine.Result) []string {
	docs := r.Documents()
	names := make([]string, len(docs))
	for i, d := range docs {
		names[i] = d.Filename
	}
	sort.Strings(names)
	return names
}

// assertDocs 断言结果包含且仅包含指定文件
func assertDocs(t *testing.T, label string, r *engine.Result, expected ...string) {
	t.Helper()
	got := docNames(r)
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

func assertEmpty(t *testing.T, label string, r *engine.Result) {
	t.Helper()
	if !r.IsEmpty() {
		t.Fatalf("%s: expected empty, got %d docs: %v", label, r.Count(), docNames(r))
	}
}

// ---------------------------------------------------------------------------
// 两种 backend 跑同一套测试
// ---------------------------------------------------------------------------

func backends() []string {
	return []string{"memory", "lsm"}
}

// ======================== 基础功能 ========================

func TestSearch_SingleTerm(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)

			// "fox" 出现在 doc1, doc3
			assertDocs(t, "fox", e.Search("fox"), "doc1.txt", "doc3.txt")

			// "cat" 出现在 doc2, doc5
			assertDocs(t, "cat", e.Search("cat"), "doc2.txt", "doc5.txt")

			// "rainbow" 只在 doc4
			assertDocs(t, "rainbow", e.Search("rainbow"), "doc4.txt")
		})
	}
}

func TestSearch_MultiTermAND(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)

			// "fox" AND "dog": doc1 有 fox+dog, doc3 有 fox+dog
			assertDocs(t, "fox AND dog", e.Search("fox", "dog"), "doc1.txt", "doc3.txt")

			// "cat" AND "dog": doc5 有 cat+dog
			assertDocs(t, "cat AND dog", e.Search("cat", "dog"), "doc5.txt")

			// "fox" AND "cat": 没有文档同时包含
			assertEmpty(t, "fox AND cat", e.Search("fox", "cat"))
		})
	}
}

func TestSearch_NotFound(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)

			assertEmpty(t, "nonexistent", e.Search("zzzznotaword"))
		})
	}
}

// ======================== Result 链式操作 ========================

func TestResult_Or(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)

			// fox OR cat = doc1, doc2, doc3, doc5
			r := e.Search("fox").Or("cat")
			assertDocs(t, "fox OR cat", r, "doc1.txt", "doc2.txt", "doc3.txt", "doc5.txt")
		})
	}
}

func TestResult_Not(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)

			// "dog" 在 doc1, doc3, doc5. 去掉 "fox"(doc1, doc3) → doc5
			r := e.Search("dog").Not("fox")
			assertDocs(t, "dog NOT fox", r, "doc5.txt")
		})
	}
}

func TestResult_AndResult(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)

			foxOrCat := e.Search("fox").Or("cat") // doc1,2,3,5
			dogResult := e.Search("dog")          // doc1,3,5
			r := foxOrCat.AndResult(dogResult)    // doc1,3,5
			assertDocs(t, "(fox OR cat) AND dog", r, "doc1.txt", "doc3.txt", "doc5.txt")
		})
	}
}

func TestResult_Complex_Chain(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)

			// (fox OR cat) AND NOT rainbow
			r := e.Search("fox").Or("cat").Not("rainbow")
			assertDocs(t, "(fox OR cat) NOT rainbow", r,
				"doc1.txt", "doc2.txt", "doc3.txt", "doc5.txt")
		})
	}
}

func TestResult_Immutable(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)

			original := e.Search("dog") // doc1,3,5
			_ = original.Not("fox")     // doc5, 但不应修改 original

			// original 应该还是 3 个
			if original.Count() != 3 {
				t.Fatalf("original mutated: expected 3, got %d", original.Count())
			}
		})
	}
}

// ======================== Parser 集成 ========================

func TestParser_SingleTerm(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)
			r, err := parser.Execute(e, "fox")
			if err != nil {
				t.Fatal(err)
			}
			assertDocs(t, "parse: fox", r, "doc1.txt", "doc3.txt")
		})
	}
}

func TestParser_AND(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)
			r, err := parser.Execute(e, "fox AND dog")
			if err != nil {
				t.Fatal(err)
			}
			assertDocs(t, "parse: fox AND dog", r, "doc1.txt", "doc3.txt")
		})
	}
}

func TestParser_OR(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)
			r, err := parser.Execute(e, "fox OR cat")
			if err != nil {
				t.Fatal(err)
			}
			assertDocs(t, "parse: fox OR cat", r,
				"doc1.txt", "doc2.txt", "doc3.txt", "doc5.txt")
		})
	}
}

func TestParser_AND_NOT(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)
			r, err := parser.Execute(e, "dog AND NOT fox")
			if err != nil {
				t.Fatal(err)
			}
			assertDocs(t, "parse: dog AND NOT fox", r, "doc5.txt")
		})
	}
}

func TestParser_Nested(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)
			r, err := parser.Execute(e, "(fox OR cat) AND dog")
			if err != nil {
				t.Fatal(err)
			}
			// fox: doc1,3  cat: doc2,5  union: doc1,2,3,5
			// dog: doc1,3,5  intersect: doc1,3,5
			assertDocs(t, "parse: (fox OR cat) AND dog", r,
				"doc1.txt", "doc3.txt", "doc5.txt")
		})
	}
}

func TestParser_Complex_Nested(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)
			r, err := parser.Execute(e, "(fox OR cat) AND NOT rainbow")
			if err != nil {
				t.Fatal(err)
			}
			assertDocs(t, "parse: (fox OR cat) AND NOT rainbow", r,
				"doc1.txt", "doc2.txt", "doc3.txt", "doc5.txt")
		})
	}
}

func TestParser_Errors(t *testing.T) {
	e := setupEngine(t, "memory", standardDocs)

	badQueries := []string{
		"",
		"AND fox",
		"fox AND",
		"fox OR",
		"(fox",
		"fox)",
		"AND",
		"OR",
		"()",
	}

	for _, q := range badQueries {
		t.Run(q, func(t *testing.T) {
			_, err := parser.Execute(e, q)
			if err == nil {
				t.Fatalf("expected error for query %q, got nil", q)
			}
		})
	}
}

// ======================== Stats ========================

func TestStats(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, standardDocs)
			s := e.Stats()

			if s.DocCount != uint32(len(standardDocs)) {
				t.Fatalf("expected %d docs, got %d", len(standardDocs), s.DocCount)
			}
			if s.TermCount == 0 {
				t.Fatal("expected non-zero term count")
			}
		})
	}
}

func TestLSMStatsReport(t *testing.T) {
	e := setupEngine(t, "lsm", standardDocs)
	report := e.LSMStatsReport()
	if report == "" {
		t.Fatal("expected non-empty LSM stats report")
	}
	t.Log(report)
}

// ======================== 边界情况 ========================

func TestEmptyDir(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			e := setupEngine(t, b, nil) // 没有文档
			s := e.Stats()
			if s.DocCount != 0 {
				t.Fatalf("expected 0 docs, got %d", s.DocCount)
			}
			assertEmpty(t, "search on empty", e.Search("anything"))
		})
	}
}

func TestSingleDoc(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			docs := []testDoc{{"only.txt", "hello world"}}
			e := setupEngine(t, b, docs)

			assertDocs(t, "hello", e.Search("hello"), "only.txt")
			assertDocs(t, "world", e.Search("world"), "only.txt")
			assertEmpty(t, "missing", e.Search("missing"))
		})
	}
}

func TestManyDocs(t *testing.T) {
	for _, b := range backends() {
		t.Run(b, func(t *testing.T) {
			// 50 个文档，每个包含 "common" 和自己的唯一词
			docs := make([]testDoc, 50)
			for i := range docs {
				docs[i] = testDoc{
					filename: fmt.Sprintf("doc%03d.txt", i),
					content:  fmt.Sprintf("common unique_%03d", i),
				}
			}
			e := setupEngine(t, b, docs)

			// "common" 应该在所有文档中
			r := e.Search("common")
			if r.Count() != 50 {
				t.Fatalf("expected 50 docs for 'common', got %d", r.Count())
			}

			// 唯一词只在一个文档中
			r = e.Search("unique_025")
			if r.Count() != 1 {
				t.Fatalf("expected 1 doc for unique term, got %d", r.Count())
			}
		})
	}
}

// ======================== Memory vs LSM 一致性 ========================

func TestMemoryAndLSM_SameResults(t *testing.T) {
	memEngine := setupEngine(t, "memory", standardDocs)
	lsmEngine := setupEngine(t, "lsm", standardDocs)

	queries := []string{
		"fox",
		"dog",
		"cat",
		"fox AND dog",
		"fox OR cat",
		"dog AND NOT fox",
		"(fox OR cat) AND dog",
		"(fox OR cat) AND NOT rainbow",
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			memResult, err := parser.Execute(memEngine, q)
			if err != nil {
				t.Fatalf("memory parse error: %v", err)
			}
			lsmResult, err := parser.Execute(lsmEngine, q)
			if err != nil {
				t.Fatalf("lsm parse error: %v", err)
			}

			memNames := docNames(memResult)
			lsmNames := docNames(lsmResult)

			if len(memNames) != len(lsmNames) {
				t.Fatalf("result count mismatch: memory=%v, lsm=%v", memNames, lsmNames)
			}
			for i := range memNames {
				if memNames[i] != lsmNames[i] {
					t.Fatalf("result mismatch: memory=%v, lsm=%v", memNames, lsmNames)
				}
			}
		})
	}
}
