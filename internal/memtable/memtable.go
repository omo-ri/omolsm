package memtable

// MemTableConstructor memtable 构造器
type MemTableConstructor func() MemTable

// MemTable 有序表 interface
type MemTable interface {
	Put(key string, value []byte)  // 写入数据
	Get(key string) ([]byte, bool) // 读取数据，第二个 bool flag 标识数据是否存在
	All() []*KV                    // 返回所有的 kv 对数据
	Size() int                     // 有序表内数据大小，单位 byte
	KvsCnt() int                   // kv 对数量
}

// KV 键值对
type KV struct {
	Key   string
	Value []byte
}
