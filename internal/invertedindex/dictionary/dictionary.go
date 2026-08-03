package dictionary

// Dictionary provides bidirectional mapping between terms and feature IDs.
type Dictionary interface {
	// GetOrAdd returns the feature_id for a term, creating a new mapping if not exists.
	GetOrAdd(term string) uint32

	// Get returns the feature_id for a term without creating a new mapping.
	Get(term string) (uint32, bool)

	// GetOrAddTerms maps a batch of terms to feature_ids, creating new mappings as needed.
	GetOrAddTerms(terms []string) []uint32

	// ScanPrefix returns featureIDs for all terms that start with the given prefix.
	ScanPrefix(prefix string) ([]uint32, error)

	// Size returns the number of terms in the dictionary.
	Size() int
}
