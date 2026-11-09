package document

import (
	"fmt"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestManagerGetMissing(t *testing.T) {
	m := NewManager()
	doc, ok := m.Get("missing")
	if ok || doc != nil {
		t.Fatalf("Get(missing) = (%v, %v), want (nil, false)", doc, ok)
	}
}

func TestManagerGetOrCreateReturnsSameDocument(t *testing.T) {
	m := NewManager()
	first := m.GetOrCreate("a", "x")
	second := m.GetOrCreate("a", "ignored")
	if first != second {
		t.Fatal("GetOrCreate returned different documents for the same ID")
	}
	assertState(t, first, "x", 0)

	got, ok := m.Get("a")
	if !ok || got != first {
		t.Fatalf("Get(a) = (%p, %v), want (%p, true)", got, ok, first)
	}
}

func TestManagerSeparateIDs(t *testing.T) {
	m := NewManager()
	a := m.GetOrCreate("a", "A")
	b := m.GetOrCreate("b", "B")
	if a == b {
		t.Fatal("different IDs returned the same document")
	}
	assertState(t, a, "A", 0)
	assertState(t, b, "B", 0)
}

func TestManagerConcurrentGetOrCreateAndSubmit(t *testing.T) {
	const n = 100
	m := NewManager()

	docs := make([]*Document, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			doc := m.GetOrCreate("doc", "")
			docs[i] = doc
			_, errs[i] = doc.Submit(fmt.Sprintf("client-%d", i), "op-1", 0, insertAt(0, 0, "x"))
		}()
	}
	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Errorf("goroutine %d: Submit error: %v", i, errs[i])
		}
		if docs[i] != docs[0] {
			t.Errorf("goroutine %d got a different document", i)
		}
	}

	content, rev := docs[0].Snapshot()
	if rev != n {
		t.Errorf("revision = %d, want %d", rev, n)
	}
	if got := utf8.RuneCountInString(content); got != n {
		t.Errorf("content length = %d runes, want %d", got, n)
	}
}
