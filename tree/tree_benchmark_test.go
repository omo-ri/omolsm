package tree

import (
	"fmt"
	"io"
	"log"
	"omolsm"
	"testing"
)

func setupBenchTree(b *testing.B) LSMTree {
	log.SetOutput(io.Discard)
	dir := b.TempDir()
	conf, err := omolsm.NewConfig(dir)
	if err != nil {
		b.Fatal(err)
	}
	tr, err := NewTree(conf)
	if err != nil {
		b.Fatal(err)
	}
	return tr
}

func setupBenchTreeSmall(b *testing.B) LSMTree {
	log.SetOutput(io.Discard)
	dir := b.TempDir()
	conf, err := omolsm.NewConfig(dir, omolsm.WithSSTSize(256))
	if err != nil {
		b.Fatal(err)
	}
	tr, err := NewTree(conf)
	if err != nil {
		b.Fatal(err)
	}
	return tr
}

// 预写入数据，返回写入数量
func preload(b *testing.B, tr LSMTree, count int) {
	for i := 0; i < count; i++ {
		tr.Put(fmt.Sprintf("key_%08d", i), []byte("value_12345678"))
	}
}

// ========== 写入 ==========

// 纯 memtable 写入（大阈值，不触发 flush）
func BenchmarkPut_MemtableOnly(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	value := []byte("value_12345678")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Put(fmt.Sprintf("key_%08d", i), value)
	}
}

// 写入含 flush（小阈值，频繁触发）
func BenchmarkPut_WithFlush(b *testing.B) {
	tr := setupBenchTreeSmall(b)
	defer tr.Close()
	value := []byte("value_12345678")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Put(fmt.Sprintf("key_%08d", i), value)
	}
}

// 覆盖写入同一个 key
func BenchmarkPut_Overwrite(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	value := []byte("value_12345678")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Put("same_key", value)
	}
}

// ========== 读取 ==========

// 顺序读（memtable 命中）
func BenchmarkGet_MemtableHit(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	count := 10000
	preload(b, tr, count)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Get(fmt.Sprintf("key_%08d", i%count))
	}
}

// 随机读（memtable 命中）
func BenchmarkGet_MemtableRandom(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	count := 10000
	preload(b, tr, count)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Get(fmt.Sprintf("key_%08d", (i*7919)%count))
	}
}

// 读不存在的 key
func BenchmarkGet_Miss(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	preload(b, tr, 10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Get(fmt.Sprintf("miss_%08d", i))
	}
}

// 读 SSTable（小阈值，数据落盘后读）
func BenchmarkGet_SSTable(b *testing.B) {
	tr := setupBenchTreeSmall(b)
	defer tr.Close()
	count := 1000
	preload(b, tr, count)
	// 再写一批触发 flush，确保之前的数据在 SSTable 里
	for i := 0; i < 100; i++ {
		tr.Put(fmt.Sprintf("flush_%08d", i), []byte("flush"))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Get(fmt.Sprintf("key_%08d", i%count))
	}
}

// ========== 删除 ==========

func BenchmarkDelete(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	preload(b, tr, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Delete(fmt.Sprintf("key_%08d", i))
	}
}

// ========== 范围查询 ==========

// 短范围（10个key）
func BenchmarkScan_Short(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	preload(b, tr, 10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := fmt.Sprintf("key_%08d", (i*10)%9900)
		end := fmt.Sprintf("key_%08d", (i*10)%9900+10)
		tr.Scan(start, end)
	}
}

// 中范围（100个key）
func BenchmarkScan_Medium(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	preload(b, tr, 10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := fmt.Sprintf("key_%08d", (i*100)%9000)
		end := fmt.Sprintf("key_%08d", (i*100)%9000+100)
		tr.Scan(start, end)
	}
}

// 长范围（1000个key）
func BenchmarkScan_Long(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	preload(b, tr, 10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Scan("key_00000000", "key_00001000")
	}
}

// 跨 SSTable 的范围查询
func BenchmarkScan_AcrossSSTable(b *testing.B) {
	tr := setupBenchTreeSmall(b)
	defer tr.Close()
	preload(b, tr, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := fmt.Sprintf("key_%08d", (i*10)%900)
		end := fmt.Sprintf("key_%08d", (i*10)%900+10)
		tr.Scan(start, end)
	}
}

// ========== 混合操作 ==========

// 读写混合 7:3（70%读 30%写）
func BenchmarkMixed_ReadHeavy(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	preload(b, tr, 10000)
	value := []byte("value_12345678")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%10 < 7 {
			tr.Get(fmt.Sprintf("key_%08d", i%10000))
		} else {
			tr.Put(fmt.Sprintf("new_%08d", i), value)
		}
	}
}

// 读写混合 3:7（30%读 70%写）
func BenchmarkMixed_WriteHeavy(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	preload(b, tr, 10000)
	value := []byte("value_12345678")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%10 < 3 {
			tr.Get(fmt.Sprintf("key_%08d", i%10000))
		} else {
			tr.Put(fmt.Sprintf("new_%08d", i), value)
		}
	}
}

// 写+删+读混合
func BenchmarkMixed_PutDeleteGet(b *testing.B) {
	tr := setupBenchTree(b)
	defer tr.Close()
	value := []byte("value_12345678")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%08d", i%5000)
		switch i % 3 {
		case 0:
			tr.Put(key, value)
		case 1:
			tr.Get(key)
		case 2:
			tr.Delete(key)
		}
	}
}
