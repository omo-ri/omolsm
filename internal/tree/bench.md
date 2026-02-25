# OmoLSM（tree 包）基准测试说明与结果分析指南

## 一、目的与总体设计

本基准旨在以单线程方式评估 OmoLSM（tree 包）在不同工作负载和参数配置下的主要性能特征，关注点包括：

- 写入吞吐（Put）
- 随机读（Get）命中/未命中
- 范围查询（Scan）
- 删除（Tombstone）
- 混合负载表现
- 参数敏感性（以 SSTSize 为示例）
- 空间放大（space amplification）的近似测量与写放大（write amplification）分析方向

> 说明：测试为单线程（与你要求一致）。真实生产环境通常为并发/异步 compaction，因此结果用于比较/定位而非直接用于容量规划。

---

## 二、如何运行

在仓库根目录或合适的包目录运行：

```bash
# 运行全部 benchmark（也会输出 ReportMetric 自定义指标）
go test ./tree -run none -bench . -benchmem
