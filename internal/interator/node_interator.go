// ========================
// 文件 2: internal/iterator/node_iterator.go
// 单个 Node (SST) 的迭代器，按 block 懒加载
// ========================

package iterator

import (
	"omolsm/internal/node"
	"omolsm/internal/sst_io/reader"
	writer "omolsm/internal/sst_io/writer"
)

// NodeIterator 逐 block 遍历一个 Node，内存中最多持有一个 block 的数据.
type NodeIterator struct {
	node      *node.Node
	sstReader *reader.SSTReader
	index     []*writer.Index // block 索引列表

	blockIdx int          // 当前 block 在 index 中的位置
	kvs      []*reader.KV // 当前 block 解析出的 KV
	kvIdx    int          // 当前 KV 在 kvs 中的位置

	started bool
	err     error
}

func NewNodeIterator(n *node.Node, sstReader *reader.SSTReader, index []*writer.Index) *NodeIterator {
	return &NodeIterator{
		node:      n,
		sstReader: sstReader,
		index:     index,
		blockIdx:  0,
		kvIdx:     -1,
	}
}

func (it *NodeIterator) Next() bool {
	if it.err != nil {
		return false
	}

	// 尝试在当前 block 内前进
	it.kvIdx++
	if it.kvs != nil && it.kvIdx < len(it.kvs) {
		return true
	}

	// 当前 block 耗尽，加载下一个 block
	if it.started {
		it.blockIdx++
	}
	it.started = true

	for it.blockIdx < len(it.index) {
		idx := it.index[it.blockIdx]
		block, err := it.sstReader.ReadBlock(idx.PrevBlockOffset, idx.PrevBlockSize)
		if err != nil {
			it.err = err
			return false
		}
		kvs, err := it.sstReader.ReadBlockData(block)
		if err != nil {
			it.err = err
			return false
		}
		if len(kvs) > 0 {
			it.kvs = kvs
			it.kvIdx = 0
			return true
		}
		// 空 block，跳过
		it.blockIdx++
	}

	return false // 所有 block 遍历完毕
}

func (it *NodeIterator) Key() []byte   { return it.kvs[it.kvIdx].Key }
func (it *NodeIterator) Value() []byte { return it.kvs[it.kvIdx].Value }
func (it *NodeIterator) Err() error    { return it.err }
