package kgram

import "github.com/RoaringBitmap/roaring"

// MemKgramIndex is an in-memory implementation of Index.
// Not thread-safe.
type MemKgramIndex struct {
	k        int
	postings map[string]*roaring.Bitmap // kgram → bitmap<featureID>
	termByID map[uint32]string          // featureID → original term (for post-filtering)
}

// NewMemKgramIndex creates an in-memory k-gram index with the given k value.
func NewMemKgramIndex(k int) *MemKgramIndex {
	return &MemKgramIndex{
		k:        k,
		postings: make(map[string]*roaring.Bitmap),
		termByID: make(map[uint32]string),
	}
}

// AddTerm registers a term and its featureID into the k-gram index.
func (m *MemKgramIndex) AddTerm(featureID uint32, term string) {
	if _, exists := m.termByID[featureID]; exists {
		return // already indexed
	}
	m.termByID[featureID] = term
	for _, gram := range Generate(term, m.k) {
		bm, ok := m.postings[gram]
		if !ok {
			bm = roaring.New()
			m.postings[gram] = bm
		}
		bm.Add(featureID)
	}
}

// Search returns featureIDs of terms matching the wildcard pattern.
func (m *MemKgramIndex) Search(pattern string) []uint32 {
	grams := ExtractFromPattern(pattern, m.k)
	if len(grams) == 0 {
		return nil
	}

	var result *roaring.Bitmap
	for _, gram := range grams {
		bm, ok := m.postings[gram]
		if !ok {
			return nil
		}
		if result == nil {
			result = bm.Clone()
		} else {
			result.And(bm)
		}
		if result.IsEmpty() {
			return nil
		}
	}
	if result == nil {
		return nil
	}

	// Post-filter: verify each candidate actually matches the pattern.
	var matched []uint32
	for _, fid := range result.ToArray() {
		if term, ok := m.termByID[fid]; ok && MatchPattern(term, pattern) {
			matched = append(matched, fid)
		}
	}
	return matched
}

var _ Index = (*MemKgramIndex)(nil)
