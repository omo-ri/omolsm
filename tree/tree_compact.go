package tree

import (
	"fmt"
	"log"
	"omolsm/memtable"
	"omolsm/node"
	"omolsm/sst_io/reader"
	writer "omolsm/sst_io/writer"
	"sort"
)

// flushMemTable 将 memtable 同步写入 level 0，然后级联检查各层是否需要 compact.
func (t *Tree) flushMemTable(memTable memtable.MemTable) {
	kvs := memTable.All()
	seq := t.levelToSeq[0].Add(1)

	sstWriter, err := writer.NewSSTWriter(t.sstFile(0, seq), t.conf)
	if err != nil {
		log.Printf("Error creating SST writer for level 0 seq %d: %v\n", seq, err)
		return
	}

	for _, kv := range kvs {
		sstWriter.Append([]byte(kv.Key), kv.Value)
	}

	size, blockToFilter, index := sstWriter.Finish()
	t.stats.BytesWritten += size
	t.insertNode(0, seq, size, blockToFilter, index)
	sstWriter.Close()

	// 级联 compact：从 level 0 开始，逐层检查
	t.cascadeCompact()
}

// cascadeCompact 从 level 0 开始，如果某层节点数 >= 阈值就合并到下一层，
// 然后继续检查下一层，直到不需要 compact 或到达最后一层.
func (t *Tree) cascadeCompact() {
	for level := 0; level < t.conf.MaxLevel-1; level++ {
		if len(t.nodes[level]) < t.conf.SSTNumPerLevel {
			break
		}
		//log.Printf("Level %d has %d nodes (>= %d), compact to level %d\n",
		//	level, len(t.nodes[level]), t.conf.SSTNumPerLevel, level+1)
		t.compactLevel(level)
	}
}

func (t *Tree) compactLevel(level int) {
	if level >= t.conf.MaxLevel-1 {
		return
	}

	nodes := t.nodes[level]
	if len(nodes) == 0 {
		return
	}

	// 1. 多路归并：新值覆盖旧值
	merged := make(map[string][]byte)
	for _, n := range nodes {
		kvs, err := n.GetAll()
		if err != nil {
			log.Printf("Error reading node %s: %v\n", n.GetFile(), err)
			return
		}
		for _, kv := range kvs {
			merged[string(kv.Key)] = kv.Value
		}
	}

	// 2. 排序
	sortedKVs := make([]*memtable.KV, 0, len(merged))
	for k, v := range merged {
		sortedKVs = append(sortedKVs, &memtable.KV{Key: k, Value: v})
	}
	sort.Slice(sortedKVs, func(i, j int) bool {
		return sortedKVs[i].Key < sortedKVs[j].Key
	})

	// 3. 写入下一层
	nextLevel := level + 1
	seq := t.levelToSeq[nextLevel].Add(1)
	sstWriter, err := writer.NewSSTWriter(t.sstFile(nextLevel, seq), t.conf)
	if err != nil {
		log.Printf("Error creating SST writer for level %d seq %d: %v\n", nextLevel, seq, err)
		return
	}

	for _, kv := range sortedKVs {
		sstWriter.Append([]byte(kv.Key), kv.Value)
	}
	size, blockToFilter, index := sstWriter.Finish()
	t.stats.BytesWritten += size
	sstWriter.Close()
	t.insertNode(nextLevel, seq, size, blockToFilter, index)

	// 4. 删除旧节点
	t.removeNodes(level, nodes)

	//log.Printf("Compact level %d -> %d done, level %d nodes: %d, level %d nodes: %d\n",
	//	level, nextLevel, level, len(t.nodes[level]), nextLevel, len(t.nodes[nextLevel]))
}

func (t *Tree) removeNodes(level int, toRemove []*node.Node) {
	removeSet := make(map[*node.Node]struct{}, len(toRemove))
	for _, n := range toRemove {
		removeSet[n] = struct{}{}
	}

	kept := make([]*node.Node, 0, len(t.nodes[level]))
	for _, n := range t.nodes[level] {
		if _, ok := removeSet[n]; ok {
			n.Destroy()
		} else {
			kept = append(kept, n)
		}
	}
	t.nodes[level] = kept
}

func (t *Tree) insertNode(level int, seq int32, size uint64, blockToFilter map[uint64][]byte, index []*writer.Index) {
	file := t.sstFile(level, seq)
	sstReader, err := reader.NewSSTReader(file, t.conf)
	if err != nil {
		log.Printf("Error creating SST reader for %s: %v\n", file, err)
		return
	}
	newNode := node.NewNode(t.conf, file, sstReader, level, seq, size, blockToFilter, index)
	t.nodes[level] = append(t.nodes[level], newNode)
}

func (t *Tree) sstFile(level int, seq int32) string {
	return fmt.Sprintf("%d_%d.sst", level, seq)
}
