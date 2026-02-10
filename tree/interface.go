package tree

type LSMTree interface {
	Put(key string, value []byte) error
	Get(key string) ([]byte, bool, error)
	Delete(key string) error
	Scan(startKey, endKey string) ([]*KVResult, error)
	Close()
}

// KVResult 范围查询返回的结果
type KVResult struct {
	Key   string
	Value []byte
}
