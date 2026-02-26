package iterator

import "omolsm/config"

// ---------------------------------------------------------------------------
// Factory functions — 供 config.WithMergeIteratorFactory() 使用
// ---------------------------------------------------------------------------

// 因为 iterator.Iterator 和 config.SubIterator 方法签名完全一致,
// NodeIterator 已经同时满足两个接口, 无需额外适配.

// KVMergeFactory 返回 KV 覆盖模式的归并迭代器工厂 (last-write-wins).
func KVMergeFactory() config.MergeIteratorFactory {
	return func(subs []config.SubIterator) config.MergeIterator {
		iters := make([]Iterator, len(subs))
		for i, s := range subs {
			iters[i] = s.(Iterator)
		}
		return NewMergeIterator(iters)
	}
}

// BitmapMergeFactory 返回 bitmap OR 合并模式的归并迭代器工厂.
// 倒排索引 compaction 使用: 相同 key 的 value 做 roaring bitmap OR 合并.
func BitmapMergeFactory() config.MergeIteratorFactory {
	return func(subs []config.SubIterator) config.MergeIterator {
		iters := make([]Iterator, len(subs))
		for i, s := range subs {
			iters[i] = s.(Iterator)
		}
		return NewBitmapMergeIterator(iters)
	}
}
