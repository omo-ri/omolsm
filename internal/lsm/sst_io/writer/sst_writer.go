package sst_io

import (
	"bytes"
	"encoding/binary"
	"omolsm/config"
	"omolsm/internal/lsm/block"
	"omolsm/internal/lsm/filter"
	"os"
	"path"
)

type Index struct {
	Key             []byte // 索引的 key. 是每一个 block 中最大的 key
	PrevBlockOffset uint64 // 索引前一个 block 起始位置在 sstable 中对应的 offset
	PrevBlockSize   uint64 // 索引前一个 block 的大小，单位 byte
}

type SSTWriter struct {
	conf          *config.Config
	filter        filter.Filter // 每个 writer 独立的 filter 实例
	dest          *os.File
	dataBuf       *bytes.Buffer
	filterBuf     *bytes.Buffer
	indexBuf      *bytes.Buffer
	blockToFilter map[uint64][]byte // prev block offset -> filter bit map
	index         []*Index          // index key -> prev block offset, prev block size

	dataBlock     *block.Block
	filterBlock   *block.Block
	indexBlock    *block.Block
	assistScratch [20]byte

	prevKey         []byte
	prevBlockOffset uint64
	prevBlockSize   uint64
}

func NewSSTWriter(file string, conf *config.Config) (*SSTWriter, error) {
	dest, err := os.OpenFile(path.Join(conf.Dir, file), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}

	return &SSTWriter{
		conf:          conf,
		filter:        conf.FilterConstructor(),
		dest:          dest,
		dataBuf:       new(bytes.Buffer),
		filterBuf:     new(bytes.Buffer),
		indexBuf:      new(bytes.Buffer),
		blockToFilter: make(map[uint64][]byte),
		dataBlock:     block.NewBlock(conf),
		filterBlock:   block.NewBlock(conf),
		indexBlock:    block.NewBlock(conf),
		prevKey:       []byte{},
	}, nil
}

func (s *SSTWriter) Finish() (size uint64, blockToFilter map[uint64][]byte, index []*Index) {
	s.refreshBlock()
	// 补齐最后一个 index
	s.insertIndex()

	// 将布隆过滤器块写入缓冲区
	_, _ = s.filterBlock.FlushTo(s.filterBuf)
	// 将索引块写入缓冲区
	_, _ = s.indexBlock.FlushTo(s.indexBuf)

	// 处理 footer，记录布隆过滤器块起始、大小、索引块起始、大小
	footer := make([]byte, s.conf.SSTFooterSize)
	size = uint64(s.dataBuf.Len())
	n := binary.PutUvarint(footer[0:], size)
	filterBufLen := uint64(s.filterBuf.Len())
	n += binary.PutUvarint(footer[n:], filterBufLen)
	size += filterBufLen
	n += binary.PutUvarint(footer[n:], size)
	indexBufLen := uint64(s.indexBuf.Len())
	n += binary.PutUvarint(footer[n:], indexBufLen)
	size += indexBufLen

	// 依次写入文件
	_, _ = s.dest.Write(s.dataBuf.Bytes())
	_, _ = s.dest.Write(s.filterBuf.Bytes())
	_, _ = s.dest.Write(s.indexBuf.Bytes())
	_, _ = s.dest.Write(footer)

	blockToFilter = s.blockToFilter
	index = s.index
	return
}

// Append 追加一笔数据到 sstable 中
func (s *SSTWriter) Append(key, value []byte) {
	if s.dataBlock.GetKvsCnt() == 0 {
		s.insertIndex()
	}

	// 将数据写入到数据块中
	s.dataBlock.Append(key, value)
	// 将 key 添加到块的布隆过滤器中
	s.filter.Add(key)
	// 记录一下最新的 key（深拷贝，防止调用方复用 buffer 导致污染）
	s.prevKey = append(s.prevKey[:0], key...)

	// 倘若数据块大小超限，则需要将其添加到 dataBuffer，并重置块
	if s.dataBlock.Size() >= s.conf.SSTDataBlockSize {
		s.refreshBlock()
	}
}

func (s *SSTWriter) Size() uint64 {
	return uint64(s.dataBuf.Len())
}

func (s *SSTWriter) Close() {
	_ = s.dest.Close()
	s.dataBuf.Reset()
	s.indexBuf.Reset()
	s.filterBuf.Reset()
}

func (s *SSTWriter) insertIndex() {
	if len(s.prevKey) == 0 && s.prevBlockSize == 0 {
		return
	}
	// 深拷贝 prevKey，确保每条 index 持有独立内存
	indexKey := append([]byte{}, s.prevKey...)
	n := binary.PutUvarint(s.assistScratch[0:], s.prevBlockOffset)
	n += binary.PutUvarint(s.assistScratch[n:], s.prevBlockSize)

	s.indexBlock.Append(indexKey, s.assistScratch[:n])
	s.index = append(s.index, &Index{
		Key:             indexKey,
		PrevBlockOffset: s.prevBlockOffset,
		PrevBlockSize:   s.prevBlockSize,
	})
}

func (s *SSTWriter) refreshBlock() {
	if s.filter.KeyLen() == 0 {
		return
	}

	s.prevBlockOffset = uint64(s.dataBuf.Len())
	// 添加布隆过滤器 bitmap
	filterBitmap := s.filter.Hash()
	s.blockToFilter[s.prevBlockOffset] = filterBitmap
	n := binary.PutUvarint(s.assistScratch[0:], s.prevBlockOffset)
	s.filterBlock.Append(s.assistScratch[:n], filterBitmap)
	// 重置布隆过滤器
	s.filter.Reset()

	// 将 block 的数据添加到缓冲区
	s.prevBlockSize, _ = s.dataBlock.FlushTo(s.dataBuf)
}
