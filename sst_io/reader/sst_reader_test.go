package reader

import (
	"bytes"
	"fmt"
	"testing"

	"omolsm"
	"omolsm/filter"
	sst_io "omolsm/sst_io/writer"
)

// ============================================================================
// Helper
// ============================================================================

// writeSST 用 SSTWriter 写入一批有序 KV，返回 writer 的产出物用于比对
func writeSST(t *testing.T, conf *omolsm.Config, kvs []KV) (size uint64, blockToFilter map[uint64][]byte, index []*sst_io.Index) {
	t.Helper()
	w, err := sst_io.NewSSTWriter("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range kvs {
		w.Append(kv.Key, kv.Value)
	}
	size, blockToFilter, index = w.Finish()
	w.Close()
	return
}

func newTestConf(t *testing.T, blockSize int) *omolsm.Config {
	t.Helper()
	return &omolsm.Config{
		Dir:              t.TempDir(),
		SSTFooterSize:    32,
		SSTDataBlockSize: blockSize,
		Filter:           filter.NewBloomFilter(1024),
	}
}

func makeKVs(n int) []KV {
	kvs := make([]KV, n)
	for i := range kvs {
		kvs[i] = KV{
			Key:   []byte(fmt.Sprintf("key_%06d", i)),
			Value: []byte(fmt.Sprintf("value_%06d", i)),
		}
	}
	return kvs
}

// ============================================================================
// NewSSTReader
// ============================================================================

func TestNewSSTReader(t *testing.T) {
	conf := newTestConf(t, 4096)
	writeSST(t, conf, makeKVs(1))

	r, err := NewSSTReader("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
}

func TestNewSSTReader_InvalidFile(t *testing.T) {
	conf := newTestConf(t, 4096)
	_, err := NewSSTReader("nonexistent.sst", conf)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// ============================================================================
// ReadFooter
// ============================================================================

func TestReadFooter(t *testing.T) {
	conf := newTestConf(t, 4096)
	writeSST(t, conf, makeKVs(5))

	r, err := NewSSTReader("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	if err := r.ReadFooter(); err != nil {
		t.Fatal(err)
	}

	// 基本合理性：filter 在 index 之前，区间不重叠
	if r.filterOffset > r.indexOffset {
		t.Errorf("filterOffset(%d) > indexOffset(%d)", r.filterOffset, r.indexOffset)
	}
	if r.filterOffset+r.filterSize > r.indexOffset {
		t.Errorf("filter overlaps index")
	}
	if r.filterSize == 0 {
		t.Error("filterSize=0")
	}
	if r.indexSize == 0 {
		t.Error("indexSize=0")
	}
}

// ============================================================================
// ReadData: 端到端验证（核心）
// ============================================================================

func TestReadData_AllRecordsMatch(t *testing.T) {
	conf := newTestConf(t, 4096)
	kvs := makeKVs(20)
	writeSST(t, conf, kvs)

	r, err := NewSSTReader("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	data, err := r.ReadData()
	if err != nil {
		t.Fatal(err)
	}

	if len(data) != len(kvs) {
		t.Fatalf("expected %d records, got %d", len(kvs), len(data))
	}
	for i := range kvs {
		if !bytes.Equal(data[i].Key, kvs[i].Key) {
			t.Errorf("key[%d]: got %q, want %q", i, data[i].Key, kvs[i].Key)
		}
		if !bytes.Equal(data[i].Value, kvs[i].Value) {
			t.Errorf("value[%d]: got %q, want %q", i, data[i].Value, kvs[i].Value)
		}
	}
}

func TestReadData_MultipleBlocks(t *testing.T) {
	conf := newTestConf(t, 32) // 小 block 触发多次切换
	kvs := makeKVs(50)
	writeSST(t, conf, kvs)

	r, err := NewSSTReader("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	data, err := r.ReadData()
	if err != nil {
		t.Fatal(err)
	}

	if len(data) != len(kvs) {
		t.Fatalf("expected %d records, got %d", len(kvs), len(data))
	}
	for i := range kvs {
		if !bytes.Equal(data[i].Key, kvs[i].Key) {
			t.Errorf("key[%d]: got %q, want %q", i, data[i].Key, kvs[i].Key)
		}
	}
}

func TestReadData_SingleRecord(t *testing.T) {
	conf := newTestConf(t, 4096)
	kvs := []KV{{Key: []byte("only_key"), Value: []byte("only_val")}}
	writeSST(t, conf, kvs)

	r, err := NewSSTReader("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	data, err := r.ReadData()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 || !bytes.Equal(data[0].Key, []byte("only_key")) {
		t.Errorf("unexpected data: %+v", data)
	}
}

// ============================================================================
// ReadFilter
// ============================================================================

func TestReadFilter(t *testing.T) {
	conf := newTestConf(t, 32)
	_, writerFilter, _ := writeSST(t, conf, makeKVs(20))

	// 需要新的 filter 实例给 reader 用
	conf.Filter = filter.NewBloomFilter(1024)
	r, err := NewSSTReader("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	readFilter, err := r.ReadFilter()
	if err != nil {
		t.Fatal(err)
	}

	// 读出的 filter 条目数应与写入时一致
	if len(readFilter) != len(writerFilter) {
		t.Errorf("filter count: got %d, want %d", len(readFilter), len(writerFilter))
	}

	// 每个 offset 对应的 bitmap 应一致
	for offset, expectedBitmap := range writerFilter {
		got, ok := readFilter[offset]
		if !ok {
			t.Errorf("missing filter for offset %d", offset)
			continue
		}
		if !bytes.Equal(got, expectedBitmap) {
			t.Errorf("filter bitmap mismatch at offset %d", offset)
		}
	}
}

// ============================================================================
// ReadIndex
// ============================================================================

func TestReadIndex(t *testing.T) {
	conf := newTestConf(t, 32)
	_, _, writerIndex := writeSST(t, conf, makeKVs(20))

	conf.Filter = filter.NewBloomFilter(1024)
	r, err := NewSSTReader("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	readIndex, err := r.ReadIndex()
	if err != nil {
		t.Fatal(err)
	}

	if len(readIndex) != len(writerIndex) {
		t.Fatalf("index count: got %d, want %d", len(readIndex), len(writerIndex))
	}

	for i := range writerIndex {
		if !bytes.Equal(readIndex[i].Key, writerIndex[i].Key) {
			t.Errorf("index[%d].Key: got %q, want %q", i, readIndex[i].Key, writerIndex[i].Key)
		}
		if readIndex[i].PrevBlockOffset != writerIndex[i].PrevBlockOffset {
			t.Errorf("index[%d].Offset: got %d, want %d", i, readIndex[i].PrevBlockOffset, writerIndex[i].PrevBlockOffset)
		}
		if readIndex[i].PrevBlockSize != writerIndex[i].PrevBlockSize {
			t.Errorf("index[%d].Size: got %d, want %d", i, readIndex[i].PrevBlockSize, writerIndex[i].PrevBlockSize)
		}
	}

	// index key 应有序
	for i := 1; i < len(readIndex); i++ {
		if bytes.Compare(readIndex[i-1].Key, readIndex[i].Key) > 0 {
			t.Errorf("index not sorted: %q > %q", readIndex[i-1].Key, readIndex[i].Key)
		}
	}
}

// ============================================================================
// Size
// ============================================================================

func TestSize(t *testing.T) {
	conf := newTestConf(t, 4096)
	writeSST(t, conf, makeKVs(10))

	r, err := NewSSTReader("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	size, err := r.Size()
	if err != nil {
		t.Fatal(err)
	}
	if size == 0 {
		t.Error("size=0")
	}
}

// ============================================================================
// ReadRecord: 前缀压缩解码
// ============================================================================

func TestReadRecord_PrefixDecoding(t *testing.T) {
	conf := newTestConf(t, 4096)
	// 用有共同前缀的 key 验证前缀压缩解码
	kvs := []KV{
		{Key: []byte("user:001"), Value: []byte("Alice")},
		{Key: []byte("user:002"), Value: []byte("Bob")},
		{Key: []byte("user:003"), Value: []byte("Charlie")},
	}
	writeSST(t, conf, kvs)

	r, err := NewSSTReader("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	data, err := r.ReadData()
	if err != nil {
		t.Fatal(err)
	}

	for i := range kvs {
		if !bytes.Equal(data[i].Key, kvs[i].Key) {
			t.Errorf("key[%d]: got %q, want %q", i, data[i].Key, kvs[i].Key)
		}
		if !bytes.Equal(data[i].Value, kvs[i].Value) {
			t.Errorf("value[%d]: got %q, want %q", i, data[i].Value, kvs[i].Value)
		}
	}
}

// ============================================================================
// 多次读取（Seek 正确性）
// ============================================================================

func TestMultipleReads(t *testing.T) {
	conf := newTestConf(t, 64)
	kvs := makeKVs(30)
	writeSST(t, conf, kvs)

	conf.Filter = filter.NewBloomFilter(1024)
	r, err := NewSSTReader("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// 连续读 footer → filter → index → data，验证 Seek 不互相干扰
	if err := r.ReadFooter(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadFilter(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadIndex(); err != nil {
		t.Fatal(err)
	}
	data, err := r.ReadData()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != len(kvs) {
		t.Errorf("expected %d records, got %d", len(kvs), len(data))
	}
}
