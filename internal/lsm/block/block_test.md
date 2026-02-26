# Block 组件单元测试文档

## 1. 组件概述

`Block` 是 LSM-Tree 实现中的核心数据结构，负责将有序的 KV 对以**前缀压缩**（prefix compression）的紧凑二进制格式写入内存缓冲区，并支持 flush 到外部 `io.Writer`。

### 编码格式

每条记录的二进制布局如下：

```
┌──────────────────┬──────────────────┬──────────────┬──────────────┬───────────┐
│ sharedPrefixLen  │  diffKeyLen      │  valueLen    │  diffKey     │  value    │
│  (uvarint)       │  (uvarint)       │  (uvarint)   │  (bytes)     │  (bytes)  │
└──────────────────┴──────────────────┴──────────────┴──────────────┴───────────┘
```

- `sharedPrefixLen`：与前一条记录 key 的公共前缀长度（第一条记录为 0）
- `diffKeyLen`：当前 key 去掉公共前缀后的剩余长度
- `valueLen`：value 的长度
- `diffKey`：key 的非公共部分（`key[sharedPrefixLen:]`）
- `value`：完整 value 数据

还原完整 key：`fullKey = prevKey[:sharedPrefixLen] + diffKey`

---

## 2. 测试矩阵

| 测试函数 | 测试目标 | 类别 |
|---|---|---|
| `TestNewBlock_InitialState` | 新建 Block 的初始状态（Size=0, KvsCnt=0, ToBytes 为空） | 初始化 |
| `TestAppend_SingleRecord` | 追加单条 KV 并解码验证正确性 | 基本功能 |
| `TestAppend_NoSharedPrefix` | 两条无公共前缀的 key，验证解码正确 | 前缀压缩 |
| `TestAppend_SharedPrefix` | 三条有公共前缀的 key，验证压缩和解码正确性 | 前缀压缩（核心） |
| `TestAppend_IdenticalKeys` | 完全相同的 key 追加两次 | 前缀压缩（边界） |
| `TestAppend_EmptyKeyAndValue` | 空 key、空 value、空 key+空 value 组合 | 边界值 |
| `TestAppend_ManyRecords` | 追加 1000 条记录，验证计数和首尾解码 | 压力 |
| `TestAppend_BinaryData` | 包含 0x00 和 0xFF 等特殊字节的二进制 key/value | 边界值 |
| `TestAppend_KeyIsPrefixOfPrev` | 当前 key 比前一个 key 短且是其前缀 | 前缀压缩（边界） |
| `TestAppend_LargeKeyValue` | 10KB key + 50KB value | 压力 |
| `TestAppend_PrevKeyNotMutated` | 确保 Append 不会修改调用方传入的 key slice | 内存安全 |
| `TestSize_GrowsWithAppend` | 每次 Append 后 Size 递增 | 基本功能 |
| `TestToBytes_ReturnsConsistentData` | 连续调用 ToBytes 返回相同数据 | 幂等性 |
| `TestToBytes_LenMatchesSize` | `len(ToBytes()) == Size()` | 一致性 |
| `TestFlushTo_WritesAllData` | FlushTo 写出的数据与 ToBytes 一致，返回值正确 | 基本功能 |
| `TestFlushTo_ClearsBlock` | FlushTo 后 block 被清空（Size=0, KvsCnt=0） | 状态重置 |
| `TestFlushTo_BlockReusableAfterFlush` | Flush 后 block 可重新写入，前缀压缩从头开始 | 复用 |
| `TestFlushTo_WriterError` | 底层 Writer 返回错误时的行为（错误透传，block 仍被清空） | 错误处理 |
| `TestFlushTo_EmptyBlock` | 对空 block 执行 FlushTo | 边界值 |
| `TestGetKvsCnt_Increments` | KvsCnt 随每次 Append 递增 | 计数器 |
| `TestClear_ResetsPrevKeyForPrefixCompression` | Flush 后 prevKey 被重置，新记录的 sharedPrefixLen=0 | 状态重置 |
| `TestEncoding_FirstRecord_NoSharedPrefix` | 白盒验证第一条记录的 uvarint 头部和载荷 | 编码格式 |
| `TestEncoding_SharedPrefix` | 白盒验证第二条记录的 sharedPrefixLen 和 diffKey | 编码格式 |
| `TestMultipleFlushCycles` | 5 轮 Append→Flush→解码 循环 | 复用/稳定性 |

---

## 3. 测试策略说明

### 3.1 解码验证（端到端）

大部分测试采用"写入 → 解码 → 比对"的端到端策略。测试辅助函数 `decodeRecord` / `decodeAllRecords` 独立实现了与 `Append` 反向的解码逻辑，能够从 Block 的二进制输出中还原出原始 KV 对。这确保：

- **不依赖被测组件自身的读取逻辑**（Block 没有 Read 方法）
- 如果编码格式出现 bug（如 varint 写反、前缀长度计算错误），解码会得到错误结果从而被捕获

### 3.2 白盒测试

`TestEncoding_*` 系列直接解析二进制字节，逐字段验证 uvarint 头部的值，确保编码格式完全符合预期。这对于 LSM-Tree 的跨组件兼容性至关重要——Block 的编码格式是 SSTable Reader 正确解析的前提。

### 3.3 前缀压缩

前缀压缩是 Block 的核心优化点，覆盖了以下场景：

- 无公共前缀（sharedPrefixLen=0）
- 有公共前缀（sharedPrefixLen>0 且 <len(key)）
- 完全相同的 key（sharedPrefixLen=len(key)，diffKeyLen=0）
- 当前 key 是前一个 key 的前缀（较短 key 跟在较长 key 后面）
- Flush 后 prevKey 被重置

### 3.4 状态生命周期

Block 支持写入→Flush→复用的循环。测试验证了：

- `FlushTo` 会调用 `clear()` 重置全部状态
- 即使写入失败（Writer 返回 error），Block 仍会被清空（`defer` 保证）
- Flush 后 prevKey 被清空，不影响后续写入的前缀压缩

### 3.5 错误与边界

- 空 key / 空 value / 空 Block Flush
- 大数据（10KB key + 50KB value）
- 二进制数据（含 0x00, 0xFF）
- Writer 失败时的错误传播

---

## 4. 已知的设计注意点

1. **`FlushTo` 的 `defer clear()`**：无论 `dest.Write` 是否成功，block 都会被清空。这是设计决策（而非 bug），测试 `TestFlushTo_WriterError` 对此进行了验证。如果未来需要在写入失败时保留数据用于重试，需要修改此行为。

2. **`prevKey` 的内存管理**：`Append` 使用 `b.prevKey = append(b.prevKey[:0], key...)` 复用底层数组，避免了每次分配。测试 `TestAppend_PrevKeyNotMutated` 确认了不会影响调用方的 slice。

3. **`conf` 字段**：当前 Block 的所有方法均未使用 `conf`，但 `NewBlock` 接收它。测试中传入 `nil` 即可。

---

## 5. 运行方式

```bash
# 运行全部测试
go test -v ./block/

# 运行特定测试
go test -v -run TestAppend_SharedPrefix ./block/

# 带竞态检测
go test -race -v ./block/

# 带覆盖率
go test -coverprofile=coverage.out ./block/
go tool cover -html=coverage.out
```

---

## 6. 覆盖率目标

| 方法 | 预期行覆盖率 |
|---|---|
| `NewBlock` | 100% |
| `Append` | 100% |
| `Size` | 100% |
| `FlushTo` | 100% |
| `ToBytes` | 100% |
| `GetKvsCnt` | 100% |
| `clear` | 100%（通过 FlushTo 间接覆盖） |