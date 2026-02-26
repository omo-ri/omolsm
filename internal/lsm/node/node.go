package node

import (
	"bytes"
	"log"
	"omolsm/config"
	"omolsm/internal/lsm/sst_io/reader"
	writer "omolsm/internal/lsm/sst_io/writer"
	"os"
	"path"
)

type Node struct {
	conf          *config.Config    // 配置文件
	file          string            // sstable 对应的文件名，不含目录路径
	level         int               // sstable 所在 level 层级
	seq           int32             // sstable 的 seq 序列号. 对应为文件名中的 level_seq.sst 中的 seq
	size          uint64            // sstable 的大小，单位 byte
	blockToFilter map[uint64][]byte // 各 block 对应的 filter bitmap
	index         []*writer.Index   // 各 block 对应的索引
	startKey      []byte            // sstable 中最小的 key
	endKey        []byte            // sstable 中最大的 key
	sstReader     *reader.SSTReader // 读取 sst 文件的 reader 入口
}

func NewNode(conf *config.Config, file string, sstReader *reader.SSTReader, level int, seq int32,
	size uint64, blockToFilter map[uint64][]byte, index []*writer.Index) *Node {

	return &Node{
		conf:          conf,
		file:          file,
		sstReader:     sstReader,
		level:         level,
		seq:           seq,
		size:          size,
		blockToFilter: blockToFilter,
		index:         index,
		startKey:      index[0].Key,
		endKey:        index[len(index)-1].Key,
	}
}

func (n *Node) GetAll() ([]*reader.KV, error) {
	return n.sstReader.ReadData()
}

func (n *Node) GetFile() string {
	return n.file
}

func (n *Node) SSTReader() *reader.SSTReader  { return n.sstReader }
func (n *Node) IndexEntries() []*writer.Index { return n.index }

// 查看是否在节点中
func (n *Node) Get(key []byte) ([]byte, bool, error) {
	// 通过索引定位到具体的块
	index, ok := n.binarySearchIndex(key, 0, len(n.index)-1)
	if !ok {
		return nil, false, nil
	}

	// 布隆过滤器辅助判断 key 是否存在
	bitmap := n.blockToFilter[index.PrevBlockOffset]
	if ok = n.conf.Filter.Exist(bitmap, key); !ok {
		return nil, false, nil
	}

	// 读取对应的块
	block, err := n.sstReader.ReadBlock(index.PrevBlockOffset, index.PrevBlockSize)
	if err != nil {
		return nil, false, err
	}

	// 将块数据转为对应的 kv 对
	kvs, err := n.sstReader.ReadBlockData(block)
	if err != nil {
		return nil, false, err
	}

	for _, kv := range kvs {
		if bytes.Equal(kv.Key, key) {
			return kv.Value, true, nil
		}
	}

	return nil, false, nil
}

// GetRange 优化版: 利用 index 二分定位，只读必要的 block.
func (n *Node) GetRange(startKey, endKey []byte) ([]*reader.KV, error) {
	// 1. 快速排除: SST 范围与查询范围不相交
	if bytes.Compare(startKey, n.endKey) > 0 || bytes.Compare(endKey, n.startKey) <= 0 {
		return nil, nil
	}

	// 2. 二分找到 startKey 可能所在的第一个 block.
	//    index[i].Key 是 block i 的最大 key.
	//    找第一个 index[i].Key >= startKey 的 i.
	startIdx := n.findFirstBlock(startKey)

	// 3. 从 startIdx 开始逐 block 读取，直到超出 endKey 范围.
	var result []*reader.KV
	for i := startIdx; i < len(n.index); i++ {
		idx := n.index[i]

		// 读取该 block
		block, err := n.sstReader.ReadBlock(idx.PrevBlockOffset, idx.PrevBlockSize)
		if err != nil {
			return nil, err
		}

		kvs, err := n.sstReader.ReadBlockData(block)
		if err != nil {
			return nil, err
		}

		for _, kv := range kvs {
			if bytes.Compare(kv.Key, startKey) >= 0 && bytes.Compare(kv.Key, endKey) < 0 {
				result = append(result, kv)
			}
		}

		// index[i].Key 是当前 block 的最大 key.
		// 如果它已经 >= endKey，后续 block 的 key 只会更大，全部跳过.
		if bytes.Compare(idx.Key, endKey) >= 0 {
			break
		}
	}

	return result, nil
}

// findFirstBlock 二分查找: 找第一个 index[i].Key >= startKey 的 i.
// 因为 index[i].Key 是 block 的最大 key，如果 index[i].Key < startKey，
// 则该 block 内所有 key 都 < startKey，可以跳过.
func (n *Node) findFirstBlock(startKey []byte) int {
	lo, hi := 0, len(n.index)-1
	for lo < hi {
		mid := lo + (hi-lo)/2
		if bytes.Compare(n.index[mid].Key, startKey) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

// GetRangeUnoptimized 优化前的原始版本, 保留用于 benchmark 对比.
func (n *Node) GetRangeUnoptimized(startKey, endKey []byte) ([]*reader.KV, error) {
	allKVs, err := n.GetAll()
	if err != nil {
		return nil, err
	}

	var result []*reader.KV
	for _, kv := range allKVs {
		if bytes.Compare(kv.Key, startKey) >= 0 && bytes.Compare(kv.Key, endKey) < 0 {
			result = append(result, kv)
		}
	}
	return result, nil
}

func (n *Node) Size() uint64 {
	return n.size
}

func (n *Node) Start() []byte {
	return n.startKey
}

func (n *Node) End() []byte {
	return n.endKey
}

func (n *Node) Index() (level int, seq int32) {
	level, seq = n.level, n.seq
	return
}

func (n *Node) Destroy() {
	n.sstReader.Close()
	err := os.Remove(path.Join(n.conf.Dir, n.file))
	if err != nil {
		log.Printf("Error deleting node file %s: %v\n", path.Join(n.conf.Dir, n.file), err)
	}
}

func (n *Node) Close() {
	n.sstReader.Close()
}

// 二分查找，key 可能从属的 block index
func (n *Node) binarySearchIndex(key []byte, start, end int) (*writer.Index, bool) {
	if start == end {
		return n.index[start], bytes.Compare(n.index[start].Key, key) >= 0
	}

	// 目标块，保证 key <= index[i].key && key > index[i-1].key
	mid := start + (end-start)>>1
	if bytes.Compare(n.index[mid].Key, key) < 0 {
		return n.binarySearchIndex(key, mid+1, end)
	}

	return n.binarySearchIndex(key, start, mid)
}
