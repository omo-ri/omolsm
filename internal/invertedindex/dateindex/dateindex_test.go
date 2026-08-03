package dateindex

import (
	"testing"
)

func TestRange_Basic(t *testing.T) {
	di := New()
	// 10 docs, day 0..9
	for i := 0; i < 10; i++ {
		di.Add(int32(i), uint32(i))
	}

	bm := di.Range(3, 6)
	got := bm.ToArray()
	if len(got) != 4 {
		t.Fatalf("expected 4 docs, got %d: %v", len(got), got)
	}
	for i, want := range []uint32{3, 4, 5, 6} {
		if got[i] != want {
			t.Fatalf("got[%d]=%d, want %d", i, got[i], want)
		}
	}
}

func TestRange_Empty(t *testing.T) {
	di := New()
	di.Add(10, 0)
	di.Add(20, 1)

	bm := di.Range(11, 19)
	if !bm.IsEmpty() {
		t.Fatalf("expected empty, got %v", bm.ToArray())
	}
}

func TestRange_SingleDay(t *testing.T) {
	di := New()
	di.Add(5, 10)
	di.Add(5, 20)
	di.Add(6, 30)

	bm := di.Range(5, 5)
	got := bm.ToArray()
	if len(got) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(got))
	}
}

func TestRange_EmptyIndex(t *testing.T) {
	di := New()
	bm := di.Range(0, 100)
	if !bm.IsEmpty() {
		t.Fatal("expected empty")
	}
}

func TestLeBitmap(t *testing.T) {
	di := New()
	for i := 0; i < 5; i++ {
		di.Add(int32(i*10), uint32(i))
	}
	// days: 0, 10, 20, 30, 40
	bm := di.LeBitmap(25)
	got := bm.ToArray()
	// docs 0(day0), 1(day10), 2(day20)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d: %v", len(got), got)
	}
}

func TestGeBitmap(t *testing.T) {
	di := New()
	for i := 0; i < 5; i++ {
		di.Add(int32(i*10), uint32(i))
	}
	// days: 0, 10, 20, 30, 40
	bm := di.GeBitmap(25)
	got := bm.ToArray()
	// docs 3(day30), 4(day40)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(got), got)
	}
}

func TestAddOutOfOrder(t *testing.T) {
	di := New()
	di.Add(30, 3)
	di.Add(10, 1)
	di.Add(20, 2)

	if di.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", di.Len())
	}

	// Should still find all in range
	bm := di.Range(10, 30)
	if bm.GetCardinality() != 3 {
		t.Fatalf("expected 3, got %d", bm.GetCardinality())
	}
}

func TestDayFromString(t *testing.T) {
	d, err := DayFromString("2024-01-01")
	if err != nil {
		t.Fatal(err)
	}
	if d <= 0 {
		t.Fatalf("expected positive day, got %d", d)
	}

	_, err = DayFromString("bad-date")
	if err == nil {
		t.Fatal("expected error for bad date")
	}
}
