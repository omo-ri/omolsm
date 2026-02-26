package index

import (
	"github.com/RoaringBitmap/roaring"
	"testing"
)

func toSorted(bm *roaring.Bitmap) []uint32 {
	return bm.ToArray() // Roaring 本身就返回有序数组
}

func TestAddAndGetPostingList(t *testing.T) {
	idx := NewMemIndex()

	// doc 0 contains features [0, 1, 2]
	// doc 1 contains features [1, 2, 3]
	// doc 2 contains features [0, 3]
	idx.Add(0, []uint32{0, 1, 2})
	idx.Add(1, []uint32{1, 2, 3})
	idx.Add(2, []uint32{0, 3})

	tests := []struct {
		featureID uint32
		want      []uint32
	}{
		{0, []uint32{0, 2}},
		{1, []uint32{0, 1}},
		{2, []uint32{0, 1}},
		{3, []uint32{1, 2}},
		{999, nil}, // non-existent feature
	}

	for _, tt := range tests {
		got := toSorted(idx.GetPostingList(tt.featureID))
		if !sliceEqual(got, tt.want) {
			t.Errorf("GetPostingList(%d) = %v, want %v", tt.featureID, got, tt.want)
		}
	}
}

func TestAnd(t *testing.T) {
	idx := NewMemIndex()
	idx.Add(0, []uint32{0, 1})    // doc 0: features 0, 1
	idx.Add(1, []uint32{0, 1, 2}) // doc 1: features 0, 1, 2
	idx.Add(2, []uint32{0, 2})    // doc 2: features 0, 2

	tests := []struct {
		name       string
		featureIDs []uint32
		want       []uint32
	}{
		{"single feature", []uint32{0}, []uint32{0, 1, 2}},
		{"AND two features", []uint32{0, 1}, []uint32{0, 1}},
		{"AND three features", []uint32{0, 1, 2}, []uint32{1}},
		{"AND with non-existent", []uint32{0, 999}, nil},
		{"empty input", []uint32{}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toSorted(idx.And(tt.featureIDs))
			if !sliceEqual(got, tt.want) {
				t.Errorf("And(%v) = %v, want %v", tt.featureIDs, got, tt.want)
			}
		})
	}
}

func TestDocCount(t *testing.T) {
	idx := NewMemIndex()

	if idx.DocCount() != 0 {
		t.Errorf("empty index DocCount = %d, want 0", idx.DocCount())
	}

	idx.Add(0, []uint32{0})
	idx.Add(1, []uint32{0})
	idx.Add(1, []uint32{1}) // duplicate doc, different feature

	if idx.DocCount() != 2 {
		t.Errorf("DocCount = %d, want 2", idx.DocCount())
	}
}

func TestFeatureCount(t *testing.T) {
	idx := NewMemIndex()

	idx.Add(0, []uint32{0, 1, 2})
	idx.Add(1, []uint32{2, 3})

	if idx.FeatureCount() != 4 {
		t.Errorf("FeatureCount = %d, want 4", idx.FeatureCount())
	}
}

func TestGetPostingListIsolation(t *testing.T) {
	idx := NewMemIndex()
	idx.Add(0, []uint32{0})

	// Modifying the returned bitmap should NOT affect the index.
	bm := idx.GetPostingList(0)
	bm.Add(999)

	original := idx.GetPostingList(0)
	if original.Contains(999) {
		t.Error("modifying returned bitmap should not affect internal state")
	}
}

func TestAndEarlyExit(t *testing.T) {
	idx := NewMemIndex()
	idx.Add(0, []uint32{0})
	idx.Add(1, []uint32{1})

	// feature 0 and feature 1 have no overlap → should return empty fast.
	result := idx.And([]uint32{0, 1})
	if !result.IsEmpty() {
		t.Errorf("expected empty result, got %v", result.ToArray())
	}
}

// sliceEqual compares two uint32 slices, treating nil and empty as equal.
func sliceEqual(a, b []uint32) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
