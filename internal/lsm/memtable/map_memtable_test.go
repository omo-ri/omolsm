package memtable

import "testing"
import "github.com/stretchr/testify/assert"

func Test_MapMemTable(t *testing.T) {
	mapMemTable := DefaultMemTableConstructor()

	mapMemTable.Put("a", []byte("b"))
	mapMemTable.Put("a", []byte("c"))
	mapMemTable.Put("ab", []byte("aa"))
	mapMemTable.Put("abc", []byte("aaa"))
	mapMemTable.Put("ab", []byte("bb"))
	mapMemTable.Put("bc", []byte("bbb"))

	val, _ := mapMemTable.Get("a")
	assert.Equal(t, "c", string(val))
	val, _ = mapMemTable.Get("ab")
	assert.Equal(t, "bb", string(val))
	val, _ = mapMemTable.Get("abc")
	assert.Equal(t, "aaa", string(val))
	val, _ = mapMemTable.Get("bc")
	assert.Equal(t, "bbb", string(val))
	_, ok := mapMemTable.Get("b")
	assert.False(t, ok)
	assert.Equal(t, mapMemTable.Size(), 17)
	t.Log(mapMemTable.Size())

	kvs := mapMemTable.All()
	assert.Equal(t, 4, len(kvs))

	assert.Equal(t, "a", kvs[0].Key)
	assert.Equal(t, "c", string(kvs[0].Value))

	assert.Equal(t, "ab", kvs[1].Key)
	assert.Equal(t, "bb", string(kvs[1].Value))

	assert.Equal(t, "abc", kvs[2].Key)
	assert.Equal(t, "aaa", string(kvs[2].Value))

	assert.Equal(t, "bc", kvs[3].Key)
	assert.Equal(t, "bbb", string(kvs[3].Value))
}
