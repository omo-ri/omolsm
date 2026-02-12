package block

import (
	"bytes"
	"encoding/binary"
	"io"
	"omolsm"
	"omolsm/util"
)

type Block struct {
	conf    *omolsm.Config
	buffer  [30]byte
	record  *bytes.Buffer
	kvsCnt  int
	prevKey []byte
}

func NewBlock(conf *omolsm.Config) *Block {
	return &Block{
		conf:   conf,
		record: new(bytes.Buffer),
	}
}

func (b *Block) Append(key, value []byte) {
	defer func() {
		b.prevKey = append(b.prevKey[:0], key...)
		b.kvsCnt++
	}()

	sharedPrefixLen := util.SharedPrefixLen(b.prevKey, key)

	n := binary.PutUvarint(b.buffer[0:], uint64(sharedPrefixLen))
	n += binary.PutUvarint(b.buffer[n:], uint64(len(key)-sharedPrefixLen))
	n += binary.PutUvarint(b.buffer[n:], uint64(len(value)))

	_, _ = b.record.Write(b.buffer[:n])
	b.record.Write(key[sharedPrefixLen:])
	b.record.Write(value)
}

func (b *Block) Size() int {
	return b.record.Len()
}

func (b *Block) FlushTo(dest io.Writer) (uint64, error) {
	defer b.clear()
	n, err := dest.Write(b.ToBytes())
	return uint64(n), err
}

func (b *Block) ToBytes() []byte {
	return b.record.Bytes()
}

func (b *Block) GetKvsCnt() int {
	return b.kvsCnt
}

func (b *Block) clear() {
	b.kvsCnt = 0
	b.prevKey = b.prevKey[:0]
	b.record.Reset()
}
