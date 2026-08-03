package dateindex

import (
	"sort"

	"github.com/RoaringBitmap/roaring"
)

// DateEntry pairs a day (days since Unix epoch) with a document ID.
type DateEntry struct {
	Day   int32
	DocID uint32
}

// DateIndex is a sorted slice of DateEntry (sorted by Day).
// Range queries use binary search: O(log N) to locate boundaries, O(K) to build bitmap.
type DateIndex struct {
	entries []DateEntry
}

// New creates an empty DateIndex.
func New() *DateIndex {
	return &DateIndex{}
}

// Add inserts a (day, docID) pair, maintaining sort order by day.
func (di *DateIndex) Add(day int32, docID uint32) {
	e := DateEntry{Day: day, DocID: docID}
	i := sort.Search(len(di.entries), func(j int) bool {
		return di.entries[j].Day >= day
	})
	di.entries = append(di.entries, DateEntry{})
	copy(di.entries[i+1:], di.entries[i:])
	di.entries[i] = e
}

// Len returns the number of entries.
func (di *DateIndex) Len() int { return len(di.entries) }

// Range returns a bitmap of docIDs whose day is in [from, to].
func (di *DateIndex) Range(from, to int32) *roaring.Bitmap {
	bm := roaring.New()
	if len(di.entries) == 0 {
		return bm
	}
	lo := sort.Search(len(di.entries), func(i int) bool {
		return di.entries[i].Day >= from
	})
	for i := lo; i < len(di.entries) && di.entries[i].Day <= to; i++ {
		bm.Add(di.entries[i].DocID)
	}
	return bm
}

// LeBitmap returns a bitmap of docIDs whose day <= d.
func (di *DateIndex) LeBitmap(d int32) *roaring.Bitmap {
	bm := roaring.New()
	for i := 0; i < len(di.entries) && di.entries[i].Day <= d; i++ {
		bm.Add(di.entries[i].DocID)
	}
	return bm
}

// GeBitmap returns a bitmap of docIDs whose day >= d.
func (di *DateIndex) GeBitmap(d int32) *roaring.Bitmap {
	bm := roaring.New()
	if len(di.entries) == 0 {
		return bm
	}
	lo := sort.Search(len(di.entries), func(i int) bool {
		return di.entries[i].Day >= d
	})
	for i := lo; i < len(di.entries); i++ {
		bm.Add(di.entries[i].DocID)
	}
	return bm
}
