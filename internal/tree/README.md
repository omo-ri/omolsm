# Compact 模块

LSM-Tree 的 compaction 实现，负责将 MemTable 刷盘到 Level 0 以及各层之间的 SSTable 合并。

## 整体流程

```
MemTable 写满
    │
    ▼
flushMemTable()          将 MemTable 有序数据写入 Level 0 新 SST 文件
    │
    ▼
cascadeCompact()         从 Level 0 开始逐层检查
    │
    ├─ Level 0 节点数 >= 阈值？──是──▶ compactLevel(0)  合并到 Level 1
    │                                      │
    │                                      ▼
    ├─ Level 1 节点数 >= 阈值？──是──▶ compactLevel(1)  合并到 Level 2
    │                                      │
    ...                                   ...
    │
    └─ 节点数 < 阈值 ──▶ 停止
```

## 核心方法

### flushMemTable

将内存中的 MemTable 刷盘为 Level 0 的 SST 文件：

1. 从 MemTable 获取所有有序 KV 对
2. 创建 SSTWriter，逐条写入
3. 调用 `Finish()` 落盘，生成 blockToFilter 和 index
4. 将新节点插入 Level 0
5. 触发 `cascadeCompact()` 级联检查

### cascadeCompact

从 Level 0 开始，逐层检查是否需要合并。当某层节点数 `>= SSTNumPerLevel` 时触发该层 compact，合并完成后继续检查下一层，直到某层不满足条件或到达最后一层。

### compactLevel

单层合并的核心逻辑，使用**多路归并迭代器**避免全量加载：

1. 为当前层每个 Node 创建 `NodeIterator`（懒加载，不读数据）
2. 将所有迭代器传入 `MergeIterator`（最小堆驱动的多路归并）
3. 流式地从迭代器取出 KV，写入 Level+1 的 SSTWriter
4. 落盘后删除旧节点

## 多路归并迭代器

这是本模块的核心优化，解决了"compact 时所有 SST 数据同时驻留内存"的问题。

### 问题

旧实现把每个 SST 全部数据读进内存：

```go
for _, n := range nodes {
    kvs, _ := n.GetAll()   // 整个 SST 全量加载
    for _, kv := range kvs {
        merged[string(kv.Key)] = kv.Value
    }
}
```

10 个 4MB 的 SST → 峰值内存约 40MB，且随数据量线性增长。

### 解决方案

两层迭代器抽象：

```
┌──────────────────────────────────────────┐
│           MergeIterator (最小堆)          │
│                                          │
│  堆中始终只有 N 个元素 (N = SST 数量)      │
│  每次弹出全局最小 key，自动去重             │
│                                          │
│  ┌────────────┐ ┌────────────┐           │
│  │NodeIter #0 │ │NodeIter #1 │  ...      │
│  │            │ │            │           │
│  │ 当前 block │ │ 当前 block │           │
│  │ 的 KV 数据 │ │ 的 KV 数据 │           │
│  └────────────┘ └────────────┘           │
└──────────────────────────────────────────┘
```

- **NodeIterator**：逐 block 遍历单个 SST，内存中最多持有一个 block 的数据
- **MergeIterator**：用最小堆从 N 个 NodeIterator 中选出全局最小的 KV，相同 key 只保留最新值

### 内存占用对比

| 场景 (10 个 SST × 4MB) | 旧实现 | 新实现 |
|------------------------|--------|--------|
| 峰值内存               | ~40 MB | N 个 block + 堆 ≈ 几十 KB |

### 去重机制

迭代器的下标顺序决定数据新旧：下标越大越新。堆的排序规则为先按 key 升序，key 相同时 idx 大的（更新的）优先弹出。弹出一个 key 后，跳过堆中所有相同 key 的元素，确保只保留最新值。

## 文件命名规则

SST 文件名格式为 `{level}_{seq}.sst`，例如 `0_1.sst` 表示 Level 0 的第 1 个 SST 文件。每层的 seq 通过 `atomic.Int32` 单调递增，保证文件名唯一。

## 节点管理

- **insertNode**：创建 SSTReader 并构造 Node，追加到对应层的节点列表末尾
- **removeNodes**：用 set 做快速查找，匹配的调用 `Destroy()`（关闭 reader + 删除磁盘文件），不匹配的保留

## 已知限制与改进方向

1. **Level+1 层已有数据未参与归并**：当前只合并 Level 层的节点写入 Level+1，不会读取 Level+1 已有的 SST，可能导致 Level+1 层出现 key 范围重叠。经典做法是同时将 Level+1 中 key 范围有交集的节点也纳入归并。

2. **单文件输出**：所有合并数据写入一个 SST 文件，没有按 `SSTSize` 拆分，数据量大时会产生超大文件。

3. **错误传播**：`log.Printf + return` 使调用方无法感知 compact 是否成功，建议返回 `error`。