package sst_io

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"omolsm"
	"omolsm/filter"
)

// ============================================================================
// Helper
// ============================================================================

func newTestWriter(t *testing.T, blockSize int) (*SSTWriter, *omolsm.Config) {
	t.Helper()
	conf := &omolsm.Config{
		Dir:              t.TempDir(),
		SSTFooterSize:    32,
		SSTDataBlockSize: blockSize,
		Filter:           filter.NewBloomFilter(1024),
	}
	w, err := NewSSTWriter("test.sst", conf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return w, conf
}

// ============================================================================
// NewSSTWriter
// ============================================================================

func TestNewSSTWriter(t *testing.T) {
	w, _ := newTestWriter(t, 4096)
	if w.Size() != 0 {
		t.Errorf("initial Size()=%d, want 0", w.Size())
	}
}

func TestNewSSTWriter_InvalidDir(t *testing.T) {
	conf := &omolsm.Config{
		Dir:              "/nonexistent/path/that/should/fail",
		SSTFooterSize:    32,
		SSTDataBlockSize: 64,
		Filter:           filter.NewBloomFilter(1024),
	}
	_, err := NewSSTWriter("test.sst", conf)
	if err == nil {
		t.Error("expected error for invalid dir")
	}
}

// ============================================================================
// Append + Finish
// ============================================================================

func TestSingleRecord(t *testing.T) {
	w, conf := newTestWriter(t, 4096)

	w.Append([]byte("key1"), []byte("val1"))
	size, blockToFilter, index := w.Finish()

	if size == 0 {
		t.Error("size=0")
	}
	if len(blockToFilter) == 0 {
		t.Error("blockToFilter is empty")
	}
	if len(index) < 1 {
		t.Errorf("index entries: %d", len(index))
	}

	info, err := os.Stat(filepath.Join(conf.Dir, "test.sst"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Error("file is empty")
	}
}

func TestMultipleBlocks(t *testing.T) {
	w, _ := newTestWriter(t, 32)

	for i := 0; i < 20; i++ {
		w.Append([]byte(fmt.Sprintf("key_%04d", i)), []byte(fmt.Sprintf("val_%04d", i)))
	}
	_, blockToFilter, index := w.Finish()

	if len(blockToFilter) < 2 {
		t.Errorf("expected multiple blocks, got %d", len(blockToFilter))
	}
	if len(index) < 2 {
		t.Errorf("expected multiple index entries, got %d", len(index))
	}
	for i := 1; i < len(index); i++ {
		if bytes.Compare(index[i-1].Key, index[i].Key) > 0 {
			t.Errorf("index not sorted: %q > %q", index[i-1].Key, index[i].Key)
		}
	}
}

func TestFinish_NoAppend(t *testing.T) {
	w, _ := newTestWriter(t, 4096)
	size, blockToFilter, _ := w.Finish()
	if size != 0 {
		t.Errorf("empty Finish size=%d", size)
	}
	if len(blockToFilter) != 0 {
		t.Errorf("empty Finish blockToFilter len=%d", len(blockToFilter))
	}
}

// ============================================================================
// Footer 格式
// ============================================================================

func TestFooterFormat(t *testing.T) {
	w, conf := newTestWriter(t, 4096)
	w.Append([]byte("k"), []byte("v"))
	w.Finish()
	w.Close()

	data, err := os.ReadFile(filepath.Join(conf.Dir, "test.sst"))
	if err != nil {
		t.Fatal(err)
	}

	footer := data[len(data)-conf.SSTFooterSize:]
	r := bytes.NewReader(footer)
	filterOffset, _ := binary.ReadUvarint(r)
	filterSize, _ := binary.ReadUvarint(r)
	indexOffset, _ := binary.ReadUvarint(r)
	indexSize, _ := binary.ReadUvarint(r)

	bodySize := uint64(len(data) - conf.SSTFooterSize)
	if filterOffset+filterSize > indexOffset {
		t.Errorf("filter overlaps index: filter=[%d,%d) index=%d", filterOffset, filterOffset+filterSize, indexOffset)
	}
	if indexOffset+indexSize > bodySize {
		t.Errorf("index exceeds body: [%d,%d) > %d", indexOffset, indexOffset+indexSize, bodySize)
	}
}

// ============================================================================
// blockToFilter 与 index 对应
// ============================================================================

func TestBlockToFilter_MatchesIndex(t *testing.T) {
	w, _ := newTestWriter(t, 32)
	for i := 0; i < 20; i++ {
		w.Append([]byte(fmt.Sprintf("key_%04d", i)), []byte("val"))
	}
	_, blockToFilter, index := w.Finish()

	for _, idx := range index {
		if idx.PrevBlockSize == 0 {
			continue
		}
		if _, ok := blockToFilter[idx.PrevBlockOffset]; !ok {
			t.Errorf("no filter for block at offset %d", idx.PrevBlockOffset)
		}
	}
}

// ============================================================================
// Size
// ============================================================================

func TestSize_Grows(t *testing.T) {
	w, _ := newTestWriter(t, 32)
	s0 := w.Size()
	for i := 0; i < 10; i++ {
		w.Append([]byte(fmt.Sprintf("key_%04d", i)), []byte("value"))
	}
	if w.Size() <= s0 {
		t.Error("Size did not grow")
	}
}

// ============================================================================
// prevKey 深拷贝验证（修复专项）
// ============================================================================

// TestPrevKey_CallerReusesBuffer 模拟迭代器复用同一个 buffer 的场景。
// 修复前：所有 index key 指向同一块内存，全部变成最后一个 key。
// 修复后：每条 index 持有独立拷贝。
func TestPrevKey_CallerReusesBuffer(t *testing.T) {
	w, _ := newTestWriter(t, 16) // 极小 block，每条记录都触发 block 切换

	buf := make([]byte, 20)
	val := []byte("v")

	expected := make([]string, 5)
	for i := 0; i < 5; i++ {
		s := fmt.Sprintf("key_%04d", i)
		expected[i] = s
		copy(buf, s)
		w.Append(buf[:len(s)], val)
	}

	_, _, index := w.Finish()

	var meaningful []*Index
	for _, idx := range index {
		if len(idx.Key) > 0 {
			meaningful = append(meaningful, idx)
		}
	}

	if len(meaningful) == 0 {
		t.Fatal("no meaningful index entries")
	}

	// 如果是引用，所有 key 都会是最后一个
	lastKey := expected[len(expected)-1]
	allSame := true
	for _, idx := range meaningful {
		if string(idx.Key) != lastKey {
			allSame = false
			break
		}
	}
	if allSame && len(meaningful) > 1 {
		t.Errorf("BUG: all %d index keys are %q — prevKey is a reference, not a copy",
			len(meaningful), lastKey)
	}
}

// TestPrevKey_MutationAfterAppend Append 后修改原始 key 不应影响内部状态。
func TestPrevKey_MutationAfterAppend(t *testing.T) {
	w, _ := newTestWriter(t, 16)

	key := []byte("original")
	w.Append(key, []byte("v"))

	// 调用方修改了 key
	copy(key, "XXXXXXXX")

	_, _, index := w.Finish()

	for _, idx := range index {
		if string(idx.Key) == "XXXXXXXX" {
			t.Error("BUG: index key was mutated by caller — prevKey is not a deep copy")
		}
	}
}
