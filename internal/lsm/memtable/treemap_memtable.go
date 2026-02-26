package memtable

import "github.com/emirpasic/gods/maps/treemap"

// TreeMapMemTable 基于红黑树实现的 memtable
type TreeMapMemTable struct {
	data *treemap.Map
	size int
}

func NewTreeMapMemTable() MemTable {
	return &TreeMapMemTable{
		data: treemap.NewWithStringComparator(),
	}
}

var TreeMapMemTableConstructor MemTableConstructor = func() MemTable {
	return NewTreeMapMemTable()
}

func (m *TreeMapMemTable) Put(key string, value []byte) {
	if old, ok := m.data.Get(key); ok {
		m.size -= len(key) + len(old.([]byte))
	}
	m.data.Put(key, value)
	m.size += len(key) + len(value)
}

func (m *TreeMapMemTable) Get(key string) ([]byte, bool) {
	v, ok := m.data.Get(key)
	if !ok {
		return nil, false
	}
	return v.([]byte), true
}

func (m *TreeMapMemTable) All() []*KV {
	kvs := make([]*KV, 0, m.data.Size())
	it := m.data.Iterator()
	for it.Next() {
		kvs = append(kvs, &KV{
			Key:   it.Key().(string),
			Value: it.Value().([]byte),
		})
	}
	return kvs
}

func (m *TreeMapMemTable) Size() int {
	return m.size
}

func (m *TreeMapMemTable) KvsCnt() int {
	return m.data.Size()
}
