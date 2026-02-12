# SSTReader 单元测试文档

## 1. 组件概述

`SSTReader` 负责从 SSTable 文件中读取数据。它是 `SSTWriter` 的对称组件，读取流程为：

```
ReadFooter() → 从文件尾部解析 filterOffset/filterSize/indexOffset/indexSize
ReadFilter() → 定位并解码 filter block，还原 offset → bitmap 映射
ReadIndex()  → 定位并解码 index block，还原 index 条目列表
ReadData()   → 读取全部 data block，逐条解码还原 KV 对
```

核心解码逻辑在 `ReadRecord` 中，负责还原 Block 的前缀压缩编码：

```
读取 sharedPrefixLen, diffKeyLen, valueLen (3 个 uvarint)
→ 读取 diffKey (diffKeyLen 字节)
→ 读取 value (valueLen 字节)
→ 拼接 fullKey = prevKey[:sharedPrefixLen] + diffKey
```

## 2. 测试策略

### 2.1 Writer-Reader 端到端验证

测试的核心思路是：**用 SSTWriter 写入已知数据，再用 SSTReader 读出，比对结果**。这同时验证了：
- Writer 的编码正确性
- Reader 的解码正确性
- 两者之间的格式兼容性

`writeSST` helper 封装了 Writer 侧的流程，返回 writer 的 size、blockToFilter、index 产出物，供 Reader 测试比对。

### 2.2 分层验证

不仅验证 `ReadData` 的最终结果，也分别验证 `ReadFooter`、`ReadFilter`、`ReadIndex` 各阶段的中间产物，确保每一层的解码都独立正确。

## 3. 测试矩阵

| 测试 | 类别 | 验证点 |
|---|---|---|
| `TestNewSSTReader` | 初始化 | 正常打开 SST 文件 |
| `TestNewSSTReader_InvalidFile` | 初始化/错误 | 不存在的文件返回 error |
| `TestReadFooter` | Footer | filterOffset < indexOffset，区间不重叠，size 非零 |
| `TestReadData_AllRecordsMatch` | 数据/端到端 | 20 条 KV 写入后全部读出，逐条比对 key 和 value |
| `TestReadData_MultipleBlocks` | 数据/多 block | 小 blockSize(32) + 50 条记录，跨多个 block 后读出全部正确 |
| `TestReadData_SingleRecord` | 数据/边界 | 单条记录的写入和读出 |
| `TestReadFilter` | Filter | 读出的 filter 条目数与 writer 产出一致，每个 offset 的 bitmap 逐字节比对 |
| `TestReadIndex` | Index | 读出的 index 条目数与 writer 一致，逐条比对 Key/Offset/Size，验证 key 有序 |
| `TestSize` | 元数据 | Size() 返回非零值 |
| `TestReadRecord_PrefixDecoding` | 前缀压缩 | 用 `user:001/002/003` 等有共同前缀的 key 验证解码还原 |
| `TestMultipleReads` | Seek 正确性 | 连续调用 ReadFooter → ReadFilter → ReadIndex → ReadData，验证文件指针 Seek 不互相干扰 |

## 4. 测试数据设计

- **`makeKVs(n)`**：生成 n 条 `key_000000` ~ `key_00000N` 格式的有序 KV，模拟 LSM-Tree 中 memtable flush 的典型数据
- **前缀压缩专项**：使用 `user:001/002/003` 等 key，共享前缀 `user:00`，验证 `ReadRecord` 的 sharedPrefixLen 解码和 key 拼接
- **小 blockSize(32)**：迫使频繁 block 切换，测试跨 block 边界的读取正确性

## 5. 注意事项

- 测试中 Writer 和 Reader 使用**同一个 `conf`**，但 Reader 需要**新的 Filter 实例**（因为 Writer 的 Filter 在 Finish 后状态已变）。部分测试在 `writeSST` 后重新赋值 `conf.Filter`。
- `ReadFilter` 和 `ReadIndex` 的测试直接与 Writer 的返回值比对，这依赖 Writer 实现的正确性。如果 Writer 有 bug，这些测试可能产生"两边都错但比对通过"的假象。`ReadData` 的测试与原始输入比对，不依赖 Writer 的中间产物。

## 6. 运行方式

```bash
go test -v ./sst_io/reader/
go test -race ./sst_io/reader/
go test -coverprofile=cover.out ./sst_io/reader/
go tool cover -html=cover.out
```

## 7. 覆盖率目标

| 方法 | 预期覆盖率 | 说明 |
|---|---|---|
| `NewSSTReader` | 100% | 含成功和失败路径 |
| `ReadFooter` | 100% | |
| `ReadFilter` | 100% | 含 footer 懒加载分支 |
| `ReadIndex` | 100% | 含 footer 懒加载分支 |
| `ReadData` | 100% | 含 footer 懒加载分支 |
| `ReadBlock` | 100% | 被 ReadFilter/ReadIndex/ReadData 间接调用 |
| `ReadRecord` | 100% | 被所有解码路径调用 |
| `readFilter` | 100% | |
| `readIndex` | 100% | |
| `ReadBlockData` | 100% | |
| `Size` | 100% | 含 footer 懒加载分支 |
| `Close` | 100% | |