# SSTWriter 单元测试文档

## 1. 组件概述

`SSTWriter` 负责将有序 KV 数据写入 SSTable 文件。SSTable 文件布局如下：

```
┌──────────────┬──────────────┬──────────────┬──────────┐
│  data blocks │ filter block │ index block  │  footer  │
└──────────────┴──────────────┴──────────────┴──────────┘
```

- **data blocks**：实际 KV 数据，按 `SSTDataBlockSize` 分块，块内使用前缀压缩
- **filter block**：每个 data block 对应一个布隆过滤器 bitmap，用于快速判断 key 是否可能存在
- **index block**：每个 data block 的最大 key + 对应的 offset/size，用于二分查找定位 block
- **footer**：固定大小（`SSTFooterSize` 字节），存储 4 个 uvarint：filterOffset、filterSize、indexOffset、indexSize

### 关键流程

- **写入**：`NewSSTWriter` → 多次 `Append(key, value)` → `Finish()` → `Close()`
- **block 切换**：当 `dataBlock.Size() >= SSTDataBlockSize` 时，自动将当前 block flush 到 `dataBuf`，同时生成该 block 的布隆过滤器和索引条目
- **Finish**：flush 最后一个 block，写入 filter block、index block 和 footer 到文件

---

## 2. 测试矩阵

| 测试 | 类别 | 验证点 |
|---|---|---|
| `TestNewSSTWriter` | 初始化 | 新建 writer 的 Size 为 0 |
| `TestNewSSTWriter_InvalidDir` | 初始化/错误 | 无效目录路径应返回 error |
| `TestSingleRecord` | 基本功能 | 单条 KV 写入后 Finish：返回的 size >0，blockToFilter 非空，index 非空，文件非空 |
| `TestMultipleBlocks` | 核心流程 | 小 blockSize(32) + 20 条记录 → 产生多个 block；blockToFilter 和 index 各有多条；index key 全局有序 |
| `TestFinish_NoAppend` | 边界 | 不写任何数据直接 Finish：不 panic，size=0，blockToFilter 为空 |
| `TestFooterFormat` | 文件格式 | 读取文件尾部 footer，解码 4 个 uvarint，验证 filter 和 index 区间不重叠、不越界 |
| `TestBlockToFilter_MatchesIndex` | 数据一致性 | 每个有数据的 index 条目（PrevBlockSize>0）在 blockToFilter 中都有对应的 bitmap |
| `TestSize_Grows` | 基本功能 | 多次 Append 后 Size() 递增（至少在 block flush 后增长） |
| `TestPrevKey_CallerReusesBuffer` | 内存安全 | 调用方复用同一个 key buffer 时，各 index 条目的 Key 应互相独立，不会全部变成最后一个 key |
| `TestPrevKey_MutationAfterAppend` | 内存安全 | Append 后调用方修改原始 key slice，不应影响 writer 内部已记录的 key |

---

## 3. 测试策略说明

### 3.1 端到端验证

`TestSingleRecord` 和 `TestMultipleBlocks` 通过完整的 Append → Finish → 检查返回值 + 检查文件的流程，验证写入管线的整体正确性。不深入解码 block 内部（那是 Block 组件的测试职责），而是关注 SSTWriter 层面的产出物：size、blockToFilter、index、文件是否存在。

### 3.2 文件格式验证

`TestFooterFormat` 直接读取生成的 SST 文件，从尾部解析 footer 中的 4 个 uvarint，验证：
- filter 区间 `[filterOffset, filterOffset+filterSize)` 不越过 index 起始位置
- index 区间 `[indexOffset, indexOffset+indexSize)` 不越过文件体（不含 footer）的末尾
- 两个区间不重叠

这确保了 SSTReader 可以根据 footer 正确定位 filter block 和 index block。

### 3.3 数据一致性

`TestBlockToFilter_MatchesIndex` 验证 blockToFilter 和 index 两个产出物的对应关系：每个实际包含数据的 block（PrevBlockSize>0）都应有对应的布隆过滤器 bitmap。这是读取时快速跳过不含目标 key 的 block 的前提。

### 3.4 内存安全（prevKey 深拷贝）

LSM-Tree 的 memtable 迭代器通常为了性能会复用同一个 key buffer。如果 SSTWriter 内部只保存了 key slice 的引用而非拷贝，后续写入会覆盖之前所有 index 条目的 Key。

两个测试分别覆盖两种场景：

- **`TestPrevKey_CallerReusesBuffer`**：用同一个 `buf` 写入 5 个不同的 key，Finish 后检查 index key 是否各不相同。如果内部是引用，所有 key 会变成最后一个 `"key_0004"`。
- **`TestPrevKey_MutationAfterAppend`**：Append `"original"` 后立即将 slice 改写为 `"XXXXXXXX"`，检查 index 中是否被污染。

### 3.5 边界场景

- **空 Finish**：不写任何数据直接调用 Finish，验证不 panic 且返回零值
- **无效目录**：NewSSTWriter 传入不存在的路径，验证返回 error

---

## 4. 测试配置说明

测试通过 `newTestWriter` helper 构造 writer，关键参数：

| 参数 | 测试中的值 | 说明 |
|---|---|---|
| `SSTFooterSize` | 32 | 与默认配置一致 |
| `SSTDataBlockSize` | 16 或 32 | 极小值，确保少量记录就能触发多次 block 切换 |
| `Filter` | `BloomFilter(1024)` | 1024 bit 的布隆过滤器，足够测试使用 |

使用 `t.TempDir()` 创建临时目录，测试结束自动清理。使用 `t.Cleanup` 确保 writer 被关闭。

---

## 5. 运行方式

```bash
# 运行全部测试
go test -v ./sst_io/

# 带竞态检测
go test -race ./sst_io/

# 运行特定测试
go test -v -run TestPrevKey ./sst_io/

# 覆盖率
go test -coverprofile=cover.out ./sst_io/
go tool cover -html=cover.out
```

---

## 6. 覆盖率目标

| 方法 | 预期覆盖率 | 说明 |
|---|---|---|
| `NewSSTWriter` | 100% | 含成功和失败路径 |
| `Append` | 100% | 含 block 未满和超限两个分支 |
| `Finish` | 100% | |
| `Size` | 100% | |
| `Close` | 100% | |
| `insertIndex` | 100% | 通过 Append 和 Finish 间接触发 |
| `refreshBlock` | ~90% | 含 KeyLen()==0 的提前返回分支（通过 TestFinish_NoAppend 覆盖） |