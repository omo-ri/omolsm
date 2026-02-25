package tree

import (
	"fmt"
	"omolsm/config"
	"testing"
)

func setupTestTree(t *testing.T) LSMTree {
	dir := t.TempDir()
	conf, err := config.NewConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := NewTree(conf)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// 基本写入和读取
func Test_Put_Get_Basic(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	// 写入单条
	if err := tr.Put("name", []byte("alice")); err != nil {
		t.Fatal(err)
	}

	// 读取存在的 key
	v, ok, err := tr.Get("name")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected key to exist")
	}
	if string(v) != "alice" {
		t.Fatalf("expected alice, got %s", string(v))
	}

	// 读取不存在的 key
	_, ok, err = tr.Get("missing")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected key to not exist")
	}
}

// 覆盖写入
func Test_Put_Overwrite(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	err := tr.Put("key", []byte("v1"))
	if err != nil {
		return
	}
	err = tr.Put("key", []byte("v2"))
	if err != nil {
		return
	}
	err = tr.Put("key", []byte("v3"))
	if err != nil {
		return
	}

	v, ok, _ := tr.Get("key")
	if !ok || string(v) != "v3" {
		t.Fatalf("expected v3, got %s", string(v))
	}
}

// 空值写入
func Test_Put_EmptyValue(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	tr.Put("empty", []byte{})

	v, ok, _ := tr.Get("empty")
	if !ok {
		t.Fatal("expected key to exist")
	}
	if len(v) != 0 {
		t.Fatalf("expected empty value, got %v", v)
	}
}

// Delete 基本功能
func Test_Delete(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	tr.Put("key", []byte("value"))

	// 确认存在
	_, ok, _ := tr.Get("key")
	if !ok {
		t.Fatal("expected key to exist before delete")
	}

	// 删除
	if err := tr.Delete("key"); err != nil {
		t.Fatal(err)
	}

	// 确认不存在
	_, ok, _ = tr.Get("key")
	if ok {
		t.Fatal("expected key to not exist after delete")
	}
}

// 删除不存在的 key
func Test_Delete_NonExistent(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	// 不应该报错
	if err := tr.Delete("ghost"); err != nil {
		t.Fatal(err)
	}

	_, ok, _ := tr.Get("ghost")
	if ok {
		t.Fatal("expected key to not exist")
	}
}

// 删除后重新写入
func Test_Delete_Then_Put(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	tr.Put("key", []byte("v1"))
	tr.Delete("key")
	tr.Put("key", []byte("v2"))

	v, ok, _ := tr.Get("key")
	if !ok || string(v) != "v2" {
		t.Fatalf("expected v2, got %s", string(v))
	}
}

// Scan 基本功能
func Test_Scan_Basic(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	tr.Put("a", []byte("1"))
	tr.Put("b", []byte("2"))
	tr.Put("c", []byte("3"))
	tr.Put("d", []byte("4"))
	tr.Put("e", []byte("5"))

	// 查询 b ~ d（不包含 d）
	results, err := tr.Scan("b", "d")
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Key != "b" || string(results[0].Value) != "2" {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	if results[1].Key != "c" || string(results[1].Value) != "3" {
		t.Fatalf("unexpected second result: %+v", results[1])
	}
}

// Scan 结果有序
func Test_Scan_Ordered(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	// 乱序写入
	tr.Put("dog", []byte("1"))
	tr.Put("apple", []byte("2"))
	tr.Put("cat", []byte("3"))
	tr.Put("banana", []byte("4"))
	tr.Put("egg", []byte("5"))

	results, _ := tr.Scan("a", "f")

	for i := 1; i < len(results); i++ {
		if results[i].Key <= results[i-1].Key {
			t.Fatalf("results not sorted: %s >= %s", results[i-1].Key, results[i].Key)
		}
	}
}

// Scan 空范围
func Test_Scan_Empty(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	tr.Put("a", []byte("1"))
	tr.Put("b", []byte("2"))

	// 查询不存在的范围
	results, err := tr.Scan("x", "z")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

// Scan 排除已删除的 key
func Test_Scan_With_Deleted(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	tr.Put("a", []byte("1"))
	tr.Put("b", []byte("2"))
	tr.Put("c", []byte("3"))
	tr.Delete("b")

	results, _ := tr.Scan("a", "d")

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Key != "a" {
		t.Fatalf("expected a, got %s", results[0].Key)
	}
	if results[1].Key != "c" {
		t.Fatalf("expected c, got %s", results[1].Key)
	}
}

// Scan 全范围
func Test_Scan_All(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	count := 100
	for i := 0; i < count; i++ {
		tr.Put(fmt.Sprintf("key_%04d", i), []byte(fmt.Sprintf("val_%04d", i)))
	}

	results, _ := tr.Scan("key_0000", "key_9999")
	if len(results) != count {
		t.Fatalf("expected %d results, got %d", count, len(results))
	}
}

// 大量写入触发 flush
func Test_Put_Triggers_Flush(t *testing.T) {
	dir := t.TempDir()
	// 小阈值，容易触发 flush
	conf, err := config.NewConfig(dir, config.WithSSTSize(256))
	if err != nil {
		t.Fatal(err)
	}
	tr, err := NewTree(conf)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()

	// 写入足够多数据触发 flush
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key_%04d", i)
		value := []byte(fmt.Sprintf("value_%04d", i))
		if err := tr.Put(key, value); err != nil {
			t.Fatal(err)
		}
	}

	// flush 后数据仍然可读
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key_%04d", i)
		v, ok, err := tr.Get(key)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("key %s not found after flush", key)
		}
		expected := fmt.Sprintf("value_%04d", i)
		if string(v) != expected {
			t.Fatalf("expected %s, got %s", expected, string(v))
		}
	}
}

// flush 后删除仍然生效
func Test_Delete_After_Flush(t *testing.T) {
	dir := t.TempDir()
	conf, _ := config.NewConfig(dir, config.WithSSTSize(256))
	tr, _ := NewTree(conf)
	defer tr.Close()

	// 写入触发 flush
	for i := 0; i < 100; i++ {
		tr.Put(fmt.Sprintf("key_%04d", i), []byte("value"))
	}

	// 删除一些 key
	tr.Delete("key_0010")
	tr.Delete("key_0050")

	_, ok, _ := tr.Get("key_0010")
	if ok {
		t.Fatal("expected key_0010 to be deleted")
	}

	_, ok, _ = tr.Get("key_0050")
	if ok {
		t.Fatal("expected key_0050 to be deleted")
	}

	// 未删除的仍然存在
	v, ok, _ := tr.Get("key_0001")
	if !ok || string(v) != "value" {
		t.Fatal("expected key_0001 to exist")
	}
}

// Scan 跨 memtable 和 SSTable
func Test_Scan_Across_Flush(t *testing.T) {
	dir := t.TempDir()
	conf, _ := config.NewConfig(dir, config.WithSSTSize(256))
	tr, _ := NewTree(conf)
	defer tr.Close()

	// 写入足够触发 flush，部分在 SSTable，部分在 memtable
	for i := 0; i < 200; i++ {
		tr.Put(fmt.Sprintf("key_%04d", i), []byte(fmt.Sprintf("val_%04d", i)))
	}

	results, err := tr.Scan("key_0000", "key_9999")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 200 {
		t.Fatalf("expected 200 results, got %d", len(results))
	}

	// 验证有序
	for i := 1; i < len(results); i++ {
		if results[i].Key <= results[i-1].Key {
			t.Fatalf("results not sorted at index %d", i)
		}
	}
}

// 空树操作
func Test_Empty_Tree(t *testing.T) {
	tr := setupTestTree(t)
	defer tr.Close()

	_, ok, err := tr.Get("any")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no result from empty tree")
	}

	results, err := tr.Scan("a", "z")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatal("expected empty scan from empty tree")
	}
}
