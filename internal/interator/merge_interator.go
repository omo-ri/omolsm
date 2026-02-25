// ========================
// 文件 3: internal/iterator/merge_iterator.go
// 多路归并迭代器，用最小堆驱动
// ========================

package iterator

import (
	"bytes"
	"container/heap"
)

// heapItem 是堆中的元素，记录来自哪个迭代器.
type heapItem struct {
	key   []byte
	value []byte
	idx   int // 迭代器在 iters 中的下标，idx 越大数据越新
}

// mergeHeap 最小堆：先按 key 升序，key 相同时按 idx 降序（新数据优先弹出）.
type mergeHeap []*heapItem

func (h mergeHeap) Len() int { return len(h) }
func (h mergeHeap) Less(i, j int) bool {
	cmp := bytes.Compare(h[i].key, h[j].key)
	if cmp != 0 {
		return cmp < 0
	}
	// key 相同，idx 大的（更新的数据）优先
	return h[i].idx > h[j].idx
}
func (h mergeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *mergeHeap) Push(x any) { *h = append(*h, x.(*heapItem)) }
func (h *mergeHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return item
}

// MergeIterator 对多个有序迭代器做归并，相同 key 只保留最新值.
// iters 的顺序约定: 下标越大数据越新（后传入的覆盖先传入的）.
type MergeIterator struct {
	iters []Iterator
	h     mergeHeap

	key    []byte
	value  []byte
	err    error
	inited bool
}

func NewMergeIterator(iters []Iterator) *MergeIterator {
	return &MergeIterator{
		iters: iters,
	}
}

// init 将每个迭代器的第一个元素推入堆.
func (m *MergeIterator) init() {
	m.h = make(mergeHeap, 0, len(m.iters))
	for i, iter := range m.iters {
		if iter.Next() {
			heap.Push(&m.h, &heapItem{
				key:   iter.Key(),
				value: iter.Value(),
				idx:   i,
			})
		}
		if iter.Err() != nil {
			m.err = iter.Err()
			return
		}
	}
	m.inited = true
}

func (m *MergeIterator) Next() bool {
	if m.err != nil {
		return false
	}

	if !m.inited {
		m.init()
		if m.err != nil {
			return false
		}
	}

	for m.h.Len() > 0 {
		// 弹出当前最小 key（如果 key 相同则是最新的那个）
		top := heap.Pop(&m.h).(*heapItem)
		m.key = top.key
		m.value = top.value

		// 推进该迭代器
		iter := m.iters[top.idx]
		if iter.Next() {
			heap.Push(&m.h, &heapItem{
				key:   iter.Key(),
				value: iter.Value(),
				idx:   top.idx,
			})
		}
		if iter.Err() != nil {
			m.err = iter.Err()
			return false
		}

		// 跳过堆中所有相同 key 的旧版本
		for m.h.Len() > 0 && bytes.Equal(m.h[0].key, m.key) {
			dup := heap.Pop(&m.h).(*heapItem)
			dupIter := m.iters[dup.idx]
			if dupIter.Next() {
				heap.Push(&m.h, &heapItem{
					key:   dupIter.Key(),
					value: dupIter.Value(),
					idx:   dup.idx,
				})
			}
			if dupIter.Err() != nil {
				m.err = dupIter.Err()
				return false
			}
		}

		return true
	}

	return false
}

func (m *MergeIterator) Key() []byte   { return m.key }
func (m *MergeIterator) Value() []byte { return m.value }
func (m *MergeIterator) Err() error    { return m.err }
