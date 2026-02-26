package dictionary

// Dictionary provides bidirectional mapping between terms and feature IDs.
type Dictionary interface {
	// GetOrAdd returns the feature_id for a term, creating a new mapping if not exists.
	GetOrAdd(term string) uint32

	// Get returns the feature_id for a term without creating a new mapping.
	Get(term string) (uint32, bool)

	// GetTerm returns the term for a given feature_id.
	GetTerm(id uint32) (string, bool)

	// GetOrAddTerms maps a batch of terms to feature_ids, creating new mappings as needed.
	GetOrAddTerms(terms []string) []uint32

	// GetTerms maps a batch of feature_ids back to terms.
	GetTerms(ids []uint32) []string

	// Size returns the number of terms in the dictionary.
	Size() int
}
