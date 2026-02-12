package tree

import (
	"omolsm"
	"omolsm/memtable"
	"omolsm/node"
	"sort"
	"sync"
	"sync/atomic"
)

type Tree struct {
	conf     *omolsm.Config
	dataLock sync.RWMutex

	memTable memtable.MemTable
	nodes    [][]*node.Node

	levelToSeq []atomic.Int32
}

func NewTree(conf *omolsm.Config) (LSMTree, error) {
	t := &Tree{
		conf:       conf,
		memTable:   conf.MemTableConstructor(),
		levelToSeq: make([]atomic.Int32, conf.MaxLevel),
		nodes:      make([][]*node.Node, conf.MaxLevel),
	}
	return t, nil
}

func (t *Tree) Put(key string, value []byte) error {
	t.dataLock.Lock()
	defer t.dataLock.Unlock()

	t.memTable.Put(key, value)

	if t.memTable.Size() < t.conf.SSTSize {
		return nil
	}

	// memtable 满了，同步 flush + 级联 compact
	t.flushMemTable(t.memTable)
	t.memTable = t.conf.MemTableConstructor()

	return nil
}

func (t *Tree) Get(key string) ([]byte, bool, error) {
	t.dataLock.RLock()
	defer t.dataLock.RUnlock()

	if v, ok := t.memTable.Get(key); ok {
		if v == nil {
			return nil, false, nil // 墓碑，已删除
		}
		return v, true, nil
	}

	for level := 0; level < len(t.nodes); level++ {
		for i := len(t.nodes[level]) - 1; i >= 0; i-- {
			v, ok, err := t.nodes[level][i].Get([]byte(key))
			if err != nil {
				return nil, false, err
			}
			if ok {
				if v == nil {
					return nil, false, nil // 墓碑
				}
				return v, true, nil
			}
		}
	}

	return nil, false, nil
}

func (t *Tree) Delete(key string) error {
	// 写入墓碑标记，nil 表示删除
	return t.Put(key, nil)
}

func (t *Tree) Scan(startKey, endKey string) ([]*KVResult, error) {
	t.dataLock.RLock()
	defer t.dataLock.RUnlock()

	// 用 map 收集，新值覆盖旧值
	merged := make(map[string][]byte)

	// 1. 先从 SSTable 收集（旧数据先放，后面被新数据覆盖）
	// 从高层到低层，旧的先写入 map
	for level := len(t.nodes) - 1; level >= 0; level-- {
		// 同层从旧到新
		for _, n := range t.nodes[level] {
			kvs, err := n.GetRange([]byte(startKey), []byte(endKey))
			if err != nil {
				return nil, err
			}
			for _, kv := range kvs {
				merged[string(kv.Key)] = kv.Value
			}
		}
	}

	// 2. 再从 memtable 收集（最新数据，覆盖旧值）
	allKVs := t.memTable.All()
	for _, kv := range allKVs {
		if kv.Key >= startKey && kv.Key < endKey {
			merged[kv.Key] = kv.Value
		}
	}

	// 3. 排序 + 过滤墓碑
	result := make([]*KVResult, 0, len(merged))
	for k, v := range merged {
		if v == nil {
			continue // 跳过已删除的 key
		}
		result = append(result, &KVResult{Key: k, Value: v})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key < result[j].Key
	})

	return result, nil
}

func (t *Tree) Close() {
	for level := 0; level < len(t.nodes); level++ {
		for _, n := range t.nodes[level] {
			n.Close()
		}
	}
}
