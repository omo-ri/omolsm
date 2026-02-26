package tree

import (
	"fmt"
	"log"
	iterator2 "omolsm/internal/lsm/interator"
	"omolsm/internal/lsm/memtable"
	"omolsm/internal/lsm/node"
	"omolsm/internal/lsm/sst_io/reader"
	writer "omolsm/internal/lsm/sst_io/writer"
)

// flushMemTable 将 memtable 同步写入 level 0，然后级联检查各层是否需要 compact.
func (t *Tree) flushMemTable(memTable memtable.MemTable) {
	kvs := memTable.All()
	seq := t.levelToSeq[0].Add(1)

	sstWriter, err := writer.NewSSTWriter(t.sstFile(0, seq), t.Conf)
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
	for level := 0; level < t.Conf.MaxLevel-1; level++ {
		if len(t.nodes[level]) < t.Conf.SSTNumPerLevel {
			break
		}
		//log.Printf("Level %d has %d nodes (>= %d), compact to level %d\n",
		//	level, len(t.nodes[level]), t.conf.SSTNumPerLevel, level+1)
		t.compactLevel(level)
	}
}

// compactLevel 使用多路归并迭代器，避免将所有数据一次性加载到内存.
func (t *Tree) compactLevel(level int) {
	if level >= t.Conf.MaxLevel-1 {
		return
	}

	nodes := t.nodes[level]
	if len(nodes) == 0 {
		return
	}

	iters := make([]iterator2.Iterator, 0, len(nodes))
	for _, n := range nodes {
		it := iterator2.NewNodeIterator(n, n.SSTReader(), n.IndexEntries())
		iters = append(iters, it)
	}

	mergeIter := iterator2.NewMergeIterator(iters)

	nextLevel := level + 1
	seq := t.levelToSeq[nextLevel].Add(1)
	sstWriter, err := writer.NewSSTWriter(t.sstFile(nextLevel, seq), t.Conf)
	if err != nil {
		log.Printf("Error creating SST writer: %v\n", err)
		return
	}

	for mergeIter.Next() {
		sstWriter.Append(mergeIter.Key(), mergeIter.Value())
	}

	if mergeIter.Err() != nil {
		log.Printf("Merge iteration error: %v\n", mergeIter.Err())
		sstWriter.Close()
		return
	}

	size, blockToFilter, index := sstWriter.Finish()
	t.stats.BytesWritten += size
	sstWriter.Close()
	t.insertNode(nextLevel, seq, size, blockToFilter, index)

	// 4. 删除旧节点
	t.removeNodes(level, nodes)
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
	sstReader, err := reader.NewSSTReader(file, t.Conf)
	if err != nil {
		log.Printf("Error creating SST reader for %s: %v\n", file, err)
		return
	}
	newNode := node.NewNode(t.Conf, file, sstReader, level, seq, size, blockToFilter, index)
	t.nodes[level] = append(t.nodes[level], newNode)
}

func (t *Tree) sstFile(level int, seq int32) string {
	return fmt.Sprintf("%d_%d.sst", level, seq)
}
