package filter

import (
	"github.com/spaolacci/murmur3"
)

// BloomFilter 布隆过滤器
type BloomFilter struct {
	m          int      // bitmap 的长度，单位 bit
	hashedKeys []uint32 // 添加到布隆过滤器的一系列 key 的 hash 值
}

func NewBloomFilter(m int) *BloomFilter {
	return &BloomFilter{
		m: m,
	}
}

func (bf *BloomFilter) Add(key []byte) {
	bf.hashedKeys = append(bf.hashedKeys, murmur3.Sum32(key))
}

func (bf *BloomFilter) Exist(bitmap, key []byte) bool {
	h := murmur3.Sum32(key)
	k := bf.calcK()
	for i := uint32(0); i < k; i++ {
		pos := bf.getPos(h, i)
		byteIndex := pos / 8
		bitOffset := pos % 8
		if byteIndex >= uint32(len(bitmap)) {
			return false
		}
		if bitmap[byteIndex]&(1<<bitOffset) == 0 {
			return false
		}
	}
	return true
}

func (bf *BloomFilter) Hash() []byte {
	bitmap := make([]byte, (bf.m+7)/8)
	k := bf.calcK()
	for _, h := range bf.hashedKeys {
		for i := uint32(0); i < k; i++ {
			pos := bf.getPos(h, i)
			byteIndex := pos / 8
			bitOffset := pos % 8
			bitmap[byteIndex] |= 1 << bitOffset
		}
	}
	return bitmap
}

func (bf *BloomFilter) Reset() {
	bf.hashedKeys = bf.hashedKeys[:0]
}

func (bf *BloomFilter) KeyLen() int {
	return len(bf.hashedKeys)
}

// calcK 计算最优哈希函数个数 k = (m/n) * ln2
func (bf *BloomFilter) calcK() uint32 {
	n := len(bf.hashedKeys)
	if n == 0 {
		return 1
	}
	k := uint32(float64(bf.m) / float64(n) * 0.693)
	if k < 1 {
		return 1
	}
	return k
}

// getPos 通过 double hashing 模拟第 i 个哈希函数: h(i) = (h1 + i*h2) % m
func (bf *BloomFilter) getPos(h uint32, i uint32) uint32 {
	h1 := h
	h2 := h>>17 | h<<15 // 简单旋转得到第二个哈希
	return (h1 + i*h2) % uint32(bf.m)
}
