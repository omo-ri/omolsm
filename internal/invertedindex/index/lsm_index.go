package index

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"

	"omolsm/config"
	"omolsm/internal/lsm/iterator"
	"omolsm/internal/lsm/tree"

	"github.com/RoaringBitmap/roaring"
)

const (
	defaultBlockSize = 1024 // docID 分片大小
	compositeKeyLen  = 8    // featureID(4) + blockID(4)
)

// LSMIndex implements InvertedIndex with LSM-tree persistent storage.
//
// Key encoding: [featureID 4B big-endian][blockID 4B big-endian] = 8 bytes.
// Value: roaring.Bitmap serialised via ToBytes().
//
// 不再自己实现 LSM 逻辑, 而是复用 tree.Tree 并注入 BitmapMergeFactory.
type LSMIndex struct {
	tree      tree.LSMTree // 底层统一的 LSM Tree
	conf      *config.Config
	blockSize uint32
	docCount  uint32
}

// LSMIndexOption configures LSMIndex.
type LSMIndexOption func(*LSMIndex)

func WithBlockSize(size uint32) LSMIndexOption {
	return func(idx *LSMIndex) { idx.blockSize = size }
}

// NewLSMIndex creates a new LSM-backed inverted index with an existing config.
// 关键: config 必须已设置 MergeIteratorFactory 为 BitmapMergeFactory.
func NewLSMIndex(conf *config.Config, opts ...LSMIndexOption) (*LSMIndex, error) {
	t, err := tree.NewTree(conf)
	if err != nil {
		return nil, fmt.Errorf("create lsm tree: %w", err)
	}

	idx := &LSMIndex{
		tree:      t,
		conf:      conf,
		blockSize: defaultBlockSize,
	}
	for _, opt := range opts {
		opt(idx)
	}
	return idx, nil
}

// NewLSMIndexFromConfig creates an LSM-backed inverted index with a temp directory.
// 自动注入 BitmapMergeFactory, 调用方无需手动设置.
func NewLSMIndexFromConfig(dir string, idxOpts []LSMIndexOption, confOpts ...config.ConfigOption) (*LSMIndex, error) {
	confOpts = append(confOpts, config.WithMergeIteratorFactory(
		iterator.BitmapMergeFactory(),
	))
	conf, err := config.NewConfig(dir, confOpts...)
	if err != nil {
		return nil, fmt.Errorf("create config: %w", err)
	}
	return NewLSMIndex(conf, idxOpts...)
}

// ---------------------------------------------------------------------------
// InvertedIndex interface
// ---------------------------------------------------------------------------

// Add indexes a document: each featureID → docID mapping is written as
// (featureID, blockID) → roaring bitmap into the LSM tree.
func (idx *LSMIndex) Add(docID uint32, featureIDs []uint32) {
	blockID := docID / idx.blockSize

	for _, fid := range featureIDs {
		key := string(encodeCompositeKey(fid, blockID))

		// 读旧 bitmap, OR 合并新 docID, 写回
		bm := roaring.New()
		if oldVal, ok, _ := idx.tree.Get(key); ok && oldVal != nil {
			if _, err := bm.FromBuffer(oldVal); err != nil {
				log.Printf("LSMIndex Add: bitmap deserialize error: %v", err)
				bm = roaring.New()
			}
		}

		bm.Add(docID)

		val, err := bm.ToBytes()
		if err != nil {
			log.Printf("LSMIndex Add: bitmap serialize error: %v", err)
			continue
		}
		if err := idx.tree.Put(key, val); err != nil {
			log.Printf("LSMIndex Add: put error: %v", err)
		}
	}

	if docID >= idx.docCount {
		idx.docCount = docID + 1
	}
}

// GetPostingList returns the bitmap of doc IDs for a given featureID.
// Scans all (featureID, blockID=*) entries and OR-merges them.
func (idx *LSMIndex) GetPostingList(featureID uint32) *roaring.Bitmap {
	result := roaring.New()

	startKey := string(encodeCompositeKey(featureID, 0))
	endKey := string(encodeCompositeKey(featureID+1, 0))

	kvs, err := idx.tree.Scan(startKey, endKey)
	if err != nil {
		log.Printf("LSMIndex GetPostingList: scan error: %v", err)
		return result
	}

	for _, kv := range kvs {
		bm := roaring.New()
		if _, err := bm.FromBuffer(kv.Value); err != nil {
			log.Printf("LSMIndex GetPostingList: bitmap deserialize error: %v", err)
			continue
		}
		result.Or(bm)
	}

	return result
}

// And returns doc IDs that contain ALL the given featureIDs.
func (idx *LSMIndex) And(featureIDs []uint32) *roaring.Bitmap {
	if len(featureIDs) == 0 {
		return roaring.New()
	}

	result := idx.GetPostingList(featureIDs[0])
	for i := 1; i < len(featureIDs); i++ {
		if result.IsEmpty() {
			break
		}
		result.And(idx.GetPostingList(featureIDs[i]))
	}
	return result
}

func (idx *LSMIndex) DocCount() uint32  { return idx.docCount }
func (idx *LSMIndex) FeatureCount() int { return 0 } // 需要全扫描, 近似值

// GetTreeStats returns the underlying LSM tree stats for observability.
func (idx *LSMIndex) GetTreeStats() tree.Stats {
	if t, ok := idx.tree.(*tree.Tree); ok {
		return t.GetStats()
	}
	return tree.Stats{}
}

// Close releases all resources.
func (idx *LSMIndex) Close() {
	idx.tree.Close()
	if idx.conf.Dir != "" {
		_ = os.RemoveAll(idx.conf.Dir)
	}
}

// ---------------------------------------------------------------------------
// Key encoding
// ---------------------------------------------------------------------------

func encodeCompositeKey(featureID, blockID uint32) []byte {
	key := make([]byte, compositeKeyLen)
	binary.BigEndian.PutUint32(key[0:4], featureID)
	binary.BigEndian.PutUint32(key[4:8], blockID)
	return key
}

func decodeCompositeKey(key []byte) (featureID, blockID uint32) {
	featureID = binary.BigEndian.Uint32(key[0:4])
	blockID = binary.BigEndian.Uint32(key[4:8])
	return
}

// SSTPerLevel 返回底层 LSM Tree 每层的 SST 数量.
func (idx *LSMIndex) SSTPerLevel() []int {
	if t, ok := idx.tree.(*tree.Tree); ok {
		return t.SSTPerLevel()
	}
	return nil
}
