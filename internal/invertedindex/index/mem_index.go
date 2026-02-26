package index

import "github.com/RoaringBitmap/roaring"

// MemIndex is an in-memory inverted index backed by Roaring Bitmaps.
// Not thread-safe.
type MemIndex struct {
	postings map[uint32]*roaring.Bitmap // feature_id → doc IDs
	allDocs  *roaring.Bitmap            // all known doc IDs (for NOT operations)
}

func NewMemIndex() *MemIndex {
	return &MemIndex{
		postings: make(map[uint32]*roaring.Bitmap),
		allDocs:  roaring.New(),
	}
}

func (m *MemIndex) Add(docID uint32, featureIDs []uint32) {
	m.allDocs.Add(docID)
	for _, fid := range featureIDs {
		bm, ok := m.postings[fid]
		if !ok {
			bm = roaring.New()
			m.postings[fid] = bm
		}
		bm.Add(docID)
	}
}

func (m *MemIndex) GetPostingList(featureID uint32) *roaring.Bitmap {
	bm, ok := m.postings[featureID]
	if !ok {
		return roaring.New()
	}
	return bm.Clone()
}

func (m *MemIndex) And(featureIDs []uint32) *roaring.Bitmap {
	if len(featureIDs) == 0 {
		return roaring.New()
	}

	// Start with the smallest posting list for efficiency.
	result := m.getOrEmpty(featureIDs[0]).Clone()
	for _, fid := range featureIDs[1:] {
		result.And(m.getOrEmpty(fid))
		// Early exit: if result is already empty, no need to continue.
		if result.IsEmpty() {
			return result
		}
	}
	return result
}

func (m *MemIndex) DocCount() uint32 {
	return uint32(m.allDocs.GetCardinality())
}

func (m *MemIndex) FeatureCount() int {
	return len(m.postings)
}

// getOrEmpty returns the posting list for a feature_id, or an empty bitmap.
// Does NOT clone — callers that need to mutate must clone first.
func (m *MemIndex) getOrEmpty(featureID uint32) *roaring.Bitmap {
	bm, ok := m.postings[featureID]
	if !ok {
		return roaring.New()
	}
	return bm
}
