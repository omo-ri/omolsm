package filter

import (
	"math"

	"github.com/spaolacci/murmur3"
)

const maxK uint32 = 30

type BloomFilter struct {
	m          int
	hashedKeys []uint32
}

func NewBloomFilter(m int) *BloomFilter {
	return &BloomFilter{
		m:          m,
		hashedKeys: make([]uint32, 0),
	}
}

func (bf *BloomFilter) Add(key []byte) {
	bf.hashedKeys = append(bf.hashedKeys, murmur3.Sum32(key))
}

// Exist 判断 key 是否可能存在于 bitmap 中.
// bitmap 格式: [k_byte] [bitmap_data...]
// 第一个字节存储生成时使用的哈希函数个数 k，后续为实际 bitmap 数据.
func (bf *BloomFilter) Exist(bitmap, key []byte) bool {
	if len(bitmap) < 1 {
		return false
	}

	// 从 bitmap 头部读取 k
	k := uint32(bitmap[0])
	if k == 0 {
		return false
	}
	data := bitmap[1:]

	h := murmur3.Sum32(key)
	for i := uint32(0); i < k; i++ {
		pos := bf.getPos(h, i)
		byteIndex := pos / 8
		bitOffset := pos % 8
		if byteIndex >= uint32(len(data)) {
			return false
		}
		if data[byteIndex]&(1<<bitOffset) == 0 {
			return false
		}
	}
	return true
}

// Hash 生成带 k 值头部的 bitmap.
// 返回格式: [k_byte] [bitmap_data...]
func (bf *BloomFilter) Hash() []byte {
	k := bf.calcK()

	bitmapData := make([]byte, (bf.m+7)/8)
	for _, h := range bf.hashedKeys {
		for i := uint32(0); i < k; i++ {
			pos := bf.getPos(h, i)
			byteIndex := pos / 8
			bitOffset := pos % 8
			bitmapData[byteIndex] |= 1 << bitOffset
		}
	}

	// 头部 1 字节存 k，后面跟 bitmap 数据
	result := make([]byte, 1+len(bitmapData))
	result[0] = byte(k)
	copy(result[1:], bitmapData)
	return result
}

func (bf *BloomFilter) Reset() {
	bf.hashedKeys = bf.hashedKeys[:0]
}

func (bf *BloomFilter) KeyLen() int {
	return len(bf.hashedKeys)
}

// calcK 计算最优哈希函数个数 k = round((m/n) * ln2)
// 下限 1，上限 maxK(30)
func (bf *BloomFilter) calcK() uint32 {
	n := len(bf.hashedKeys)
	if n == 0 {
		return 1
	}
	k := uint32(math.Round(float64(bf.m) / float64(n) * math.Ln2))
	if k < 1 {
		return 1
	}
	if k > maxK {
		return maxK
	}
	return k
}

// getPos 通过 double hashing 模拟第 i 个哈希函数: h(i) = (h1 + i*h2) % m
func (bf *BloomFilter) getPos(h uint32, i uint32) uint32 {
	h1 := h
	h2 := h>>17 | h<<15 // 简单旋转得到第二个哈希
	return (h1 + i*h2) % uint32(bf.m)
}
