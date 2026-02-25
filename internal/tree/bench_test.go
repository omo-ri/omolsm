package tree

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"omolsm/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	// 每条写入的 value 大小（字节），可按需调整
	testValueSize = 256
	// 预加载到 tree 的 key 数量（用于读/scan 基准）
	preloadCount = 200000
)

// randomBytes 生成随机字节数组
func randomBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	return b
}

func createTestTree(tb testing.TB, opts ...config.ConfigOption) *Tree {
	log.SetOutput(io.Discard)
	tb.Helper()
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("omolsm_bench_%d", time.Now().UnixNano()))

	conf, err := config.NewConfig(dir, opts...)
	if err != nil {
		tb.Fatalf("NewConfig failed: %v", err)
	}

	lsm, err := NewTree(conf)
	if err != nil {
		tb.Fatalf("NewTree failed: %v", err)
	}

	tb.Cleanup(func() {
		lsm.Close()
		// 若需调试可以注释掉下一行以保留数据
		os.RemoveAll(dir)
	})

	return lsm.(*Tree)
}

// dirSize 计算目录下所有文件的总字节数（递归）
func dirSize(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			// propagate error
			return err
		}
		if info == nil {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// countFiles 统计目录下文件数量（递归）
func countFiles(path string) (int64, error) {
	var cnt int64
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil {
			return nil
		}
		if !info.IsDir() {
			cnt++
		}
		return nil
	})
	return cnt, err
}

// estimateLogicalBytes 估算逻辑写入字节数：numOps * (keyLen + valueLen + metaOverhead)
func estimateLogicalBytes(numOps int64, keyLen int, valueLen int) int64 {
	const metaOverhead = 8
	per := int64(keyLen + valueLen + metaOverhead)
	return per * numOps
}

// reportSpaceMetrics 通过 b.ReportMetric 上报磁盘占用、逻辑字节与 space_amp 等指标
func reportSpaceMetrics(b *testing.B, t *Tree, numOps int64, keySize int, valueSize int) {
	b.Helper()
	dir := t.conf.Dir

	diskBytes, err := dirSize(dir)
	if err != nil {
		b.Logf("dirSize error: %v", err)
		diskBytes = 0
	}

	sstFiles, _ := countFiles(dir)

	logicalBytes := estimateLogicalBytes(numOps, keySize, valueSize)

	if numOps > 0 {
		diskPerOp := float64(diskBytes) / float64(numOps)
		logicalPerOp := float64(logicalBytes) / float64(numOps)
		spaceAmp := 0.0
		if logicalBytes > 0 {
			spaceAmp = float64(diskBytes) / float64(logicalBytes)
		}
		b.ReportMetric(diskPerOp, "disk_bytes/op")
		b.ReportMetric(logicalPerOp, "logical_bytes/op")
		b.ReportMetric(spaceAmp, "space_amp")
	}
	b.ReportMetric(float64(diskBytes), "disk_bytes")
	b.ReportMetric(float64(sstFiles), "sst_files")
}

// ================= Benchmarks =================

// Benchmark_Put: 纯写基准
func Benchmark_Put(b *testing.B) {
	tree := createTestTree(b)
	value := randomBytes(testValueSize)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key_%08d", i)
		if err := tree.Put(key, value); err != nil {
			b.Fatalf("Put error: %v", err)
		}
	}
	b.StopTimer()

	reportSpaceMetrics(b, tree, int64(b.N), len(fmt.Sprintf("key_%08d", 0)), testValueSize)
}

// Benchmark_Get_Random: 预加载后随机读
func Benchmark_Get_Random(b *testing.B) {
	tree := createTestTree(b)
	value := randomBytes(testValueSize)

	for i := 0; i < preloadCount; i++ {
		if err := tree.Put(fmt.Sprintf("key_%08d", i), value); err != nil {
			b.Fatalf("preload Put error: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := rand.Intn(preloadCount)
		_, _, _ = tree.Get(fmt.Sprintf("key_%08d", k))
	}
	b.StopTimer()

	reportSpaceMetrics(b, tree, int64(preloadCount), len(fmt.Sprintf("key_%08d", 0)), testValueSize)
}

// Benchmark_Get_Miss: 预加载后查询不存在的 key（用于测试 filter）
func Benchmark_Get_Miss(b *testing.B) {
	tree := createTestTree(b)
	value := randomBytes(testValueSize)

	for i := 0; i < preloadCount; i++ {
		if err := tree.Put(fmt.Sprintf("key_%08d", i), value); err != nil {
			b.Fatalf("preload Put error: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = tree.Get(fmt.Sprintf("not_exist_%08d", i))
	}
	b.StopTimer()

	reportSpaceMetrics(b, tree, int64(preloadCount), len(fmt.Sprintf("key_%08d", 0)), testValueSize)
}

// Benchmark_Scan_100: 范围扫描（窗口大小 100）
func Benchmark_Scan_100(b *testing.B) {
	tree := createTestTree(b)
	value := randomBytes(testValueSize)

	for i := 0; i < preloadCount; i++ {
		if err := tree.Put(fmt.Sprintf("key_%08d", i), value); err != nil {
			b.Fatalf("preload Put error: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := rand.Intn(preloadCount - 100)
		_, err := tree.Scan(fmt.Sprintf("key_%08d", start), fmt.Sprintf("key_%08d", start+100))
		if err != nil {
			b.Fatalf("Scan error: %v", err)
		}
	}
	b.StopTimer()

	reportSpaceMetrics(b, tree, int64(preloadCount), len(fmt.Sprintf("key_%08d", 0)), testValueSize)
}

// Benchmark_Delete: 删除（tombstone）基准
func Benchmark_Delete(b *testing.B) {
	tree := createTestTree(b)
	value := randomBytes(testValueSize)

	for i := 0; i < preloadCount; i++ {
		if err := tree.Put(fmt.Sprintf("key_%08d", i), value); err != nil {
			b.Fatalf("preload Put error: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := rand.Intn(preloadCount)
		if err := tree.Delete(fmt.Sprintf("key_%08d", k)); err != nil {
			b.Fatalf("Delete error: %v", err)
		}
	}
	b.StopTimer()

	reportSpaceMetrics(b, tree, int64(preloadCount), len(fmt.Sprintf("key_%08d", 0)), testValueSize)
}

// Benchmark_MixedWorkload: 混合负载（50% Put, 30% Get, 10% Delete, 10% Scan）
func Benchmark_MixedWorkload(b *testing.B) {
	tree := createTestTree(b)

	// 轻量预热
	for i := 0; i < preloadCount/10; i++ {
		if err := tree.Put(fmt.Sprintf("key_%08d", i), randomBytes(testValueSize)); err != nil {
			b.Fatalf("preload Put error: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := rand.Intn(100)
		key := fmt.Sprintf("key_%08d", rand.Intn(preloadCount))
		switch {
		case r < 50:
			if err := tree.Put(key, randomBytes(testValueSize)); err != nil {
				b.Fatalf("Put error: %v", err)
			}
		case r < 80:
			_, _, _ = tree.Get(key)
		case r < 90:
			if err := tree.Delete(key); err != nil {
				b.Fatalf("Delete error: %v", err)
			}
		default:
			start := rand.Intn(preloadCount - 50)
			_, _ = tree.Scan(fmt.Sprintf("key_%08d", start), fmt.Sprintf("key_%08d", start+50))
		}
	}
	b.StopTimer()

	reportSpaceMetrics(b, tree, int64(preloadCount), len(fmt.Sprintf("key_%08d", 0)), testValueSize)
}

// Benchmark_SSTSize_Variants: 比较不同 SSTSize 对写入与空间放大的影响
func Benchmark_SSTSize_Variants(b *testing.B) {
	sizes := []int{512 * 1024, 4 * 1024 * 1024, 16 * 1024 * 1024}
	for _, s := range sizes {
		sz := s
		b.Run(fmt.Sprintf("SST_%dKB", sz/1024), func(b *testing.B) {
			tree := createTestTree(b, config.WithSSTSize(sz))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				key := fmt.Sprintf("key_%08d", i)
				if err := tree.Put(key, randomBytes(testValueSize)); err != nil {
					b.Fatalf("Put error: %v", err)
				}
			}
			b.StopTimer()

			reportSpaceMetrics(b, tree, int64(b.N), len(fmt.Sprintf("key_%08d", 0)), testValueSize)
		})
	}
}
