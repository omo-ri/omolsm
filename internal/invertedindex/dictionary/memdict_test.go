package dictionary

import (
	"testing"
)

func TestGetOrAdd(t *testing.T) {
	d := NewMemDictionary()

	id1 := d.GetOrAdd("dog")
	id2 := d.GetOrAdd("cat")
	id3 := d.GetOrAdd("dog")

	if id1 != 0 {
		t.Errorf("first term should get id 0, got %d", id1)
	}
	if id2 != 1 {
		t.Errorf("second term should get id 1, got %d", id2)
	}
	if id3 != id1 {
		t.Errorf("duplicate term should return same id: got %d, want %d", id3, id1)
	}
	if d.Size() != 2 {
		t.Errorf("size should be 2, got %d", d.Size())
	}
}

func TestGet(t *testing.T) {
	d := NewMemDictionary()
	d.GetOrAdd("dog")

	if id, ok := d.Get("dog"); !ok || id != 0 {
		t.Errorf("Get(dog) = (%d, %v), want (0, true)", id, ok)
	}
	if _, ok := d.Get("missing"); ok {
		t.Error("Get(missing) should return false")
	}
}

func TestGetTerm(t *testing.T) {
	d := NewMemDictionary()
	d.GetOrAdd("dog")
	d.GetOrAdd("cat")

	if term, ok := d.GetTerm(0); !ok || term != "dog" {
		t.Errorf("GetTerm(0) = (%q, %v), want (dog, true)", term, ok)
	}
	if term, ok := d.GetTerm(1); !ok || term != "cat" {
		t.Errorf("GetTerm(1) = (%q, %v), want (cat, true)", term, ok)
	}
	if _, ok := d.GetTerm(999); ok {
		t.Error("GetTerm(999) should return false")
	}
}

func TestGetOrAddTerms(t *testing.T) {
	d := NewMemDictionary()

	ids := d.GetOrAddTerms([]string{"dog", "cat", "dog", "bird"})

	if len(ids) != 4 {
		t.Fatalf("expected 4 ids, got %d", len(ids))
	}
	if ids[0] != ids[2] {
		t.Errorf("duplicate 'dog' should have same id: %d vs %d", ids[0], ids[2])
	}
	if d.Size() != 3 {
		t.Errorf("size should be 3 (dog, cat, bird), got %d", d.Size())
	}
}

func TestGetTerms(t *testing.T) {
	d := NewMemDictionary()
	d.GetOrAdd("dog")
	d.GetOrAdd("cat")
	d.GetOrAdd("bird")

	terms := d.GetTerms([]uint32{2, 0, 1})
	want := []string{"bird", "dog", "cat"}

	for i, term := range terms {
		if term != want[i] {
			t.Errorf("GetTerms[%d] = %q, want %q", i, term, want[i])
		}
	}
}

func TestGetTermsWithInvalidID(t *testing.T) {
	d := NewMemDictionary()
	d.GetOrAdd("dog")

	terms := d.GetTerms([]uint32{0, 999})
	if terms[0] != "dog" {
		t.Errorf("expected 'dog', got %q", terms[0])
	}
	if terms[1] != "" {
		t.Errorf("expected empty string for invalid id, got %q", terms[1])
	}
}

func TestCyrillicTerms(t *testing.T) {
	d := NewMemDictionary()

	id1 := d.GetOrAdd("собак")
	id2 := d.GetOrAdd("быстр")
	id3 := d.GetOrAdd("собак")

	if id1 == id2 {
		t.Error("different terms should have different ids")
	}
	if id1 != id3 {
		t.Error("same term should return same id")
	}
}

func TestEmptyDictionary(t *testing.T) {
	d := NewMemDictionary()

	if d.Size() != 0 {
		t.Errorf("empty dict size should be 0, got %d", d.Size())
	}
	if _, ok := d.Get("anything"); ok {
		t.Error("empty dict should return false for any Get")
	}
	if _, ok := d.GetTerm(0); ok {
		t.Error("empty dict should return false for any GetTerm")
	}
}
