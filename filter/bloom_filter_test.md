# BloomFilter 单元测试文档

## 组件概述

`BloomFilter` 用于快速判断 key 是否可能存在于某个 SSTable 中。核心属性：**无假阴性，允许假阳性**。

写入流程：`Add(key)` → `Hash()` → 持久化 bitmap  
查询流程：`Exist(bitmap, key)` → 从 bitmap 判断

## 修复说明

原实现中 `Exist()` 调用 `calcK()` 计算哈希函数个数，而 `calcK()` 依赖 `len(hashedKeys)`。当 `Reset()` 后或用新实例查询时，k 值与生成 bitmap 时不一致，导致查询结果错误。

修复方案：bitmap 格式改为 `[k_byte][bitmap_data...]`，`Exist` 从 `bitmap[0]` 读取 k。同时 `calcK` 改用四舍五入并加上限 30。

## 测试矩阵

| 测试 | 验证点 |
|---|---|
| `TestNewBloomFilter` | 初始状态 |
| `TestAdd_And_KeyLen` | Add 计数 |
| `TestExist_NoFalseNegatives` | 200 key 无假阴性（核心属性） |
| `TestExist_AfterReset_UsesEncodedK` | **修复验证**：Reset 后查询仍正确 |
| `TestExist_NewInstance_UsesEncodedK` | **修复验证**：新实例读旧 bitmap 正确 |
| `TestHash_Format` | bitmap 长度和 k 字节编码 |
| `TestHash_Deterministic` | 相同输入生成相同 bitmap |
| `TestFalsePositiveRate` | 5 万次查询 FP 率 < 5% |
| `TestReset` | Reset 清空状态，bitmap 全零 |
| `TestCalcK` | 多组 m/n 验证四舍五入计算 |
| `TestCalcK_Bounds` | k 下限 1，上限 maxK |
| `TestExist_EdgeCases` | nil/空/k=0 bitmap 不 panic |
| `TestSmallM` | m=1 极端场景 |
| `TestAdd_EmptyKey` | 空 key 可正常添加和查询 |

## 运行

```bash
go test -v ./filter/
go test -race ./filter/
go test -bench=. ./filter/
```