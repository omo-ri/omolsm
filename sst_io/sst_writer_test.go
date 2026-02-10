package sst_io

import (
	"omolsm"
	"os"
	"testing"
)

func Test_SSTWriter(t *testing.T) {
	dir := "./lsm"
	err := os.RemoveAll(dir)
	if err != nil {
		return
	}

	conf, err := omolsm.NewConfig("./lsm", omolsm.WithSSTDataBlockSize(16)) // 16B 的 block 大小，方便测试
	if err != nil {
		t.Error(err)
		return
	}
	sstWriter, err := NewSSTWriter("test.sst", conf)
	if err != nil {
		t.Error(err)
		return
	}
	defer sstWriter.Close()

	sstWriter.Append([]byte("a"), []byte("b"))
	sstWriter.Append([]byte("ab"), []byte("cd"))
	sstWriter.Append([]byte("e"), []byte("f"))
	sstWriter.Append([]byte("ef"), []byte("gh"))

	// datablock1: record: [0 1 1 a b] [1 1 2 b cd] [0 1 1 e f]
	// datablock2: [1 1 2 ef gh]
	// filter: 0 -> bitmap1  16 -> bitmap2
	// index: [“” 0 0] [e 0 16] [ef 16 7]
	// footer: ...
	_, blockToFilter, index := sstWriter.Finish()
	if len(blockToFilter) != 2 {
		t.Errorf("unexpect filter len: %d", len(blockToFilter))
	}

	if _, ok := blockToFilter[0]; !ok {
		t.Error("miss filter key: 0")
	}

	if _, ok := blockToFilter[16]; !ok {
		t.Error("miss filter key: 19")
	}

	if len(index) != 3 {
		t.Errorf("unexpect index len: %d", len(index))
	}

	if string(index[0].Key) != "" || index[0].PrevBlockOffset != 0 || index[0].PrevBlockSize != 0 {
		t.Errorf("invalid index0: %+v, key: %s", index[0], index[0].Key)
	}

	if string(index[1].Key) != "e" || index[1].PrevBlockOffset != 0 || index[1].PrevBlockSize != 16 {
		t.Errorf("invalid index1: %+v", index[1])
	}

	if string(index[2].Key) != "ef" || index[2].PrevBlockOffset != 16 || index[2].PrevBlockSize != 7 {
		t.Errorf("invalid index2: %+v", index[2])
	}
}
