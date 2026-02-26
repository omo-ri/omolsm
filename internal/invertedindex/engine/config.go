package engine

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the inverted index engine.
type Config struct {
	Source          SourceConfig          `yaml:"source"`
	Language        string                `yaml:"language"`
	Index           PipelineConfig        `yaml:"index"`
	Query           PipelineConfig        `yaml:"query"`
	CustomStopWords CustomStopWordsConfig `yaml:"custom_stop_words"`
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

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Source: SourceConfig{
			Dir:        "./documents",
			Encoding:   "utf-8",
			Extensions: []string{".txt"},
		},
		Language: "auto",
		Index: PipelineConfig{
			StopWords:    true,
			Stemming:     true,
			StripAccents: false,
			Normalize:    true,
			MinTermLen:   1,
		},
		Query: PipelineConfig{
			StopWords:    false,
			Stemming:     true,
			StripAccents: false,
			Normalize:    true,
			MinTermLen:   1,
		},
	}
}

// LoadConfig reads a YAML config file, falling back to defaults for missing fields.
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

	return nil
}
