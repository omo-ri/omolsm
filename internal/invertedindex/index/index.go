package index

import "github.com/RoaringBitmap/roaring"

// InvertedIndex maps feature_id → set of doc IDs (as Roaring Bitmap).
type InvertedIndex interface {
	// Add indexes a document: associates all given feature_ids with the docID.
	Add(docID uint32, featureIDs []uint32)

	// GetPostingList returns the bitmap of doc IDs for a given feature_id.
	// Returns an empty bitmap if the feature_id is not in the index.
	GetPostingList(featureID uint32) *roaring.Bitmap

	// And returns doc IDs that contain ALL the given feature_ids.
	And(featureIDs []uint32) *roaring.Bitmap

	// DocCount returns the total number of distinct documents in the index.
	DocCount() uint32

	// FeatureCount returns the number of distinct features (terms) in the index.
	FeatureCount() int
}
