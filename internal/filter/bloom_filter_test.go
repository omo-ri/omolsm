package filter

import (
	"encoding/binary"
	"fmt"
	"math"
	"testing"
)

// ============================================================================
// 基础功能
// ============================================================================

func TestNewBloomFilter(t *testing.T) {
	bf := NewBloomFilter(1024)
	if bf.KeyLen() != 0 {
		t.Errorf("expected KeyLen=0, got %d", bf.KeyLen())
	}
}

func TestAdd_And_KeyLen(t *testing.T) {
	bf := NewBloomFilter(1024)
	for i := 0; i < 5; i++ {
		bf.Add([]byte(fmt.Sprintf("key%d", i)))
	}
	if bf.KeyLen() != 5 {
		t.Errorf("expected KeyLen=5, got %d", bf.KeyLen())
	}
}

// ============================================================================
// 核心属性：无假阴性
// ============================================================================

func TestExist_NoFalseNegatives(t *testing.T) {
	bf := NewBloomFilter(4096)
	keys := make([][]byte, 200)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("key_%04d", i))
		bf.Add(keys[i])
	}
	bitmap := bf.Hash()

	for i, key := range keys {
		if !bf.Exist(bitmap, key) {
			t.Fatalf("false negative for key[%d]=%q", i, key)
		}
	}
}

// ============================================================================
// 核心修复验证：k 值一致性
// ============================================================================

func TestExist_AfterReset_UsesEncodedK(t *testing.T) {
	bf := NewBloomFilter(4096)
	keys := make([][]byte, 100)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("key_%04d", i))
		bf.Add(keys[i])
	}
	bitmap := bf.Hash()
	bf.Reset()

	for i, key := range keys {
		if !bf.Exist(bitmap, key) {
			t.Fatalf("false negative after Reset for key[%d]", i)
		}
	}
}

func TestExist_NewInstance_UsesEncodedK(t *testing.T) {
	bfWrite := NewBloomFilter(4096)
	for i := 0; i < 50; i++ {
		bfWrite.Add([]byte(fmt.Sprintf("k%d", i)))
	}
	bitmap := bfWrite.Hash()

	bfRead := NewBloomFilter(4096)
	if !bfRead.Exist(bitmap, []byte("k0")) {
		t.Error("new instance failed to query with existing bitmap")
	}
}

// ============================================================================
// Hash 格式
// ============================================================================

func TestHash_Format(t *testing.T) {
	bf := NewBloomFilter(1024)
	for i := 0; i < 100; i++ {
		bf.Add([]byte(fmt.Sprintf("k%d", i)))
	}
	bitmap := bf.Hash()

	// 长度 = 1 (k byte) + ceil(m/8)
	if len(bitmap) != 1+128 {
		t.Errorf("expected len=%d, got %d", 1+128, len(bitmap))
	}
	// 首字节 = k
	if uint32(bitmap[0]) != bf.calcK() {
		t.Errorf("bitmap[0]=%d, expected k=%d", bitmap[0], bf.calcK())
	}
}

func TestHash_Deterministic(t *testing.T) {
	bf := NewBloomFilter(1024)
	bf.Add([]byte("key1"))
	bf.Add([]byte("key2"))

	b1, b2 := bf.Hash(), bf.Hash()
	for i := range b1 {
		if b1[i] != b2[i] {
			t.Fatalf("non-deterministic at byte %d", i)
		}
	}
}

// ============================================================================
// 假阳性率
// ============================================================================

func TestFalsePositiveRate(t *testing.T) {
	n := 5000
	m := n * 10
	bf := NewBloomFilter(m)

	for i := 0; i < n; i++ {
		key := make([]byte, 8)
		binary.LittleEndian.PutUint64(key, uint64(i))
		bf.Add(key)
	}
	bitmap := bf.Hash()

	fp := 0
	testN := 50000
	for i := n; i < n+testN; i++ {
		key := make([]byte, 8)
		binary.LittleEndian.PutUint64(key, uint64(i))
		if bf.Exist(bitmap, key) {
			fp++
		}
	}

	fpRate := float64(fp) / float64(testN)
	theoretical := math.Pow(1-math.Exp(-float64(bf.calcK())*float64(n)/float64(m)), float64(bf.calcK()))
	t.Logf("FP rate: actual=%.4f theoretical=%.4f k=%d", fpRate, theoretical, bf.calcK())

	if fpRate > 0.05 {
		t.Errorf("FP rate %.4f exceeds 5%%", fpRate)
	}
}

// ============================================================================
// Reset
// ============================================================================

func TestReset(t *testing.T) {
	bf := NewBloomFilter(1024)
	bf.Add([]byte("key"))
	bf.Reset()

	if bf.KeyLen() != 0 {
		t.Errorf("KeyLen after Reset: %d", bf.KeyLen())
	}
	// data 部分应全零
	bitmap := bf.Hash()
	for i := 1; i < len(bitmap); i++ {
		if bitmap[i] != 0 {
			t.Fatalf("bitmap[%d]=%02x after Reset", i, bitmap[i])
		}
	}
}

// ============================================================================
// calcK
// ============================================================================

func TestCalcK(t *testing.T) {
	tests := []struct {
		m, n     int
		expected uint32
	}{
		{1000, 100, 7},  // round(10 * ln2) = 7
		{2000, 100, 14}, // round(20 * ln2) = 14
		{50, 100, 1},    // round(0.346) = 0 → clamp to 1
	}
	for _, tt := range tests {
		bf := NewBloomFilter(tt.m)
		for i := 0; i < tt.n; i++ {
			bf.Add([]byte(fmt.Sprintf("k%d", i)))
		}
		if k := bf.calcK(); k != tt.expected {
			t.Errorf("m=%d n=%d: calcK()=%d, want %d", tt.m, tt.n, k, tt.expected)
		}
	}
}

func TestCalcK_Bounds(t *testing.T) {
	// 下限
	bf := NewBloomFilter(1024)
	if bf.calcK() != 1 {
		t.Errorf("n=0: calcK()=%d, want 1", bf.calcK())
	}
	// 上限
	bf2 := NewBloomFilter(1000000)
	bf2.Add([]byte("x"))
	if bf2.calcK() != maxK {
		t.Errorf("extreme m/n: calcK()=%d, want %d", bf2.calcK(), maxK)
	}
}

// ============================================================================
// 边界场景
// ============================================================================

func TestExist_EdgeCases(t *testing.T) {
	bf := NewBloomFilter(1024)

	// nil / 空 / k=0 的 bitmap 都应返回 false，不 panic
	for _, bm := range [][]byte{nil, {}, {0, 0xFF}} {
		if bf.Exist(bm, []byte("key")) {
			t.Errorf("should return false for bitmap %v", bm)
		}
	}
}

func TestSmallM(t *testing.T) {
	bf := NewBloomFilter(1)
	bf.Add([]byte("key"))
	bitmap := bf.Hash()

	// m=1: 任何 key 都会命中唯一的 bit
	if !bf.Exist(bitmap, []byte("anything")) {
		t.Error("single-bit filter should always return true")
	}
}

func TestAdd_EmptyKey(t *testing.T) {
	bf := NewBloomFilter(1024)
	bf.Add([]byte{})
	bitmap := bf.Hash()
	if !bf.Exist(bitmap, []byte{}) {
		t.Error("false negative for empty key")
	}
}

// ============================================================================
// Benchmark
// ============================================================================

func BenchmarkAdd(b *testing.B) {
	bf := NewBloomFilter(100000)
	key := []byte("benchmark_key")
	for i := 0; i < b.N; i++ {
		bf.Add(key)
	}
}

func BenchmarkExist(b *testing.B) {
	bf := NewBloomFilter(100000)
	for i := 0; i < 1000; i++ {
		bf.Add([]byte(fmt.Sprintf("key_%06d", i)))
	}
	bitmap := bf.Hash()
	key := []byte("lookup_key")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bf.Exist(bitmap, key)
	}
}
