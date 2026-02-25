package config

import (
	"fmt"
	"omolsm/internal/filter"
	"omolsm/internal/memtable"
	"os"
)

type Config struct {
	Dir      string // sst 文件存放的目录
	MaxLevel int    // lsm tree 总共多少层

	// sst 相关
	SSTSize          int // 每个 sst table 大小，默认 4M
	SSTNumPerLevel   int // 每层多少个 sst table，默认 10 个
	SSTDataBlockSize int // sst table 中 block 大小 默认 16KB
	SSTFooterSize    int // sst table 中 footer 部分大小. 固定为 32B

	Filter              filter.Filter                // 供 Node 读路径 Exist 调用（只读，无状态）
	FilterConstructor   func() filter.Filter         // 供 SSTWriter 创建独立实例（写路径，有状态）
	MemTableConstructor memtable.MemTableConstructor // memtable 构造器，默认为跳表
}

// NewConfig 配置文件构造器.
func NewConfig(dir string, opts ...ConfigOption) (*Config, error) {
	c := Config{
		Dir:                 dir,
		MaxLevel:            4,
		SSTSize:             4 * 1024 * 1024, // 4MB，memtable 大小阈值
		SSTDataBlockSize:    4 * 1024,        // 4KB，每个 block 大小
		SSTNumPerLevel:      10,
		SSTFooterSize:       32,
		Filter:              filter.NewBloomFilter(10240), // 10240 bit ≈ 1.25KB
		FilterConstructor:   func() filter.Filter { return filter.NewBloomFilter(10240) },
		MemTableConstructor: memtable.NewTreeMapMemTable,
	}

	for _, opt := range opts {
		opt(&c)
	}

	return &c, c.check()
}

func (c *Config) check() error {
	if _, err := os.ReadDir(c.Dir); err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(c.Dir, os.ModePerm); err != nil {
				return fmt.Errorf("创建目录失败: %w", err)
			}
		} else {
			return fmt.Errorf("检查目录失败: %w", err)
		}
	}
	return nil
}

type ConfigOption func(*Config)

// WithMaxLevel lsm tree 最大层数. 默认为 7 层.
func WithMaxLevel(maxLevel int) ConfigOption {
	return func(c *Config) {
		c.MaxLevel = maxLevel
	}
}

// WithSSTSize level0层每个 sstable 文件的大小，单位 byte. 默认为 1 MB.
// 且每加深一层，sstable 文件大小限制阈值放大 10 倍.
func WithSSTSize(sstSize int) ConfigOption {
	return func(c *Config) {
		c.SSTSize = sstSize
	}
}

// WithSSTDataBlockSize sstable 中每个 block 块的大小限制. 默认为 16KB.
func WithSSTDataBlockSize(sstDataBlockSize int) ConfigOption {
	return func(c *Config) {
		c.SSTDataBlockSize = sstDataBlockSize
	}
}

// WithSSTNumPerLevel 每个 level 层预期最多存放的 sstable 文件个数. 默认为 10 个.
func WithSSTNumPerLevel(sstNumPerLevel int) ConfigOption {
	return func(c *Config) {
		c.SSTNumPerLevel = sstNumPerLevel
	}
}

// WithFilter 注入过滤器的具体实现. 默认使用本项目下实现的布隆过滤器 bloom filter.
func WithFilter(f filter.Filter) ConfigOption {
	return func(c *Config) {
		c.Filter = f
	}
}

// WithFilterConstructor 注入过滤器构造器，供 SSTWriter 为每个实例创建独立 filter.
func WithFilterConstructor(fc func() filter.Filter) ConfigOption {
	return func(c *Config) {
		c.FilterConstructor = fc
	}
}

// WithMemtableConstructor 注入有序表构造器. 默认使用本项目下实现的跳表 skiplist.
func WithMemtableConstructor(memtableConstructor memtable.MemTableConstructor) ConfigOption {
	return func(c *Config) {
		c.MemTableConstructor = memtableConstructor
	}
}
