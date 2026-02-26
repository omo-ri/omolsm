package tree

import (
	"fmt"
	"log"
	"omolsm/config"
	iterator2 "omolsm/internal/lsm/iterator"
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
	t.stats.FlushCount++
	t.insertNode(0, seq, size, blockToFilter, index)
	sstWriter.Close()

	// Hook: OnFlush
	if t.Conf.Hooks.OnFlush != nil {
		t.Conf.Hooks.OnFlush(config.FlushInfo{
			Level:   0,
			SSTSize: size,
			KVCount: len(kvs),
			MemSize: memTable.Size(),
		})
	}

	t.cascadeCompact()
}

func (t *Tree) cascadeCompact() {
	for level := 0; level < t.Conf.MaxLevel-1; level++ {
		if len(t.nodes[level]) < t.Conf.SSTNumPerLevel {
			break
		}
		t.compactLevel(level)
	}
}

// compactLevel 使用注入的 MergeIteratorFactory 进行归并.
// - KV 模式 (默认): last-write-wins
// - Bitmap 模式: OR 合并
func (t *Tree) compactLevel(level int) {
	if level >= t.Conf.MaxLevel-1 {
		return
	}

	nodes := t.nodes[level]
	if len(nodes) == 0 {
		return
	}

	// 构建子迭代器
	subs := make([]config.SubIterator, 0, len(nodes))
	for _, n := range nodes {
		it := iterator2.NewNodeIterator(n, n.SSTReader(), n.IndexEntries())
		subs = append(subs, it) // NodeIterator 满足 config.SubIterator
	}

	// 用注入的工厂创建归并迭代器, nil 则使用默认 KV 模式
	var mergeIter config.MergeIterator
	if t.Conf.MergeIteratorFactory != nil {
		mergeIter = t.Conf.MergeIteratorFactory(subs)
	} else {
		// 默认: KV 覆盖模式
		iters := make([]iterator2.Iterator, len(subs))
		for i, s := range subs {
			iters[i] = s.(iterator2.Iterator)
		}
		mergeIter = iterator2.NewMergeIterator(iters)
	}

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
	t.stats.CompactCount++
	sstWriter.Close()
	t.insertNode(nextLevel, seq, size, blockToFilter, index)

	// Hook: OnCompaction
	if t.Conf.Hooks.OnCompaction != nil {
		t.Conf.Hooks.OnCompaction(config.CompactionInfo{
			FromLevel:  level,
			ToLevel:    nextLevel,
			InputFiles: len(nodes),
			OutputSize: size,
		})
	}

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
