package memtable

import (
	"sort"
)

// MapMemTable 基于 map 实现的 memtable
type MapMemTable struct {
	data map[string][]byte
	size int
}

// NewMapMemTable 构造函数
func NewMapMemTable() MemTable {
	return &MapMemTable{
		data: make(map[string][]byte),
		size: 0,
	}
}

// DefaultMemTableConstructor 默认构造器
var DefaultMemTableConstructor MemTableConstructor = func() MemTable {
	return NewMapMemTable()
}

func (m *MapMemTable) Put(key string, value []byte) {
	if old, ok := m.data[key]; ok {
		m.size -= len(key) + len(old)
	}
	m.data[key] = value
	m.size += len(key) + len(value)
}

func (m *MapMemTable) Get(key string) ([]byte, bool) {
	v, ok := m.data[key]
	return v, ok
}

func (m *MapMemTable) All() []*KV {
	kvs := make([]*KV, 0, len(m.data))
	for k, v := range m.data {
		kvs = append(kvs, &KV{Key: k, Value: v})
	}
	sort.Slice(kvs, func(i, j int) bool {
		return kvs[i].Key < kvs[j].Key
	})
	return kvs
}

func (m *MapMemTable) Size() int {
	return m.size
}

func (m *MapMemTable) KvsCnt() int {
	return len(m.data)
}
