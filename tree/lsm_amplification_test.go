package tree

import (
	"fmt"
	"math"
	"math/rand"
	"omolsm"
	"testing"
)

func generateRandomString(rng *rand.Rand, minLen, maxLen int) string {
	length := minLen + rng.Intn(maxLen-minLen+1)
	b := make([]byte, length)
	for i := range b {
		b[i] = 'a' + byte(rng.Intn(26))
	}
	return string(b)
}

func Test_WriteAmplification_Bounded(t *testing.T) {
	dir := t.TempDir()
	conf, err := omolsm.NewConfig(dir,
		omolsm.WithSSTSize(1024),
		omolsm.WithSSTNumPerLevel(4),
	)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := NewTree(conf)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	rng := rand.New(rand.NewSource(42))

	// 生成 key 和 value 池
	keysCount := 3000
	keys := make([]string, keysCount)
	values := make([][]byte, keysCount)
	for i := 0; i < keysCount; i++ {
		keys[i] = generateRandomString(rng, 5, 7)
		values[i] = []byte(generateRandomString(rng, 10, 20))
	}

	// 执行写入，统计用户逻辑写入量
	operations := 6000
	var bytesWrittenIdeal uint64
	for i := 0; i < operations; i++ {
		key := keys[rng.Intn(len(keys))]
		value := values[rng.Intn(len(values))]
		if err := tr.Put(key, value); err != nil {
			t.Fatal(err)
		}
		bytesWrittenIdeal += uint64(len(key) + len(value))
	}

	stats := tr.GetStats()

	// 写放大 = 实际磁盘写入 / 用户逻辑写入
	writeAmplification := float64(stats.BytesWritten) / float64(bytesWrittenIdeal)
	t.Logf("User written (ideal):  %d bytes", bytesWrittenIdeal)
	t.Logf("Disk written (actual): %d bytes", stats.BytesWritten)
	t.Logf("Write amplification:   %.2f", writeAmplification)

	// 写放大应该小于 log2(operations)
	limit := math.Log2(float64(operations))
	t.Logf("Limit (log2(%d)):      %.2f", operations, limit)
	if writeAmplification >= limit {
		t.Errorf("write amplification %.2f exceeds limit %.2f", writeAmplification, limit)
	}

	// 读取量应该小于写入量
	if stats.BytesRead > stats.BytesWritten {
		t.Errorf("bytes read %d exceeds bytes written %d", stats.BytesRead, stats.BytesWritten)
	}
}

func Test_WriteAmplification_ScalesWithData(t *testing.T) {
	sizes := []int{1000, 3000, 6000}
	var prevWA float64

	for _, ops := range sizes {
		dir := t.TempDir()
		conf, _ := omolsm.NewConfig(dir,
			omolsm.WithSSTSize(1024),
			omolsm.WithSSTNumPerLevel(4),
		)
		tr, _ := NewTree(conf)

		rng := rand.New(rand.NewSource(42))
		var bytesIdeal uint64

		for i := 0; i < ops; i++ {
			key := generateRandomString(rng, 5, 7)
			value := []byte(generateRandomString(rng, 10, 20))
			tr.Put(key, value)
			bytesIdeal += uint64(len(key) + len(value))
		}

		stats := tr.GetStats()
		wa := float64(stats.BytesWritten) / float64(bytesIdeal)
		t.Logf("ops=%d, WA=%.2f, ideal=%d, actual=%d", ops, wa, bytesIdeal, stats.BytesWritten)

		// 写放大不应该随数据量线性增长
		if prevWA > 0 && wa > prevWA*3 {
			t.Errorf("write amplification growing too fast: %d ops=%.2f, prev=%.2f", ops, wa, prevWA)
		}
		prevWA = wa
		tr.Close()
	}
}

func Test_ReadAmplification(t *testing.T) {
	dir := t.TempDir()
	conf, _ := omolsm.NewConfig(dir,
		omolsm.WithSSTSize(1024),
		omolsm.WithSSTNumPerLevel(4),
	)
	tr, _ := NewTree(conf)
	defer tr.Close()

	// 写入数据触发多次 flush
	rng := rand.New(rand.NewSource(42))
	count := 3000
	keys := make([]string, count)
	for i := 0; i < count; i++ {
		keys[i] = fmt.Sprintf("key_%06d", i)
		tr.Put(keys[i], []byte(generateRandomString(rng, 10, 20)))
	}

	// 重置读统计（如果支持的话），否则记录当前值
	statsBefore := tr.GetStats()

	// 执行 1000 次随机读
	reads := 1000
	for i := 0; i < reads; i++ {
		tr.Get(keys[rng.Intn(count)])
	}

	statsAfter := tr.GetStats()
	bytesRead := statsAfter.BytesRead - statsBefore.BytesRead

	avgBytesPerRead := float64(bytesRead) / float64(reads)
	t.Logf("Total reads: %d", reads)
	t.Logf("Total bytes read: %d", bytesRead)
	t.Logf("Avg bytes per read: %.2f", avgBytesPerRead)

	// 每次读不应该超过 SST 文件总大小
	if avgBytesPerRead > float64(statsAfter.BytesWritten) {
		t.Error("average bytes per read exceeds total data written")
	}
}
