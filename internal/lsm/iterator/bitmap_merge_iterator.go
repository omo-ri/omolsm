package iterator

import (
	"bytes"
	"container/heap"

	"github.com/RoaringBitmap/roaring"
)

// BitmapMergeIterator merges multiple sorted iterators whose values are
// serialised roaring bitmaps. When the same key appears in more than one
// iterator, the bitmaps are OR-unioned (instead of last-write-wins).
//
// It implements the Iterator interface and is intended for inverted-index
// compaction where posting lists must be merged, not overwritten.
type BitmapMergeIterator struct {
	iters []Iterator
	h     mergeHeap // reuse the same heap type from merge_iterator.go

	key    []byte
	value  []byte
	err    error
	inited bool
}

func NewBitmapMergeIterator(iters []Iterator) *BitmapMergeIterator {
	return &BitmapMergeIterator{
		iters: iters,
	}
}

// init pushes the first element of every sub-iterator into the heap.
func (m *BitmapMergeIterator) init() {
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

func (m *BitmapMergeIterator) Next() bool {
	if m.err != nil {
		return false
	}

	if !m.inited {
		m.init()
		if m.err != nil {
			return false
		}
	}

	if m.h.Len() == 0 {
		return false
	}

	// Pop the smallest key.
	top := heap.Pop(&m.h).(*heapItem)
	curKey := top.key

	// Advance that sub-iterator.
	if !m.advanceIterator(top.idx) {
		return false
	}

	// Collect all values sharing the same key and OR-merge them.
	merged := m.deserializeBitmap(top.value)
	if m.err != nil {
		return false
	}

	for m.h.Len() > 0 && bytes.Equal(m.h[0].key, curKey) {
		dup := heap.Pop(&m.h).(*heapItem)

		bm := m.deserializeBitmap(dup.value)
		if m.err != nil {
			return false
		}
		merged.Or(bm)

		if !m.advanceIterator(dup.idx) {
			return false
		}
	}

	// Serialize the merged bitmap as the output value.
	mergedBytes, err := merged.ToBytes()
	if err != nil {
		m.err = err
		return false
	}

	m.key = curKey
	m.value = mergedBytes
	return true
}

func (m *BitmapMergeIterator) Key() []byte   { return m.key }
func (m *BitmapMergeIterator) Value() []byte { return m.value }
func (m *BitmapMergeIterator) Err() error    { return m.err }

// advanceIterator pushes the next element from iters[idx] into the heap.
// Returns false only if a read error occurred (sets m.err).
func (m *BitmapMergeIterator) advanceIterator(idx int) bool {
	iter := m.iters[idx]
	if iter.Next() {
		heap.Push(&m.h, &heapItem{
			key:   iter.Key(),
			value: iter.Value(),
			idx:   idx,
		})
	}
	if iter.Err() != nil {
		m.err = iter.Err()
		return false
	}
	return true
}

// deserializeBitmap reads a roaring bitmap from raw bytes.
func (m *BitmapMergeIterator) deserializeBitmap(data []byte) *roaring.Bitmap {
	bm := roaring.New()
	if len(data) == 0 {
		return bm
	}
	if _, err := bm.FromBuffer(data); err != nil {
		m.err = err
	}
	return bm
}
