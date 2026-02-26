package tree

import (
	"omolsm/config"
	"omolsm/internal/lsm/memtable"
	"omolsm/internal/lsm/node"
	"sort"
	"sync/atomic"
)

type Tree struct {
	Conf *config.Config

	memTable memtable.MemTable
	nodes    [][]*node.Node

	levelToSeq []atomic.Int32
	stats      Stats
}

// Stats LSM 运行时统计信息.
type Stats struct {
	BytesWritten uint64 // 实际写入磁盘的字节数
	BytesRead    uint64 // 实际从磁盘读取的字节数
	FlushCount   int    // memtable → SST flush 次数
	CompactCount int    // compaction 次数
}

// GetStats 返回运行时统计信息.
func (t *Tree) GetStats() Stats {
	return t.stats
}

// SSTPerLevel 返回每层的 SST 数量 (用于外部观测).
func (t *Tree) SSTPerLevel() []int {
	result := make([]int, len(t.nodes))
	for i, level := range t.nodes {
		result[i] = len(level)
	}
	return result
}

// MemTableSize 返回当前 memtable 的字节数.
func (t *Tree) MemTableSize() int {
	return t.memTable.Size()
}

// MemTableCount 返回当前 memtable 的 KV 数量.
func (t *Tree) MemTableCount() int {
	return t.memTable.KvsCnt()
}

func NewTree(conf *config.Config) (LSMTree, error) {
	t := &Tree{
		Conf:       conf,
		memTable:   conf.MemTableConstructor(),
		levelToSeq: make([]atomic.Int32, conf.MaxLevel),
		nodes:      make([][]*node.Node, conf.MaxLevel),
	}
	return t, nil
}

func (t *Tree) Put(key string, value []byte) error {
	t.memTable.Put(key, value)

	// Hook: OnWrite
	if t.Conf.Hooks.OnWrite != nil {
		t.Conf.Hooks.OnWrite([]byte(key), len(value))
	}

	if t.memTable.Size() < t.Conf.SSTSize {
		return nil
	}

	t.flushMemTable(t.memTable)
	t.memTable = t.Conf.MemTableConstructor()

	return nil
}

func (t *Tree) Get(key string) ([]byte, bool, error) {
	if v, ok := t.memTable.Get(key); ok {
		if v == nil {
			return nil, false, nil // 墓碑
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
	return t.Put(key, nil)
}

func (t *Tree) Scan(startKey, endKey string) ([]*KVResult, error) {
	merged := make(map[string][]byte)

	for level := len(t.nodes) - 1; level >= 0; level-- {
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

	allKVs := t.memTable.All()
	for _, kv := range allKVs {
		if kv.Key >= startKey && kv.Key < endKey {
			merged[kv.Key] = kv.Value
		}
	}

	result := make([]*KVResult, 0, len(merged))
	for k, v := range merged {
		if v == nil {
			continue
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
