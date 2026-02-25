package iterator

// Iterator 是所有迭代器的统一接口.
// 调用方式: for iter.Next() { k, v := iter.Key(), iter.Value() }
type Iterator interface {
	// Next 推进到下一个 KV. 首次调用指向第一个元素.
	// 返回 false 表示遍历结束或出错.
	Next() bool
	Key() []byte
	Value() []byte
	Err() error
}
