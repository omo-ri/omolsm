package node_test

import (
	"encoding/binary"
	"fmt"
	"os"
	"testing"

	"omolsm/config"
	"omolsm/internal/lsm/node"
	"omolsm/internal/lsm/sst_io/reader"
	writer "omolsm/internal/lsm/sst_io/writer"
)

// buildTestSST 构建一个包含 numKeys 个 KV 对的 SST 文件, 返回 Node.
// key 格式: [featureID 4B][blockID 4B], 模拟倒排索引的 composite key.
// 共 numFeatures 个 feature, 每个 feature 有 numKeys/numFeatures 个 block.
func buildTestSST(b *testing.B, numKeys, numFeatures int) (*node.Node, *config.Config, string) {
	b.Helper()

	dir, err := os.MkdirTemp("", "bench-getrange-")
	if err != nil {
		b.Fatal(err)
	}

	conf, err := config.NewConfig(dir,
		config.WithSSTSize(64*1024*1024),    // 大一些避免触发 flush
		config.WithSSTDataBlockSize(4*1024), // 4KB block
	)
	if err != nil {
		b.Fatal(err)
	}

	file := "bench_0_1.sst"
	sstWriter, err := writer.NewSSTWriter(file, conf)
	if err != nil {
		b.Fatal(err)
	}

	blocksPerFeature := numKeys / numFeatures

	// 写入 KV 对: featureID 从 0 到 numFeatures-1, blockID 从 0 到 blocksPerFeature-1
	for fid := 0; fid < numFeatures; fid++ {
		for bid := 0; bid < blocksPerFeature; bid++ {
			key := make([]byte, 8)
			binary.BigEndian.PutUint32(key[0:4], uint32(fid))
			binary.BigEndian.PutUint32(key[4:8], uint32(bid))

			// value: 模拟一个小 bitmap (64 bytes)
			value := make([]byte, 64)
			binary.BigEndian.PutUint32(value[0:4], uint32(fid*1000+bid))

			sstWriter.Append(key, value)
		}
	}

	size, blockToFilter, index := sstWriter.Finish()
	sstWriter.Close()

	sstReader, err := reader.NewSSTReader(file, conf)
	if err != nil {
		b.Fatal(err)
	}

	n := node.NewNode(conf, file, sstReader, 0, 1, size, blockToFilter, index)
	return n, conf, dir
}

func encodeKey(featureID, blockID uint32) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint32(key[0:4], featureID)
	binary.BigEndian.PutUint32(key[4:8], blockID)
	return key
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkGetRange_Optimized 优化后: 二分定位 + 只读必要 block
func BenchmarkGetRange_Optimized(b *testing.B) {
	for _, tc := range []struct {
		numKeys     int
		numFeatures int
	}{
		{1000, 100},    // 小: 1000 KV, 100 feature, 每 feature 10 block
		{10000, 100},   // 中: 10000 KV, 100 feature, 每 feature 100 block
		{100000, 1000}, // 大: 100000 KV, 1000 feature, 每 feature 100 block
	} {
		name := fmt.Sprintf("keys=%d_features=%d", tc.numKeys, tc.numFeatures)
		b.Run(name, func(b *testing.B) {
			n, _, dir := buildTestSST(b, tc.numKeys, tc.numFeatures)
			defer os.RemoveAll(dir)
			defer n.Close()

			// 查询中间某个 feature 的全部 block
			targetFeature := uint32(tc.numFeatures / 2)
			startKey := encodeKey(targetFeature, 0)
			endKey := encodeKey(targetFeature+1, 0)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := n.GetRange(startKey, endKey)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkGetRange_Unoptimized 优化前: 全扫 + 过滤
func BenchmarkGetRange_Unoptimized(b *testing.B) {
	for _, tc := range []struct {
		numKeys     int
		numFeatures int
	}{
		{1000, 100},
		{10000, 100},
		{100000, 1000},
	} {
		name := fmt.Sprintf("keys=%d_features=%d", tc.numKeys, tc.numFeatures)
		b.Run(name, func(b *testing.B) {
			n, _, dir := buildTestSST(b, tc.numKeys, tc.numFeatures)
			defer os.RemoveAll(dir)
			defer n.Close()

			targetFeature := uint32(tc.numFeatures / 2)
			startKey := encodeKey(targetFeature, 0)
			endKey := encodeKey(targetFeature+1, 0)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := n.GetRangeUnoptimized(startKey, endKey)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkGetRange_Comparison 直接对比输出
func BenchmarkGetRange_Comparison(b *testing.B) {
	numKeys := 50000
	numFeatures := 500

	n, _, dir := buildTestSST(b, numKeys, numFeatures)
	defer os.RemoveAll(dir)
	defer n.Close()

	targetFeature := uint32(numFeatures / 2)
	startKey := encodeKey(targetFeature, 0)
	endKey := encodeKey(targetFeature+1, 0)

	b.Run("Optimized", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = n.GetRange(startKey, endKey)
		}
	})

	b.Run("Unoptimized", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = n.GetRangeUnoptimized(startKey, endKey)
		}
	})
}

// TestGetRange_Correctness 确保优化后结果和优化前一致
func TestGetRange_Correctness(t *testing.T) {
	// 直接用 benchmark helper，传 testing.B 的 wrapper
	dir, err := os.MkdirTemp("", "test-getrange-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	conf, err := config.NewConfig(dir,
		config.WithSSTSize(64*1024*1024),
		config.WithSSTDataBlockSize(4*1024),
	)
	if err != nil {
		t.Fatal(err)
	}

	numKeys := 5000
	numFeatures := 50

	file := "test_0_1.sst"
	sstWriter, err := writer.NewSSTWriter(file, conf)
	if err != nil {
		t.Fatal(err)
	}

	blocksPerFeature := numKeys / numFeatures
	for fid := 0; fid < numFeatures; fid++ {
		for bid := 0; bid < blocksPerFeature; bid++ {
			key := encodeKey(uint32(fid), uint32(bid))
			value := make([]byte, 64)
			binary.BigEndian.PutUint32(value[0:4], uint32(fid*1000+bid))
			sstWriter.Append(key, value)
		}
	}

	size, blockToFilter, index := sstWriter.Finish()
	sstWriter.Close()

	sstReader, err := reader.NewSSTReader(file, conf)
	if err != nil {
		t.Fatal(err)
	}

	n := node.NewNode(conf, file, sstReader, 0, 1, size, blockToFilter, index)
	defer n.Close()

	// 对每个 feature 比较两种实现的结果
	for fid := 0; fid < numFeatures; fid++ {
		startKey := encodeKey(uint32(fid), 0)
		endKey := encodeKey(uint32(fid+1), 0)

		optimized, err := n.GetRange(startKey, endKey)
		if err != nil {
			t.Fatalf("feature %d optimized error: %v", fid, err)
		}

		unoptimized, err := n.GetRangeUnoptimized(startKey, endKey)
		if err != nil {
			t.Fatalf("feature %d unoptimized error: %v", fid, err)
		}

		if len(optimized) != len(unoptimized) {
			t.Fatalf("feature %d: optimized returned %d results, unoptimized returned %d",
				fid, len(optimized), len(unoptimized))
		}

		for i := range optimized {
			if string(optimized[i].Key) != string(unoptimized[i].Key) {
				t.Fatalf("feature %d, result %d: key mismatch", fid, i)
			}
			if string(optimized[i].Value) != string(unoptimized[i].Value) {
				t.Fatalf("feature %d, result %d: value mismatch", fid, i)
			}
		}
	}

	// 边界: 查不存在的 feature
	startKey := encodeKey(uint32(numFeatures+100), 0)
	endKey := encodeKey(uint32(numFeatures+101), 0)
	result, err := n.GetRange(startKey, endKey)
	if err != nil {
		t.Fatalf("non-existent feature error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 results for non-existent feature, got %d", len(result))
	}
}
