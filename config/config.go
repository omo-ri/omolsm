package config

import (
	"fmt"
	"omolsm/internal/lsm/filter"
	"omolsm/internal/lsm/memtable"
	"os"
)

// ---------------------------------------------------------------------------
// Merge Iterator Factory — 统一 KV 覆盖模式 和 Bitmap OR 合并模式的关键
// ---------------------------------------------------------------------------

// MergeIterator 是 compaction 归并迭代器的最小接口.
// merge_iterator.go 和 bitmap_merge_iterator.go 都已满足此接口.
type MergeIterator interface {
	Next() bool
	Key() []byte
	Value() []byte
	Err() error
}

// SubIterator 是单个 SST node 的迭代器接口 (即 iterator.Iterator).
type SubIterator interface {
	Next() bool
	Key() []byte
	Value() []byte
	Err() error
}

// MergeIteratorFactory 根据一组子迭代器创建归并迭代器.
//   - KV 模式: 返回 NewMergeIterator (last-write-wins)
//   - Bitmap 模式: 返回 NewBitmapMergeIterator (OR 合并)
type MergeIteratorFactory func(iters []SubIterator) MergeIterator

// ---------------------------------------------------------------------------
// Hooks — LSM 运行时事件回调 (可观测性)
// ---------------------------------------------------------------------------

// Hooks 提供 LSM 关键事件的回调，所有字段可选 (nil = 不回调).
type Hooks struct {
	OnFlush      func(info FlushInfo)
	OnCompaction func(info CompactionInfo)
	OnWrite      func(key []byte, valueSize int)
}

// FlushInfo flush 事件详情.
type FlushInfo struct {
	Level   int    // 目标层 (通常为 0)
	SSTSize uint64 // 生成的 SST 文件大小
	KVCount int    // flush 的 kv 对数量
	MemSize int    // flush 前 memtable 字节数
}

// CompactionInfo compaction 事件详情.
type CompactionInfo struct {
	FromLevel  int    // 源层
	ToLevel    int    // 目标层
	InputFiles int    // 参与 compact 的 SST 数量
	OutputSize uint64 // 输出 SST 大小
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

type Config struct {
	Dir      string // sst 文件存放的目录
	MaxLevel int    // lsm tree 总共多少层

	// sst 相关
	SSTSize          int // 每个 sst table 大小，默认 4M (也是 memtable flush 阈值)
	SSTNumPerLevel   int // 每层多少个 sst table，默认 10 个
	SSTDataBlockSize int // sst table 中 block 大小 默认 4KB
	SSTFooterSize    int // sst table 中 footer 部分大小. 固定为 32B

	Filter              filter.Filter        // 供 Node 读路径 Exist 调用（只读，无状态）
	FilterConstructor   func() filter.Filter // 供 SSTWriter 创建独立实例（写路径，有状态）
	MemTableConstructor memtable.MemTableConstructor

	// 归并策略: KV 模式 vs Bitmap 模式
	MergeIteratorFactory MergeIteratorFactory

	// 可观测性
	Hooks Hooks
}

// NewConfig 配置文件构造器.
func NewConfig(dir string, opts ...ConfigOption) (*Config, error) {
	c := Config{
		Dir:                 dir,
		MaxLevel:            4,
		SSTSize:             4 * 1024 * 1024, // 4MB
		SSTDataBlockSize:    4 * 1024,        // 4KB
		SSTNumPerLevel:      10,
		SSTFooterSize:       32,
		Filter:              filter.NewBloomFilter(10240),
		FilterConstructor:   func() filter.Filter { return filter.NewBloomFilter(10240) },
		MemTableConstructor: memtable.NewTreeMapMemTable,
		// 默认: KV 覆盖模式 (nil 表示使用默认 MergeIterator, 在 tree 中处理)
		MergeIteratorFactory: nil,
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

// ---------------------------------------------------------------------------
// ConfigOption
// ---------------------------------------------------------------------------

type ConfigOption func(*Config)

func WithMaxLevel(maxLevel int) ConfigOption {
	return func(c *Config) { c.MaxLevel = maxLevel }
}

func WithSSTSize(sstSize int) ConfigOption {
	return func(c *Config) { c.SSTSize = sstSize }
}

func WithSSTDataBlockSize(sstDataBlockSize int) ConfigOption {
	return func(c *Config) { c.SSTDataBlockSize = sstDataBlockSize }
}

func WithSSTNumPerLevel(sstNumPerLevel int) ConfigOption {
	return func(c *Config) { c.SSTNumPerLevel = sstNumPerLevel }
}

func WithFilter(f filter.Filter) ConfigOption {
	return func(c *Config) { c.Filter = f }
}

func WithFilterConstructor(fc func() filter.Filter) ConfigOption {
	return func(c *Config) { c.FilterConstructor = fc }
}

func WithMemtableConstructor(mc memtable.MemTableConstructor) ConfigOption {
	return func(c *Config) { c.MemTableConstructor = mc }
}

// WithMergeIteratorFactory 注入归并迭代器工厂.
//   - 不调用此 option 或传 nil: 使用默认 KV 覆盖模式 (MergeIterator)
//   - 传 BitmapMergeIterator 工厂: bitmap OR 合并模式
func WithMergeIteratorFactory(f MergeIteratorFactory) ConfigOption {
	return func(c *Config) { c.MergeIteratorFactory = f }
}

// WithHooks 注入运行时事件回调.
func WithHooks(h Hooks) ConfigOption {
	return func(c *Config) { c.Hooks = h }
}
