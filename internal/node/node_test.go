package node

import (
	"bytes"
	"omolsm/config"
	"omolsm/internal/sst_io/reader"
	writer "omolsm/internal/sst_io/writer"
	"testing"
)

func Test_Node_Get(t *testing.T) {
	conf, err := config.NewConfig("./lsm")
	if err != nil {
		t.Error(err)
		return
	}
	sstWriter, err := writer.NewSSTWriter("test_node_get.sst", conf)
	if err != nil {
		t.Error(err)
		return
	}
	defer sstWriter.Close()

	kvs := []*reader.KV{
		{
			Key:   []byte("a"),
			Value: []byte("b"),
		},
		{
			Key:   []byte("b"),
			Value: []byte("c"),
		},
		{
			Key:   []byte("c"),
			Value: []byte("d"),
		},
		{
			Key:   []byte("d"),
			Value: []byte("e"),
		},
	}
	for _, kv := range kvs {
		sstWriter.Append(kv.Key, kv.Value)
	}

	size, blockToFilter, index := sstWriter.Finish()
	sstReader, err := reader.NewSSTReader("test_node_get.sst", conf)
	if err != nil {
		t.Error(err)
		return
	}
	defer sstReader.Close()

	node := NewNode(conf, "test_node_get.sst", sstReader, 0, 0, size, blockToFilter, index)
	for _, kv := range kvs {
		v, ok, err := node.Get(kv.Key)
		if err != nil {
			t.Error(err)
			continue
		}
		if !ok {
			t.Errorf("get key: %s failed", kv.Key)
			continue
		}

		if !bytes.Equal(v, kv.Value) {
			t.Errorf("key: %s, expect v: %s, got: %v", kv.Key, kv.Value, v)
		}
	}

	_, ok, err := node.Get([]byte("e"))
	if err != nil {
		t.Error(err)
		return
	}
	if ok {
		t.Errorf("key: e, expect ok: %t, got: %t", false, true)
	}
}

func Test_Node_binarySearchIndex(t *testing.T) {
	tests := []struct {
		name             string
		index            []*writer.Index
		key              []byte
		expectIndexExist bool
		expectIndexKey   []byte
	}{
		{
			index: []*writer.Index{
				{
					Key: []byte("a"),
				},
				{
					Key: []byte("b"),
				},
			},
			key: []byte("c"),
		},
		{
			index: []*writer.Index{
				{
					Key: []byte("a"),
				},
				{
					Key: []byte("b"),
				},
				{
					Key: []byte("d"),
				},
			},
			key:              []byte("c"),
			expectIndexExist: true,
			expectIndexKey:   []byte("d"),
		},
		{
			index: []*writer.Index{
				{
					Key: []byte("b"),
				},
				{
					Key: []byte("d"),
				},
				{
					Key: []byte("e"),
				},
				{
					Key: []byte("f"),
				},
			},
			key:              []byte("a"),
			expectIndexExist: true,
			expectIndexKey:   []byte("b"),
		},
		{
			index: []*writer.Index{
				{
					Key: []byte("b"),
				},
				{
					Key: []byte("d"),
				},
				{
					Key: []byte("e"),
				},
				{
					Key: []byte("f"),
				},
			},
			key: []byte("g"),
		},
	}

	node := Node{}
	for _, test := range tests {
		if pass := t.Run(test.name, func(t *testing.T) {
			node.index = test.index
			index, ok := node.binarySearchIndex(test.key, 0, len(node.index)-1)
			if ok != test.expectIndexExist {
				t.Errorf("key: %s expect index exist: %t, got: %t", test.key, test.expectIndexExist, ok)
			}
			if !ok {
				return
			}
			if !bytes.Equal(index.Key, test.expectIndexKey) {
				t.Errorf("key: %s expect index key: %s, got: %s", test.key, test.expectIndexKey, index.Key)
			}
		}); !pass {
			t.Errorf("%s not pass", test.name)
		}
	}
}

func Test_Node_Destroy(t *testing.T) {
	conf, err := config.NewConfig("./lsm")
	if err != nil {
		t.Error(err)
		return
	}
	sstWriter, err := writer.NewSSTWriter("test_node_destroy.sst", conf)
	if err != nil {
		t.Error(err)
		return
	}
	defer sstWriter.Close()

	kvs := []*reader.KV{
		{
			Key:   []byte("a"),
			Value: []byte("b"),
		},
	}
	for _, kv := range kvs {
		sstWriter.Append(kv.Key, kv.Value)
	}

	size, blockToFilter, index := sstWriter.Finish()
	sstReader, err := reader.NewSSTReader("test_node_destroy.sst", conf)
	if err != nil {
		t.Error(err)
		return
	}
	sstWriter.Close()
	sstReader.Close()
	node := NewNode(conf, "test_node_destroy.sst", sstReader, 0, 0, size, blockToFilter, index)
	node.Destroy()
}
