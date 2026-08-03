package kgram

import (
	"path/filepath"

	"omolsm/config"
	"omolsm/internal/invertedindex/dictionary"
	"omolsm/internal/invertedindex/index"

	"github.com/RoaringBitmap/roaring"
)

// LSMKgramIndex is an LSM-backed implementation of Index.
//
// Internally it reuses two existing components:
//   - dictionary.LSMDictionary: maps kgram string → kgramID (uint32)
//   - index.LSMIndex:           maps kgramID → bitmap<featureID>
//
// termByID is kept in memory for post-filtering (pattern matching).
// Not thread-safe.
type LSMKgramIndex struct {
	k         int
	kgramDict *dictionary.LSMDictionary
	kgramIdx  *index.LSMIndex
	termByID  map[uint32]string // featureID → term (for post-filtering)
}

// NewLSMKgramIndex creates an LSM-backed k-gram index.
// dir is the base directory; two sub-dirs (dict/, idx/) are created inside it.
func NewLSMKgramIndex(dir string, k int, opts ...config.ConfigOption) (*LSMKgramIndex, error) {
	dictDir := filepath.Join(dir, "dict")
	idxDir := filepath.Join(dir, "idx")

	kgramDict, err := dictionary.NewLSMDictionaryFromConfig(dictDir, opts...)
	if err != nil {
		return nil, err
	}

	kgramIdx, err := index.NewLSMIndexFromConfig(idxDir, nil, opts...)
	if err != nil {
		kgramDict.Close()
		return nil, err
	}

	return &LSMKgramIndex{
		k:         k,
		kgramDict: kgramDict,
		kgramIdx:  kgramIdx,
		termByID:  make(map[uint32]string),
	}, nil
}

// AddTerm registers a term and its featureID into the k-gram index.
func (l *LSMKgramIndex) AddTerm(featureID uint32, term string) {
	if _, exists := l.termByID[featureID]; exists {
		return
	}
	l.termByID[featureID] = term

	grams := Generate(term, l.k)
	kgramIDs := make([]uint32, len(grams))
	for i, gram := range grams {
		kgramIDs[i] = l.kgramDict.GetOrAdd(gram)
	}
	// Reuse LSMIndex: "docID" = featureID, "featureIDs" = kgramIDs.
	l.kgramIdx.Add(featureID, kgramIDs)
}

// Search returns featureIDs of terms matching the wildcard pattern.
func (l *LSMKgramIndex) Search(pattern string) []uint32 {
	grams := ExtractFromPattern(pattern, l.k)
	if len(grams) == 0 {
		return nil
	}

	var result *roaring.Bitmap
	for _, gram := range grams {
		kgramID, ok := l.kgramDict.Get(gram)
		if !ok {
			return nil
		}
		// GetPostingList returns bitmap<featureID> for this kgramID.
		bm := l.kgramIdx.GetPostingList(kgramID)
		if bm.IsEmpty() {
			return nil
		}
		if result == nil {
			result = bm
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
		if term, ok := l.termByID[fid]; ok && MatchPattern(term, pattern) {
			matched = append(matched, fid)
		}
	}
	return matched
}

// Close releases LSM resources.
func (l *LSMKgramIndex) Close() {
	l.kgramDict.Close()
	l.kgramIdx.Close()
}

var _ Index = (*LSMKgramIndex)(nil)
