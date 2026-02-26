package dictionary_test

import (
	"fmt"
	"os"
	"testing"

	"omolsm/config"
	"omolsm/internal/invertedindex/dictionary"
)

// ---------------------------------------------------------------------------
// Helper: 构建两种 Dictionary 实现
// ---------------------------------------------------------------------------

type dictFactory struct {
	name    string
	create  func(t *testing.T) dictionary.Dictionary
	cleanup func()
}

func allDictFactories(t *testing.T) []dictFactory {
	return []dictFactory{
		{
			name: "Memory",
			create: func(t *testing.T) dictionary.Dictionary {
				return dictionary.NewMemDictionary()
			},
		},
		{
			name: "LSM",
			create: func(t *testing.T) dictionary.Dictionary {
				dir, err := os.MkdirTemp("", "dict-test-")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.RemoveAll(dir) })

				conf, err := config.NewConfig(dir,
					config.WithSSTSize(1024),     // 小阈值，容易触发 flush/compaction
					config.WithSSTNumPerLevel(3), // 3 个就 compact
					config.WithSSTDataBlockSize(256),
					config.WithMaxLevel(3),
				)
				if err != nil {
					t.Fatal(err)
				}

				d, err := dictionary.NewLSMDictionary(conf)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { d.Close() })
				return d
			},
		},
	}
}

// ---------------------------------------------------------------------------
// 统一测试: 两种实现跑同一套用例
// ---------------------------------------------------------------------------

func TestGetOrAdd_Basic(t *testing.T) {
	for _, f := range allDictFactories(t) {
		t.Run(f.name, func(t *testing.T) {
			d := f.create(t)

			// 首次添加
			id1 := d.GetOrAdd("hello")
			id2 := d.GetOrAdd("world")

			if id1 == id2 {
				t.Fatalf("different terms got same id: %d", id1)
			}

			// 重复添加返回相同 id
			id1Again := d.GetOrAdd("hello")
			if id1 != id1Again {
				t.Fatalf("same term got different id: %d vs %d", id1, id1Again)
			}

			if d.Size() != 2 {
				t.Fatalf("expected size 2, got %d", d.Size())
			}
		})
	}
}

func TestGet_Existing(t *testing.T) {
	for _, f := range allDictFactories(t) {
		t.Run(f.name, func(t *testing.T) {
			d := f.create(t)

			expected := d.GetOrAdd("apple")

			got, ok := d.Get("apple")
			if !ok {
				t.Fatal("Get returned not found for existing term")
			}
			if got != expected {
				t.Fatalf("expected id %d, got %d", expected, got)
			}
		})
	}
}

func TestGet_NonExistent(t *testing.T) {
	for _, f := range allDictFactories(t) {
		t.Run(f.name, func(t *testing.T) {
			d := f.create(t)

			_, ok := d.Get("nonexistent")
			if ok {
				t.Fatal("Get returned found for non-existent term")
			}
		})
	}
}

func TestGetOrAddTerms_Batch(t *testing.T) {
	for _, f := range allDictFactories(t) {
		t.Run(f.name, func(t *testing.T) {
			d := f.create(t)

			terms := []string{"cat", "dog", "bird", "cat", "dog"}
			ids := d.GetOrAddTerms(terms)

			if len(ids) != len(terms) {
				t.Fatalf("expected %d ids, got %d", len(terms), len(ids))
			}

			// cat 出现两次，id 应该相同
			if ids[0] != ids[3] {
				t.Fatalf("same term 'cat' got different ids: %d vs %d", ids[0], ids[3])
			}

			// dog 出现两次，id 应该相同
			if ids[1] != ids[4] {
				t.Fatalf("same term 'dog' got different ids: %d vs %d", ids[1], ids[4])
			}

			// 三个不同 term
			if d.Size() != 3 {
				t.Fatalf("expected size 3, got %d", d.Size())
			}
		})
	}
}

func TestIDsAreUnique(t *testing.T) {
	for _, f := range allDictFactories(t) {
		t.Run(f.name, func(t *testing.T) {
			d := f.create(t)

			n := 200
			seen := make(map[uint32]string, n)
			for i := 0; i < n; i++ {
				term := fmt.Sprintf("term_%d", i)
				id := d.GetOrAdd(term)
				if prev, exists := seen[id]; exists {
					t.Fatalf("duplicate id %d for terms %q and %q", id, prev, term)
				}
				seen[id] = term
			}

			if d.Size() != n {
				t.Fatalf("expected size %d, got %d", n, d.Size())
			}
		})
	}
}

func TestGetAfterManyInserts(t *testing.T) {
	for _, f := range allDictFactories(t) {
		t.Run(f.name, func(t *testing.T) {
			d := f.create(t)

			n := 500
			expected := make(map[string]uint32, n)

			// 写入
			for i := 0; i < n; i++ {
				term := fmt.Sprintf("word_%04d", i)
				id := d.GetOrAdd(term)
				expected[term] = id
			}

			// 全部读回来验证
			for term, expectedID := range expected {
				got, ok := d.Get(term)
				if !ok {
					t.Fatalf("term %q not found after insert", term)
				}
				if got != expectedID {
					t.Fatalf("term %q: expected id %d, got %d", term, expectedID, got)
				}
			}
		})
	}
}

// TestLSM_SurvivesCompaction 专门测 LSM: 大量写入触发多轮 compaction 后数据不丢
func TestLSM_SurvivesCompaction(t *testing.T) {
	dir, err := os.MkdirTemp("", "dict-compact-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	conf, err := config.NewConfig(dir,
		config.WithSSTSize(512),      // 极小阈值，频繁 flush
		config.WithSSTNumPerLevel(2), // 2 个就 compact
		config.WithSSTDataBlockSize(128),
		config.WithMaxLevel(3),
	)
	if err != nil {
		t.Fatal(err)
	}

	d, err := dictionary.NewLSMDictionary(conf)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	n := 1000
	expected := make(map[string]uint32, n)

	for i := 0; i < n; i++ {
		term := fmt.Sprintf("compact_term_%04d", i)
		id := d.GetOrAdd(term)
		expected[term] = id
	}

	// 验证所有数据在 compaction 后依然可读
	for term, expectedID := range expected {
		got, ok := d.Get(term)
		if !ok {
			t.Fatalf("term %q lost after compaction", term)
		}
		if got != expectedID {
			t.Fatalf("term %q: expected id %d, got %d after compaction", term, expectedID, got)
		}
	}

	// 打印 stats 帮助确认 compaction 确实发生了
	stats := d.GetTreeStats()
	t.Logf("LSM Stats: flushes=%d, compactions=%d, bytes_written=%d",
		stats.FlushCount, stats.CompactCount, stats.BytesWritten)

	if stats.FlushCount == 0 {
		t.Fatal("expected at least one flush with small SST size")
	}
	if stats.CompactCount == 0 {
		t.Fatal("expected at least one compaction with small SST num per level")
	}
}

func TestEmptyDictionary(t *testing.T) {
	for _, f := range allDictFactories(t) {
		t.Run(f.name, func(t *testing.T) {
			d := f.create(t)

			if d.Size() != 0 {
				t.Fatalf("expected size 0, got %d", d.Size())
			}

			_, ok := d.Get("anything")
			if ok {
				t.Fatal("empty dictionary should not find anything")
			}

			ids := d.GetOrAddTerms(nil)
			if len(ids) != 0 {
				t.Fatalf("expected empty ids, got %d", len(ids))
			}

			ids = d.GetOrAddTerms([]string{})
			if len(ids) != 0 {
				t.Fatalf("expected empty ids, got %d", len(ids))
			}
		})
	}
}

func TestUnicodeTerms(t *testing.T) {
	for _, f := range allDictFactories(t) {
		t.Run(f.name, func(t *testing.T) {
			d := f.create(t)

			terms := []string{
				"hello",
				"привет", // Russian
				"你好",     // Chinese
				"مرحبا",  // Arabic
				"🎉",      // Emoji
				"café",
				"naïve",
			}

			ids := d.GetOrAddTerms(terms)

			// 验证全部可以读回
			for i, term := range terms {
				got, ok := d.Get(term)
				if !ok {
					t.Fatalf("term %q not found", term)
				}
				if got != ids[i] {
					t.Fatalf("term %q: expected id %d, got %d", term, ids[i], got)
				}
			}

			if d.Size() != len(terms) {
				t.Fatalf("expected size %d, got %d", len(terms), d.Size())
			}
		})
	}
}

// TestMemAndLSM_SameResults 两种实现对同样输入产生相同的 id 映射
func TestMemAndLSM_SameResults(t *testing.T) {
	// Memory
	mem := dictionary.NewMemDictionary()

	// LSM
	dir, err := os.MkdirTemp("", "dict-compare-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	conf, err := config.NewConfig(dir,
		config.WithSSTSize(1024),
		config.WithSSTNumPerLevel(3),
		config.WithSSTDataBlockSize(256),
	)
	if err != nil {
		t.Fatal(err)
	}

	lsm, err := dictionary.NewLSMDictionary(conf)
	if err != nil {
		t.Fatal(err)
	}
	defer lsm.Close()

	// 相同顺序插入
	terms := []string{"alpha", "beta", "gamma", "delta", "alpha", "beta", "epsilon"}

	memIDs := mem.GetOrAddTerms(terms)
	lsmIDs := lsm.GetOrAddTerms(terms)

	// id 的具体数值可以不同，但映射关系必须一致:
	// - 相同 term 得到相同 id
	// - 不同 term 得到不同 id
	for i := 0; i < len(terms); i++ {
		for j := 0; j < len(terms); j++ {
			memSame := memIDs[i] == memIDs[j]
			lsmSame := lsmIDs[i] == lsmIDs[j]
			if memSame != lsmSame {
				t.Fatalf("mapping inconsistency at terms[%d]=%q, terms[%d]=%q: mem_same=%v, lsm_same=%v",
					i, terms[i], j, terms[j], memSame, lsmSame)
			}
		}
	}

	if mem.Size() != lsm.Size() {
		t.Fatalf("size mismatch: mem=%d, lsm=%d", mem.Size(), lsm.Size())
	}
}

// ---------------------------------------------------------------------------
// Benchmark: Memory vs LSM
// ---------------------------------------------------------------------------

func BenchmarkGetOrAdd(b *testing.B) {
	for _, bc := range []struct {
		name   string
		create func(b *testing.B) dictionary.Dictionary
	}{
		{
			name: "Memory",
			create: func(b *testing.B) dictionary.Dictionary {
				return dictionary.NewMemDictionary()
			},
		},
		{
			name: "LSM",
			create: func(b *testing.B) dictionary.Dictionary {
				dir, _ := os.MkdirTemp("", "dict-bench-")
				b.Cleanup(func() { os.RemoveAll(dir) })
				conf, _ := config.NewConfig(dir)
				d, _ := dictionary.NewLSMDictionary(conf)
				b.Cleanup(func() { d.Close() })
				return d
			},
		},
	} {
		b.Run(bc.name+"/Insert", func(b *testing.B) {
			d := bc.create(b)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				d.GetOrAdd(fmt.Sprintf("term_%d", i))
			}
		})

		b.Run(bc.name+"/Lookup", func(b *testing.B) {
			d := bc.create(b)
			// 预填充
			for i := 0; i < 10000; i++ {
				d.GetOrAdd(fmt.Sprintf("term_%d", i))
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				d.Get(fmt.Sprintf("term_%d", i%10000))
			}
		})

		b.Run(bc.name+"/BatchInsert_100", func(b *testing.B) {
			d := bc.create(b)
			batch := make([]string, 100)
			for i := range batch {
				batch[i] = fmt.Sprintf("batch_%d", i)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				d.GetOrAddTerms(batch)
			}
		})
	}
}
