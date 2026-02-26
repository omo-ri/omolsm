package dictionary

import (
	"encoding/binary"
	"fmt"
	"os"

	"omolsm/config"
	"omolsm/internal/lsm/tree"
)

// LSMDictionary implements Dictionary with an LSM tree backend.
// Key encoding: term (string) → value: featureID (4 bytes big-endian).
type LSMDictionary struct {
	tree   tree.LSMTree
	dir    string // temp directory for SST files
	nextID uint32
}

func NewLSMDictionary(conf *config.Config) (*LSMDictionary, error) {
	lsmTree, err := tree.NewTree(conf)
	if err != nil {
		return nil, fmt.Errorf("create tree: %w", err)
	}
	return &LSMDictionary{
		tree: lsmTree,
		dir:  conf.Dir,
	}, nil
}

// NewLSMDictionaryFromConfig creates an LSM-backed dictionary.
// dir is the directory for SST files (auto-created, cleaned up on Close).
func NewLSMDictionaryFromConfig(dir string, opts ...config.ConfigOption) (*LSMDictionary, error) {
	conf, err := config.NewConfig(dir, opts...)
	if err != nil {
		return nil, fmt.Errorf("create config: %w", err)
	}

	lsmTree, err := tree.NewTree(conf)
	if err != nil {
		return nil, fmt.Errorf("create tree: %w", err)
	}

	return &LSMDictionary{
		tree: lsmTree,
		dir:  dir,
	}, nil
}

// GetOrAdd returns the featureID for a term, creating a new mapping if it does not exist.
func (d *LSMDictionary) GetOrAdd(term string) uint32 {
	if id, ok := d.get(term); ok {
		return id
	}

	id := d.nextID
	d.nextID++
	d.put(term, id)
	return id
}

// Get returns the featureID for a term without creating a new mapping.
func (d *LSMDictionary) Get(term string) (uint32, bool) {
	return d.get(term)
}

// GetOrAddTerms maps a batch of terms to featureIDs, creating new mappings as needed.
func (d *LSMDictionary) GetOrAddTerms(terms []string) []uint32 {
	ids := make([]uint32, len(terms))
	for i, term := range terms {
		if id, ok := d.get(term); ok {
			ids[i] = id
			continue
		}

		id := d.nextID
		d.nextID++
		d.put(term, id)
		ids[i] = id
	}

	return ids
}

// Size returns the number of terms in the dictionary.
func (d *LSMDictionary) Size() int {
	return int(d.nextID)
}

// GetTreeStats returns the underlying LSM tree's I/O stats.
func (d *LSMDictionary) GetTreeStats() tree.Stats {
	return d.tree.GetStats()
}

// SSTPerLevel 返回底层 LSM Tree 每层的 SST 数量.
func (d *LSMDictionary) SSTPerLevel() []int {
	if t, ok := d.tree.(*tree.Tree); ok {
		return t.SSTPerLevel()
	}
	return nil
}

// Close releases the LSM tree and removes the temp directory.
func (d *LSMDictionary) Close() {
	d.tree.Close()
	if d.dir != "" {
		_ = os.RemoveAll(d.dir)
	}
}

// --- internal helpers ---

func (d *LSMDictionary) get(term string) (uint32, bool) {
	val, ok, err := d.tree.Get(term)
	if err != nil || !ok || len(val) < 4 {
		return 0, false
	}
	return binary.BigEndian.Uint32(val), true
}

func (d *LSMDictionary) put(term string, id uint32) {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], id)
	_ = d.tree.Put(term, buf[:])
}
