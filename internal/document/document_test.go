package document

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/aden-cs/parity-ot/internal/ot"
)

// insertAt builds an op that inserts text at pos in a document of docLen runes.
func insertAt(docLen, pos uint32, text string) ot.Operation {
	var comps []ot.Component
	if pos > 0 {
		comps = append(comps, ot.Retain(pos))
	}
	comps = append(comps, ot.Insert(text))
	if rest := docLen - pos; rest > 0 {
		comps = append(comps, ot.Retain(rest))
	}
	return ot.Operation{
		Components:   comps,
		BaseLength:   docLen,
		TargetLength: docLen + uint32(utf8.RuneCountInString(text)),
	}
}

// deleteAt builds an op that deletes n runes at pos in a document of docLen runes.
func deleteAt(docLen, pos, n uint32) ot.Operation {
	var comps []ot.Component
	if pos > 0 {
		comps = append(comps, ot.Retain(pos))
	}
	comps = append(comps, ot.Delete(n))
	if rest := docLen - pos - n; rest > 0 {
		comps = append(comps, ot.Retain(rest))
	}
	return ot.Operation{
		Components:   comps,
		BaseLength:   docLen,
		TargetLength: docLen - n,
	}
}

func mustSubmit(t *testing.T, d *Document, clientID, opID string, base uint64, op ot.Operation) CommittedOperation {
	t.Helper()
	c, err := d.Submit(clientID, opID, base, op)
	if err != nil {
		t.Fatalf("Submit(%s, %s, base %d): unexpected error: %v", clientID, opID, base, err)
	}
	return c
}

func assertState(t *testing.T, d *Document, wantContent string, wantRevision uint64) {
	t.Helper()
	content, rev := d.Snapshot()
	if content != wantContent {
		t.Errorf("content = %q, want %q", content, wantContent)
	}
	if rev != wantRevision {
		t.Errorf("revision = %d, want %d", rev, wantRevision)
	}
	if got := uint64(len(d.history)); got != rev {
		t.Errorf("len(history) = %d, want it to equal revision %d", got, rev)
	}
}

func TestNew(t *testing.T) {
	assertState(t, New("hello"), "hello", 0)
}

func TestSubmitAtCurrentRevision(t *testing.T) {
	d := New("hello")

	c := mustSubmit(t, d, "a", "1", 0, insertAt(5, 5, " world"))
	if c.Revision != 1 || c.ClientID != "a" || c.OperationID != "1" {
		t.Errorf("committed = %+v, want Revision 1, ClientID a, OperationID 1", c)
	}
	assertState(t, d, "hello world", 1)

	mustSubmit(t, d, "a", "2", 1, deleteAt(11, 0, 6))
	assertState(t, d, "world", 2)
}

func TestSubmitConcurrentEditsAtSameBase(t *testing.T) {
	tests := []struct {
		name    string
		initial string
		opA     ot.Operation // committed first
		opB     ot.Operation // arrives second, same base
		want    string
	}{
		{
			name:    "inserts at different positions",
			initial: "abc",
			opA:     insertAt(3, 0, "X"),
			opB:     insertAt(3, 3, "Y"),
			want:    "XabcY",
		},
		{
			// Transform puts its first argument's text first, and Submit
			// passes the committed op first, so the earlier commit wins ties.
			name:    "inserts at same position: earlier commit goes first",
			initial: "abc",
			opA:     insertAt(3, 1, "A"),
			opB:     insertAt(3, 1, "B"),
			want:    "aABbc",
		},
		{
			name:    "insert inside a concurrently deleted range",
			initial: "abcdef",
			opA:     deleteAt(6, 1, 4),
			opB:     insertAt(6, 3, "X"),
			want:    "aXf",
		},
		{
			name:    "overlapping deletes",
			initial: "abcdef",
			opA:     deleteAt(6, 1, 2), // "bc"
			opB:     deleteAt(6, 2, 2), // "cd"
			want:    "aef",
		},
		{
			name:    "identical deletes",
			initial: "abcdef",
			opA:     deleteAt(6, 2, 2),
			opB:     deleteAt(6, 2, 2),
			want:    "abef",
		},
		{
			name:    "multi-byte runes",
			initial: "héllo",
			opA:     deleteAt(5, 1, 1),
			opB:     insertAt(5, 5, "ö"),
			want:    "hlloö",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New(tt.initial)
			mustSubmit(t, d, "a", "1", 0, tt.opA)
			c := mustSubmit(t, d, "b", "1", 0, tt.opB)
			if c.Revision != 2 {
				t.Errorf("second commit Revision = %d, want 2", c.Revision)
			}
			assertState(t, d, tt.want, 2)
		})
	}
}

func TestSubmitTransformsAcrossSeveralRevisions(t *testing.T) {
	d := New("abc")
	mustSubmit(t, d, "a", "1", 0, insertAt(3, 0, "1")) // "1abc"
	mustSubmit(t, d, "a", "2", 1, insertAt(4, 0, "2")) // "21abc"
	mustSubmit(t, d, "a", "3", 2, deleteAt(5, 4, 1))   // "21ab"
	mustSubmit(t, d, "b", "1", 0, insertAt(3, 2, "X")) // based on "abc": "abXc"
	assertState(t, d, "21abX", 4)
}

func TestSubmitDuplicateIsIdempotent(t *testing.T) {
	d := New("abc")
	first := mustSubmit(t, d, "a", "1", 0, insertAt(3, 3, "d"))
	mustSubmit(t, d, "b", "1", 1, insertAt(4, 0, "Z"))

	// Client a never got its ack and resends with its original base.
	again := mustSubmit(t, d, "a", "1", 0, insertAt(3, 3, "d"))
	if again.Revision != first.Revision {
		t.Errorf("resend Revision = %d, want original %d", again.Revision, first.Revision)
	}
	assertState(t, d, "Zabcd", 2)
}

func TestSubmitResultDoesNotAliasHistory(t *testing.T) {
	d := New("abc")
	want := ot.Normalize(insertAt(3, 3, "d"))

	// Corrupt both the fresh result and a duplicate-submission result.
	first := mustSubmit(t, d, "a", "1", 0, insertAt(3, 3, "d"))
	first.Operation.Components[0] = ot.Delete(3)
	dup := mustSubmit(t, d, "a", "1", 0, insertAt(3, 3, "d"))
	if !reflect.DeepEqual(dup.Operation, want) {
		t.Fatalf("history changed through the returned result: got %+v, want %+v", dup.Operation, want)
	}
	dup.Operation.Components[0] = ot.Delete(3)

	again := mustSubmit(t, d, "a", "1", 0, insertAt(3, 3, "d"))
	if !reflect.DeepEqual(again.Operation, want) {
		t.Fatalf("history changed through a duplicate result: got %+v, want %+v", again.Operation, want)
	}

	// Rebasing from revision 0 has to go through the stored op.
	mustSubmit(t, d, "b", "1", 0, insertAt(3, 0, "Z"))
	assertState(t, d, "Zabcd", 2)
}

func TestSubmitRejectsReusedOpID(t *testing.T) {
	tests := []struct {
		name string
		base uint64
		op   ot.Operation
	}{
		{
			name: "same op at a later base revision",
			base: 2,
			op:   insertAt(5, 4, "d"),
		},
		{
			name: "different op at the original base revision",
			base: 0,
			op:   insertAt(3, 0, "X"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New("abc")
			mustSubmit(t, d, "a", "1", 0, insertAt(3, 3, "d"))
			mustSubmit(t, d, "b", "1", 1, insertAt(4, 0, "Z")) // "Zabcd", rev 2

			_, err := d.Submit("a", "1", tt.base, tt.op)
			if !errors.Is(err, ErrOperationIDReused) {
				t.Fatalf("Submit: err = %v, want ErrOperationIDReused", err)
			}
			assertState(t, d, "Zabcd", 2)
		})
	}
}

func TestSubmitDuplicateMatchesAfterNormalize(t *testing.T) {
	d := New("abc")
	first := mustSubmit(t, d, "a", "1", 0, insertAt(3, 3, "d"))

	// The same edit, written with split components.
	split := ot.Operation{
		Components:   []ot.Component{ot.Retain(1), ot.Retain(2), ot.Insert("d")},
		BaseLength:   3,
		TargetLength: 4,
	}
	again := mustSubmit(t, d, "a", "1", 0, split)
	if again.Revision != first.Revision {
		t.Errorf("resend Revision = %d, want original %d", again.Revision, first.Revision)
	}
	assertState(t, d, "abcd", 1)
}

func TestSubmitSameOpIDDifferentClients(t *testing.T) {
	d := New("")
	mustSubmit(t, d, "a", "1", 0, insertAt(0, 0, "a"))
	mustSubmit(t, d, "b", "1", 1, insertAt(1, 1, "b"))
	assertState(t, d, "ab", 2)
}

func TestSubmitErrorsLeaveStateUnchanged(t *testing.T) {
	tests := []struct {
		name string
		base uint64
		op   ot.Operation
	}{
		{
			name: "base revision from the future",
			base: 2,
			op:   insertAt(3, 0, "X"),
		},
		{
			name: "components disagree with declared lengths",
			base: 1,
			op:   ot.Operation{Components: []ot.Component{ot.Retain(3)}, BaseLength: 4, TargetLength: 4},
		},
		{
			name: "base length does not match current document",
			base: 1,
			op:   insertAt(10, 0, "X"),
		},
		{
			name: "base length does not match document at base revision",
			base: 0,
			op:   insertAt(4, 0, "X"), // doc was 3 runes at revision 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := New("abc")
			mustSubmit(t, d, "a", "1", 0, insertAt(3, 3, "d")) // "abcd", rev 1

			if _, err := d.Submit("b", "1", tt.base, tt.op); err == nil {
				t.Fatal("Submit: expected error, got nil")
			}
			assertState(t, d, "abcd", 1)
		})
	}
}

func TestSubmitConcurrentGoroutines(t *testing.T) {
	const n = 50
	d := New("abc")

	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := d.Submit(fmt.Sprintf("c%d", i), "1", 0, insertAt(3, 0, "x")); err != nil {
				t.Errorf("client %d: %v", i, err)
			}
		}()
	}
	wg.Wait()

	content, rev := d.Snapshot()
	if rev != n {
		t.Errorf("revision = %d, want %d", rev, n)
	}
	if want := n + 3; utf8.RuneCountInString(content) != want {
		t.Errorf("content has %d runes, want %d: %q", utf8.RuneCountInString(content), want, content)
	}
}
