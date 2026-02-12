package block

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

// ============================================================================
// 辅助函数
// ============================================================================

// decodeRecord 将 block 中编码的一条 KV 记录解码还原出 key 和 value。
// prevKey 用于前缀压缩还原。返回解码后的 key、value 和剩余未消费的字节。
func decodeRecord(data []byte, prevKey []byte) (key, value, remaining []byte, err error) {
	r := bytes.NewReader(data)

	sharedPrefixLen, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, nil, nil, err
	}
	diffKeyLen, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, nil, nil, err
	}
	valueLen, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, nil, nil, err
	}

	diffKey := make([]byte, diffKeyLen)
	if _, err := io.ReadFull(r, diffKey); err != nil {
		return nil, nil, nil, err
	}

	val := make([]byte, valueLen)
	if _, err := io.ReadFull(r, val); err != nil {
		return nil, nil, nil, err
	}

	// 还原完整 key = prevKey[:sharedPrefixLen] + diffKey
	fullKey := make([]byte, 0, int(sharedPrefixLen)+int(diffKeyLen))
	fullKey = append(fullKey, prevKey[:sharedPrefixLen]...)
	fullKey = append(fullKey, diffKey...)

	// 剩余字节
	rem := make([]byte, r.Len())
	if len(rem) > 0 {
		if _, err := io.ReadFull(r, rem); err != nil {
			return nil, nil, nil, err
		}
	}

	return fullKey, val, rem, nil
}

// decodeAllRecords 从 block 的原始字节中解码出全部 KV 对
func decodeAllRecords(data []byte) (keys, values [][]byte, err error) {
	var prevKey []byte
	remaining := data
	for len(remaining) > 0 {
		var key, value []byte
		key, value, remaining, err = decodeRecord(remaining, prevKey)
		if err != nil {
			return nil, nil, err
		}
		keys = append(keys, key)
		values = append(values, value)
		prevKey = key
	}
	return keys, values, nil
}

// ============================================================================
// 测试用例
// ============================================================================

// ---------- NewBlock ----------

func TestNewBlock_InitialState(t *testing.T) {
	b := NewBlock(nil)

	if b == nil {
		t.Fatal("NewBlock returned nil")
	}
	if b.Size() != 0 {
		t.Errorf("expected initial Size()=0, got %d", b.Size())
	}
	if b.GetKvsCnt() != 0 {
		t.Errorf("expected initial KvsCnt=0, got %d", b.GetKvsCnt())
	}
	if len(b.ToBytes()) != 0 {
		t.Errorf("expected empty ToBytes(), got len=%d", len(b.ToBytes()))
	}
}

// ---------- Append: 单条记录 ----------

func TestAppend_SingleRecord(t *testing.T) {
	b := NewBlock(nil)
	key := []byte("hello")
	val := []byte("world")

	b.Append(key, val)

	if b.GetKvsCnt() != 1 {
		t.Errorf("expected KvsCnt=1, got %d", b.GetKvsCnt())
	}
	if b.Size() == 0 {
		t.Error("expected Size()>0 after Append")
	}

	// 解码验证
	keys, values, err := decodeAllRecords(b.ToBytes())
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 record, got %d", len(keys))
	}
	if !bytes.Equal(keys[0], key) {
		t.Errorf("key mismatch: got %q, want %q", keys[0], key)
	}
	if !bytes.Equal(values[0], val) {
		t.Errorf("value mismatch: got %q, want %q", values[0], val)
	}
}

// ---------- Append: 无公共前缀 ----------

func TestAppend_NoSharedPrefix(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("apple"), []byte("v1"))
	b.Append([]byte("banana"), []byte("v2"))

	if b.GetKvsCnt() != 2 {
		t.Errorf("expected KvsCnt=2, got %d", b.GetKvsCnt())
	}

	keys, values, err := decodeAllRecords(b.ToBytes())
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 records, got %d", len(keys))
	}
	if !bytes.Equal(keys[0], []byte("apple")) {
		t.Errorf("key[0] mismatch: %q", keys[0])
	}
	if !bytes.Equal(keys[1], []byte("banana")) {
		t.Errorf("key[1] mismatch: %q", keys[1])
	}
	if !bytes.Equal(values[0], []byte("v1")) {
		t.Errorf("value[0] mismatch: %q", values[0])
	}
	if !bytes.Equal(values[1], []byte("v2")) {
		t.Errorf("value[1] mismatch: %q", values[1])
	}
}

// ---------- Append: 有公共前缀（前缀压缩核心场景）----------

func TestAppend_SharedPrefix(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("user:001"), []byte("Alice"))
	b.Append([]byte("user:002"), []byte("Bob"))
	b.Append([]byte("user:003"), []byte("Charlie"))

	if b.GetKvsCnt() != 3 {
		t.Errorf("expected KvsCnt=3, got %d", b.GetKvsCnt())
	}

	keys, values, err := decodeAllRecords(b.ToBytes())
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 records, got %d", len(keys))
	}

	expectedKeys := []string{"user:001", "user:002", "user:003"}
	expectedVals := []string{"Alice", "Bob", "Charlie"}
	for i := range expectedKeys {
		if string(keys[i]) != expectedKeys[i] {
			t.Errorf("key[%d]: got %q, want %q", i, keys[i], expectedKeys[i])
		}
		if string(values[i]) != expectedVals[i] {
			t.Errorf("value[%d]: got %q, want %q", i, values[i], expectedVals[i])
		}
	}

	// 验证前缀压缩确实节省了空间：
	// 如果没有前缀压缩，总数据量会更大
	// "user:001" + "user:002" + "user:003" = 24 bytes 的 key
	// 有前缀压缩后，第2/3条只存 diff 部分 ("002", "003" 各3字节) 而非完整 key
	totalKeyBytesWithoutCompression := len("user:001") + len("user:002") + len("user:003")
	// block 中 key 数据应小于无压缩的总量（加上 varint 头部后依然应该更紧凑或至少不会更大很多）
	if b.Size() >= totalKeyBytesWithoutCompression+len("Alice")+len("Bob")+len("Charlie")+30 {
		t.Errorf("prefix compression does not seem to be saving space, block size=%d", b.Size())
	}
}

// ---------- Append: 完全相同的 key ----------

func TestAppend_IdenticalKeys(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("same"), []byte("v1"))
	b.Append([]byte("same"), []byte("v2"))

	keys, values, err := decodeAllRecords(b.ToBytes())
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 records, got %d", len(keys))
	}
	if string(keys[0]) != "same" || string(keys[1]) != "same" {
		t.Errorf("keys mismatch: %q, %q", keys[0], keys[1])
	}
	if string(values[0]) != "v1" || string(values[1]) != "v2" {
		t.Errorf("values mismatch: %q, %q", values[0], values[1])
	}
}

// ---------- Append: 空 key / 空 value ----------

func TestAppend_EmptyKeyAndValue(t *testing.T) {
	b := NewBlock(nil)

	// 空 key + 非空 value
	b.Append([]byte{}, []byte("val"))
	// 非空 key + 空 value
	b.Append([]byte("key"), []byte{})
	// 空 key + 空 value
	b.Append([]byte{}, []byte{})

	if b.GetKvsCnt() != 3 {
		t.Errorf("expected KvsCnt=3, got %d", b.GetKvsCnt())
	}

	keys, values, err := decodeAllRecords(b.ToBytes())
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 records, got %d", len(keys))
	}
	if len(keys[0]) != 0 {
		t.Errorf("key[0] should be empty, got %q", keys[0])
	}
	if string(values[0]) != "val" {
		t.Errorf("value[0] mismatch: %q", values[0])
	}
	if string(keys[1]) != "key" {
		t.Errorf("key[1] mismatch: %q", keys[1])
	}
	if len(values[1]) != 0 {
		t.Errorf("value[1] should be empty, got %q", values[1])
	}
	if len(keys[2]) != 0 {
		t.Errorf("key[2] should be empty, got %q", keys[2])
	}
	if len(values[2]) != 0 {
		t.Errorf("value[2] should be empty, got %q", values[2])
	}
}

// ---------- Append: 大量记录 ----------

func TestAppend_ManyRecords(t *testing.T) {
	b := NewBlock(nil)
	n := 1000
	for i := 0; i < n; i++ {
		key := []byte("key_" + padInt(i, 6))
		val := []byte("value_" + padInt(i, 6))
		b.Append(key, val)
	}

	if b.GetKvsCnt() != n {
		t.Errorf("expected KvsCnt=%d, got %d", n, b.GetKvsCnt())
	}

	keys, values, err := decodeAllRecords(b.ToBytes())
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(keys) != n {
		t.Fatalf("expected %d records, got %d", n, len(keys))
	}

	// 抽查首尾几条
	if string(keys[0]) != "key_000000" {
		t.Errorf("first key: %q", keys[0])
	}
	if string(values[0]) != "value_000000" {
		t.Errorf("first value: %q", values[0])
	}
	if string(keys[n-1]) != "key_000999" {
		t.Errorf("last key: %q", keys[n-1])
	}
	if string(values[n-1]) != "value_000999" {
		t.Errorf("last value: %q", values[n-1])
	}
}

// ---------- Append: 二进制 key/value ----------

func TestAppend_BinaryData(t *testing.T) {
	b := NewBlock(nil)
	key := []byte{0x00, 0xFF, 0x01, 0xFE}
	val := []byte{0xDE, 0xAD, 0xBE, 0xEF}

	b.Append(key, val)

	keys, values, err := decodeAllRecords(b.ToBytes())
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !bytes.Equal(keys[0], key) {
		t.Errorf("binary key mismatch: got %x, want %x", keys[0], key)
	}
	if !bytes.Equal(values[0], val) {
		t.Errorf("binary value mismatch: got %x, want %x", values[0], val)
	}
}

// ---------- Append: key 是前一个 key 的前缀 ----------

func TestAppend_KeyIsPrefixOfPrev(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("abcdef"), []byte("v1"))
	b.Append([]byte("abc"), []byte("v2")) // key 比前一个短，是其前缀

	keys, values, err := decodeAllRecords(b.ToBytes())
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(keys[0]) != "abcdef" {
		t.Errorf("key[0]: %q", keys[0])
	}
	if string(keys[1]) != "abc" {
		t.Errorf("key[1]: %q", keys[1])
	}
	if string(values[0]) != "v1" || string(values[1]) != "v2" {
		t.Errorf("values: %q, %q", values[0], values[1])
	}
}

// ---------- Size ----------

func TestSize_GrowsWithAppend(t *testing.T) {
	b := NewBlock(nil)

	s0 := b.Size()
	if s0 != 0 {
		t.Errorf("initial size should be 0, got %d", s0)
	}

	b.Append([]byte("k1"), []byte("v1"))
	s1 := b.Size()
	if s1 <= s0 {
		t.Error("size should grow after first Append")
	}

	b.Append([]byte("k2"), []byte("v2"))
	s2 := b.Size()
	if s2 <= s1 {
		t.Error("size should grow after second Append")
	}
}

// ---------- ToBytes ----------

func TestToBytes_ReturnsConsistentData(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("key"), []byte("value"))

	data1 := b.ToBytes()
	data2 := b.ToBytes()

	if !bytes.Equal(data1, data2) {
		t.Error("consecutive ToBytes() calls should return identical data")
	}
}

func TestToBytes_LenMatchesSize(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("hello"), []byte("world"))
	b.Append([]byte("foo"), []byte("bar"))

	if len(b.ToBytes()) != b.Size() {
		t.Errorf("len(ToBytes())=%d != Size()=%d", len(b.ToBytes()), b.Size())
	}
}

// ---------- FlushTo ----------

func TestFlushTo_WritesAllData(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("key1"), []byte("value1"))
	b.Append([]byte("key2"), []byte("value2"))

	expectedData := make([]byte, len(b.ToBytes()))
	copy(expectedData, b.ToBytes())
	expectedSize := b.Size()

	var dest bytes.Buffer
	n, err := b.FlushTo(&dest)

	if err != nil {
		t.Fatalf("FlushTo returned error: %v", err)
	}
	if int(n) != expectedSize {
		t.Errorf("FlushTo returned n=%d, expected %d", n, expectedSize)
	}
	if !bytes.Equal(dest.Bytes(), expectedData) {
		t.Error("FlushTo wrote different data than ToBytes()")
	}
}

func TestFlushTo_ClearsBlock(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("key"), []byte("value"))

	var dest bytes.Buffer
	_, _ = b.FlushTo(&dest)

	// flush 后 block 应被清空
	if b.Size() != 0 {
		t.Errorf("expected Size()=0 after FlushTo, got %d", b.Size())
	}
	if b.GetKvsCnt() != 0 {
		t.Errorf("expected KvsCnt=0 after FlushTo, got %d", b.GetKvsCnt())
	}
	if len(b.ToBytes()) != 0 {
		t.Errorf("expected empty ToBytes() after FlushTo, got len=%d", len(b.ToBytes()))
	}
}

func TestFlushTo_BlockReusableAfterFlush(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("old_key"), []byte("old_val"))

	var dest1 bytes.Buffer
	_, _ = b.FlushTo(&dest1)

	// flush 后写入新数据，前缀压缩应从头开始（无 prevKey）
	b.Append([]byte("new_key"), []byte("new_val"))

	if b.GetKvsCnt() != 1 {
		t.Errorf("expected KvsCnt=1 after reuse, got %d", b.GetKvsCnt())
	}

	keys, values, err := decodeAllRecords(b.ToBytes())
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(keys[0]) != "new_key" {
		t.Errorf("key after reuse: %q", keys[0])
	}
	if string(values[0]) != "new_val" {
		t.Errorf("value after reuse: %q", values[0])
	}
}

// ---------- FlushTo: 写入失败场景 ----------

type errorWriter struct {
	err error
}

func (e *errorWriter) Write(p []byte) (int, error) {
	return 0, e.err
}

func TestFlushTo_WriterError(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("key"), []byte("value"))

	dest := &errorWriter{err: bytes.ErrTooLarge}
	n, err := b.FlushTo(dest)

	if err == nil {
		t.Error("expected error from FlushTo with failing writer")
	}
	if n != 0 {
		t.Errorf("expected n=0 on write error, got %d", n)
	}

	// 即使写入失败，block 也会被 clear（因为用了 defer）
	if b.Size() != 0 {
		t.Errorf("block should be cleared even after write error, size=%d", b.Size())
	}
}

// ---------- GetKvsCnt ----------

func TestGetKvsCnt_Increments(t *testing.T) {
	b := NewBlock(nil)

	for i := 1; i <= 5; i++ {
		b.Append([]byte("k"), []byte("v"))
		if b.GetKvsCnt() != i {
			t.Errorf("after %d appends, KvsCnt=%d", i, b.GetKvsCnt())
		}
	}
}

// ---------- clear (间接测试，通过 FlushTo) ----------

func TestClear_ResetsPrevKeyForPrefixCompression(t *testing.T) {
	b := NewBlock(nil)
	// 写入有共同前缀的 key
	b.Append([]byte("shared_prefix_aaa"), []byte("v1"))

	var dest bytes.Buffer
	_, _ = b.FlushTo(&dest) // clear

	// flush 后写入另一个带 "shared_prefix" 的 key
	b.Append([]byte("shared_prefix_bbb"), []byte("v2"))

	// 解码时 sharedPrefixLen 应为 0（因为 prevKey 已被清空）
	data := b.ToBytes()
	r := bytes.NewReader(data)
	sharedLen, _ := binary.ReadUvarint(r)
	if sharedLen != 0 {
		t.Errorf("after clear, first record's sharedPrefixLen should be 0, got %d", sharedLen)
	}
}

// ---------- 编码格式正确性（白盒测试）----------

func TestEncoding_FirstRecord_NoSharedPrefix(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("abc"), []byte("xyz"))

	data := b.ToBytes()
	r := bytes.NewReader(data)

	// 第一条记录的 sharedPrefixLen 应为 0
	shared, _ := binary.ReadUvarint(r)
	if shared != 0 {
		t.Errorf("first record sharedPrefixLen: got %d, want 0", shared)
	}

	// diffKeyLen = len("abc") = 3
	diffLen, _ := binary.ReadUvarint(r)
	if diffLen != 3 {
		t.Errorf("first record diffKeyLen: got %d, want 3", diffLen)
	}

	// valueLen = len("xyz") = 3
	valLen, _ := binary.ReadUvarint(r)
	if valLen != 3 {
		t.Errorf("first record valueLen: got %d, want 3", valLen)
	}

	// 读取 diff key
	diffKey := make([]byte, diffLen)
	r.Read(diffKey)
	if string(diffKey) != "abc" {
		t.Errorf("diff key: %q", diffKey)
	}

	// 读取 value
	val := make([]byte, valLen)
	r.Read(val)
	if string(val) != "xyz" {
		t.Errorf("value: %q", val)
	}

	// 应该没有剩余数据
	if r.Len() != 0 {
		t.Errorf("unexpected trailing bytes: %d", r.Len())
	}
}

func TestEncoding_SharedPrefix(t *testing.T) {
	b := NewBlock(nil)
	b.Append([]byte("abcdef"), []byte("v1"))
	b.Append([]byte("abcxyz"), []byte("v2"))

	data := b.ToBytes()

	// 跳过第一条记录
	r := bytes.NewReader(data)
	shared0, _ := binary.ReadUvarint(r)
	diffLen0, _ := binary.ReadUvarint(r)
	valLen0, _ := binary.ReadUvarint(r)
	r.Read(make([]byte, diffLen0+valLen0))

	_ = shared0 // 第一条 shared 应为 0

	// 读取第二条记录
	shared1, _ := binary.ReadUvarint(r)
	if shared1 != 3 { // "abc" 是公共前缀，长度为 3
		t.Errorf("second record sharedPrefixLen: got %d, want 3", shared1)
	}

	diffLen1, _ := binary.ReadUvarint(r)
	if diffLen1 != 3 { // "xyz" 是 diff 部分
		t.Errorf("second record diffKeyLen: got %d, want 3", diffLen1)
	}

	valLen1, _ := binary.ReadUvarint(r)
	if valLen1 != 2 { // "v2"
		t.Errorf("second record valueLen: got %d, want 2", valLen1)
	}

	diffKey1 := make([]byte, diffLen1)
	r.Read(diffKey1)
	if string(diffKey1) != "xyz" {
		t.Errorf("second record diff key: %q, want %q", diffKey1, "xyz")
	}
}

// ---------- 边界与压力测试 ----------

func TestAppend_LargeKeyValue(t *testing.T) {
	b := NewBlock(nil)
	// 创建较大的 key 和 value
	largeKey := bytes.Repeat([]byte("K"), 10000)
	largeVal := bytes.Repeat([]byte("V"), 50000)

	b.Append(largeKey, largeVal)

	keys, values, err := decodeAllRecords(b.ToBytes())
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !bytes.Equal(keys[0], largeKey) {
		t.Error("large key mismatch")
	}
	if !bytes.Equal(values[0], largeVal) {
		t.Error("large value mismatch")
	}
}

func TestAppend_PrevKeyNotMutated(t *testing.T) {
	// 确保 Append 对传入的 key slice 不会产生副作用
	b := NewBlock(nil)
	key1 := []byte("hello")
	original := make([]byte, len(key1))
	copy(original, key1)

	b.Append(key1, []byte("v"))
	b.Append([]byte("help"), []byte("v2"))

	if !bytes.Equal(key1, original) {
		t.Errorf("Append mutated the input key: got %q, original was %q", key1, original)
	}
}

// ---------- FlushTo: 空 block ----------

func TestFlushTo_EmptyBlock(t *testing.T) {
	b := NewBlock(nil)
	var dest bytes.Buffer
	n, err := b.FlushTo(&dest)

	if err != nil {
		t.Fatalf("FlushTo on empty block should not error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected n=0 for empty block, got %d", n)
	}
	if dest.Len() != 0 {
		t.Errorf("expected empty dest, got len=%d", dest.Len())
	}
}

// ---------- 多次 Flush 循环使用 ----------

func TestMultipleFlushCycles(t *testing.T) {
	b := NewBlock(nil)

	for cycle := 0; cycle < 5; cycle++ {
		for i := 0; i < 10; i++ {
			k := []byte("cycle" + padInt(cycle, 2) + "_key" + padInt(i, 3))
			v := []byte("val" + padInt(i, 3))
			b.Append(k, v)
		}

		if b.GetKvsCnt() != 10 {
			t.Errorf("cycle %d: expected KvsCnt=10, got %d", cycle, b.GetKvsCnt())
		}

		var dest bytes.Buffer
		_, err := b.FlushTo(&dest)
		if err != nil {
			t.Fatalf("cycle %d: FlushTo error: %v", cycle, err)
		}

		// 验证 flush 的数据可以正确解码
		keys, _, err := decodeAllRecords(dest.Bytes())
		if err != nil {
			t.Fatalf("cycle %d: decode error: %v", cycle, err)
		}
		if len(keys) != 10 {
			t.Errorf("cycle %d: decoded %d records, want 10", cycle, len(keys))
		}
	}
}

// ============================================================================
// 辅助工具
// ============================================================================

func padInt(n, width int) string {
	s := ""
	for i := 0; i < width; i++ {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
