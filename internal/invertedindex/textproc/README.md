# textproc

文本预处理管道，为倒排索引提供 term 提取能力。支持英文和俄文。

## 处理流程

```
原始文本
  │
  ▼  Tokenizer          按 Unicode 字母/数字边界分词
  │                      "Hello, мир!" → ["Hello", "мир"]
  │
  ▼  Normalizer          小写 + Unicode NFC 归一化 + 可选去重音
  │                      ["Hello", "МИР"] → ["hello", "мир"]
  │
  ▼  StopWordFilter      移除停用词 (the, is, и, в, ...)
  │                      ["the", "quick", "fox"] → ["quick", "fox"]
  │
  ▼  Stemmer             Snowball 词干提取
  │                      ["running", "dogs"] → ["run", "dog"]
  │
  ▼  去重
  │
  结果: Lang + Terms + Tokens
```

语言通过 Unicode 字符分布自动检测：西里尔字母占多数判定为俄文，否则为英文。

## 快速开始

```go
import "omolsm/internal/textproc"

// 默认配置，自动检测语言
p := textproc.NewPipeline()
result := p.Process("The quick brown foxes are jumping over the lazy dogs")

result.Lang   // "english"
result.Terms  // ["quick", "brown", "fox", "jump", "lazi", "dog"]
result.Tokens // 完整 token 流（带 position，未去重）
```

```go
// 俄文
result := p.Process("Быстрая коричневая лиса прыгает через ленивую собаку")

result.Lang   // "russian"
result.Terms  // ["быстр", "коричнев", "лис", "прыга", "ленив", "собак"]
```

## 配置

所有阶段都可通过 functional options 替换或禁用：

```go
// 强制指定语言
p := textproc.NewPipeline(textproc.WithLanguage(textproc.LangRussian))

// 禁用停用词过滤（查询侧场景）
p := textproc.NewPipeline(textproc.WithStopWordFilter(nil))

// 禁用词干提取
p := textproc.NewPipeline(textproc.WithStemmer(nil))

// 启用去重音（café → cafe）
p := textproc.NewPipeline(
    textproc.WithNormalizer(textproc.NewUnicodeNormalizer(textproc.WithStripAccents())),
)

// 自定义分词器
p := textproc.NewPipeline(textproc.WithTokenizer(myTokenizer))
```

## 模块结构

```
textproc/
├── pipeline.go       Pipeline 编排器 + functional options
├── tokenizer.go      Tokenizer 接口 + UnicodeTokenizer
├── normalizer.go     Normalizer 接口 + UnicodeNormalizer (NFC + 小写 + 去重音)
├── stopwords.go      StopWordFilter 接口 + 内置英文/俄文停用词表
├── stemmer.go        Stemmer 接口 + SnowballStemmer 适配器
├── lang.go           Lang 常量 + 语言检测
└── pipeline_test.go  测试
```

## 各组件说明

### Tokenizer

基于 `strings.FieldsFunc`，以非字母非数字字符作为分隔符切割文本。`unicode.IsLetter` 天然支持拉丁和西里尔字母，无需特殊处理。

### Normalizer

三步处理：Unicode NFC 归一化 → 转小写 → 可选去重音。去重音的原理是将字符串分解为 NFD 形式，移除所有组合标记（`unicode.Mn`），再重组为 NFC。

### StopWordFilter

数据结构为 `map[Lang]map[string]struct{}`，两层 map：第一层按语言分区，第二层是该语言的停用词 set。查询是 O(1) 的 map 查找。内置英文约 120 个、俄文约 150 个常用停用词，可通过 `AddCustomWords` 扩展。

### Stemmer

封装 `github.com/kljensen/snowball`，支持英文和俄文。对 `LangUnknown` 直接返回原词。

### 语言检测

统计文本中西里尔字母和拉丁字母的数量，西里尔超过 50% 判定为俄文，否则为英文。

## 索引侧 vs 查询侧

建索引和查询时可以使用不同的 pipeline 配置：

```go
// 建索引：完整 pipeline
indexPipeline := textproc.NewPipeline()

// 查询：不去停用词（允许用户搜索任意词）
queryPipeline := textproc.NewPipeline(textproc.WithStopWordFilter(nil))
```

## 依赖

```bash
go get github.com/kljensen/snowball
go get golang.org/x/text
```

## 测试

```bash
go test ./internal/textproc/ -v
```