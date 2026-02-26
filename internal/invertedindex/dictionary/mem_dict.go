package dictionary

// MemDictionary is an in-memory implementation of Dictionary.
// Forward lookup: map[string]uint32 (term → id).
// Reverse lookup: []string (id → term, index is feature_id).
// Not thread-safe.
type MemDictionary struct {
	termToID map[string]uint32
	idToTerm []string
	nextID   uint32
}

func NewMemDictionary() *MemDictionary {
	return &MemDictionary{
		termToID: make(map[string]uint32),
	}
}

func (d *MemDictionary) GetOrAdd(term string) uint32 {
	if id, ok := d.termToID[term]; ok {
		return id
	}

	id := d.nextID
	d.termToID[term] = id
	d.idToTerm = append(d.idToTerm, term)
	d.nextID++
	return id
}

func (d *MemDictionary) Get(term string) (uint32, bool) {
	id, ok := d.termToID[term]
	return id, ok
}

func (d *MemDictionary) GetOrAddTerms(terms []string) []uint32 {
	ids := make([]uint32, len(terms))
	for i, term := range terms {
		ids[i] = d.GetOrAdd(term)
	}
	return ids
}

func (d *MemDictionary) Size() int {
	return len(d.termToID)
}
