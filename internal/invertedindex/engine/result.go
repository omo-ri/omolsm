package engine

import "github.com/RoaringBitmap/roaring"

// Result wraps a Roaring Bitmap and supports chainable boolean operations.
// Each method returns a new Result, never mutates the original.
type Result struct {
	bm     *roaring.Bitmap
	engine *Engine
}

func newResult(bm *roaring.Bitmap, e *Engine) *Result {
	return &Result{bm: bm, engine: e}
}

// --- String-based: covers 99% of use cases ---

// And returns docs matching this AND all given terms.
func (r *Result) And(terms ...string) *Result {
	return r.AndResult(r.engine.Search(terms...))
}

// Or returns docs matching this OR any of the given terms.
func (r *Result) Or(terms ...string) *Result {
	bm := r.bm.Clone()
	for _, term := range terms {
		bm.Or(r.engine.Search(term).bm)
	}
	return newResult(bm, r.engine)
}

// Not returns docs matching this but NOT any of the given terms.
func (r *Result) Not(terms ...string) *Result {
	bm := r.bm.Clone()
	for _, term := range terms {
		bm.AndNot(r.engine.Search(term).bm)
	}
	return newResult(bm, r.engine)
}

// --- Result-based: for nested / complex queries ---

// AndResult returns docs present in both this and other.
func (r *Result) AndResult(other *Result) *Result {
	bm := r.bm.Clone()
	bm.And(other.bm)
	return newResult(bm, r.engine)
}

// OrResult returns docs present in this or other.
func (r *Result) OrResult(other *Result) *Result {
	bm := r.bm.Clone()
	bm.Or(other.bm)
	return newResult(bm, r.engine)
}

// NotResult returns docs in this but not in other.
func (r *Result) NotResult(other *Result) *Result {
	bm := r.bm.Clone()
	bm.AndNot(other.bm)
	return newResult(bm, r.engine)
}

// --- Accessors ---

// IsEmpty returns true if no documents match.
func (r *Result) IsEmpty() bool {
	return r.bm.IsEmpty()
}

// Count returns the number of matching documents.
func (r *Result) Count() uint64 {
	return r.bm.GetCardinality()
}

// DocIDs returns the raw matching document IDs.
func (r *Result) DocIDs() []uint32 {
	return r.bm.ToArray()
}

// Documents returns DocInfo for all matching documents.
func (r *Result) Documents() []DocInfo {
	return r.engine.bitmapToDocs(r.bm)
}
