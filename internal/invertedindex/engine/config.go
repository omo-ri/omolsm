package engine

import (
	"fmt"
	"os"

	"omolsm/config"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the inverted index engine.
type Config struct {
	Source          SourceConfig          `yaml:"source"`
	Language        string                `yaml:"language"`
	Storage         StorageConfig         `yaml:"storage"`
	Index           PipelineConfig        `yaml:"index"`
	Query           PipelineConfig        `yaml:"query"`
	CustomStopWords CustomStopWordsConfig `yaml:"custom_stop_words"`
}

// StorageConfig controls the backend for dictionary and inverted index.
type StorageConfig struct {
	Backend   string `yaml:"backend"`    // "memory" (default) or "lsm"
	DataDir   string `yaml:"data_dir"`   // 新增，默认 ".lsm-data"
	BlockSize int    `yaml:"block_size"` // docID partition size for LSM index, default 1024

	// LSM 参数 — 直接映射到 config.Config, 不再重复定义语义
	SSTSize        int `yaml:"sst_size"`          // memtable flush threshold bytes, default 4MB
	SSTNumPerLevel int `yaml:"sst_num_per_level"` // SSTs per level before compaction, default 10
	SSTBlockSize   int `yaml:"sst_block_size"`    // data block size in SST, default 4KB
	MaxLevel       int `yaml:"max_level"`         // LSM tree max levels, default 4
}

// ToLSMConfigOptions 将 StorageConfig 转为 config.ConfigOption 切片.
// 集中管理映射关系, engine.go 不再散落转换代码.
func (sc *StorageConfig) ToLSMConfigOptions() []config.ConfigOption {
	var opts []config.ConfigOption
	if sc.SSTSize > 0 {
		opts = append(opts, config.WithSSTSize(sc.SSTSize))
	}
	if sc.SSTNumPerLevel > 0 {
		opts = append(opts, config.WithSSTNumPerLevel(sc.SSTNumPerLevel))
	}
	if sc.SSTBlockSize > 0 {
		opts = append(opts, config.WithSSTDataBlockSize(sc.SSTBlockSize))
	}
	if sc.MaxLevel > 0 {
		opts = append(opts, config.WithMaxLevel(sc.MaxLevel))
	}
	return opts
}

type SourceConfig struct {
	Dir        string   `yaml:"dir"`
	Encoding   string   `yaml:"encoding"`
	Extensions []string `yaml:"extensions"`
}

type PipelineConfig struct {
	StopWords    bool `yaml:"stop_words"`
	Stemming     bool `yaml:"stemming"`
	StripAccents bool `yaml:"strip_accents"`
	Normalize    bool `yaml:"normalize"`
	MinTermLen   int  `yaml:"min_term_length"`
}

type CustomStopWordsConfig struct {
	English []string `yaml:"english"`
	Russian []string `yaml:"russian"`
}

func DefaultConfig() *Config {
	return &Config{
		Source: SourceConfig{
			Dir:        "./documents",
			Encoding:   "utf-8",
			Extensions: []string{".txt"},
		},
		Language: "auto",
		Storage: StorageConfig{
			Backend:        "memory",
			BlockSize:      1024,
			SSTSize:        4 * 1024 * 1024,
			SSTNumPerLevel: 10,
			SSTBlockSize:   4 * 1024,
			MaxLevel:       4,
		},
		Index: PipelineConfig{
			StopWords: true, Stemming: true, Normalize: true, MinTermLen: 1,
		},
		Query: PipelineConfig{
			Stemming: true, Normalize: true, MinTermLen: 1,
		},
	}
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Source.Dir == "" {
		return fmt.Errorf("source.dir is required")
	}

	lang := c.Language
	if lang != "auto" && lang != "english" && lang != "russian" && lang != "mixed" {
		return fmt.Errorf("language must be auto, english, russian, or mixed, got %q", lang)
	}

	if c.Index.Stemming != c.Query.Stemming {
		return fmt.Errorf("index.stemming and query.stemming must match")
	}
	if c.Index.StripAccents != c.Query.StripAccents {
		return fmt.Errorf("index.strip_accents and query.strip_accents must match")
	}
	if c.Index.Normalize != c.Query.Normalize {
		return fmt.Errorf("index.normalize and query.normalize must match")
	}

	backend := c.Storage.Backend
	if backend != "" && backend != "memory" && backend != "lsm" {
		return fmt.Errorf("storage.backend must be 'memory' or 'lsm', got %q", backend)
	}

	// 设置默认值
	if c.Storage.BlockSize <= 0 {
		c.Storage.BlockSize = 1024
	}
	if c.Storage.SSTSize <= 0 {
		c.Storage.SSTSize = 4 * 1024 * 1024
	}
	if c.Storage.SSTNumPerLevel <= 0 {
		c.Storage.SSTNumPerLevel = 10
	}
	if c.Storage.SSTBlockSize <= 0 {
		c.Storage.SSTBlockSize = 4 * 1024
	}
	if c.Storage.MaxLevel <= 0 {
		c.Storage.MaxLevel = 4
	}

	return nil
}
