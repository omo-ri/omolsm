package kgram

// Index maps k-grams to the set of term featureIDs that contain them,
// enabling wildcard query resolution.
type Index interface {
	// AddTerm registers a term and its featureID into the k-gram index.
	// Must be called for every term during document indexing.
	AddTerm(featureID uint32, term string)

	// Search returns featureIDs of indexed terms that match the wildcard pattern.
	// Pattern may contain '*' as wildcard (e.g. "he*o", "*ello", "fox*").
	// Returns nil if no terms match.
	Search(pattern string) []uint32
}
