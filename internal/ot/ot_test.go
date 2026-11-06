package ot

import (
	"math"
	"math/rand"
	"slices"
	"testing"
	"unicode/utf8"
)

// newOp builds an operation with correct lengths. Use a plain Operation{}
// literal in cases that need the lengths to be wrong.
func newOp(components ...Component) Operation {
	op := Operation{Components: components}
	for _, c := range components {
		switch c.kind {
		case RetainKind:
			op.BaseLength += c.count
			op.TargetLength += c.count
		case DeleteKind:
			op.BaseLength += c.count
		case InsertKind:
			op.TargetLength += uint32(utf8.RuneCountInString(c.text))
		}
	}
	return op
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		op      Operation
		wantErr bool
	}{
		// Valid operations
		{name: "empty operation", op: Operation{}},
		{name: "retain only", op: Operation{Components: []Component{Retain(5)}, BaseLength: 5, TargetLength: 5}},
		{name: "delete only", op: Operation{Components: []Component{Delete(5)}, BaseLength: 5, TargetLength: 0}},
		{name: "insert only", op: Operation{Components: []Component{Insert("hi")}, BaseLength: 0, TargetLength: 2}},
		{name: "retain then insert", op: Operation{Components: []Component{Retain(5), Insert(" world")}, BaseLength: 5, TargetLength: 11}},
		{name: "all three kinds", op: Operation{Components: []Component{Retain(2), Delete(3), Insert("abc")}, BaseLength: 5, TargetLength: 5}},
		{name: "insert length counts runes", op: Operation{Components: []Component{Insert("😀")}, BaseLength: 0, TargetLength: 1}},

		// Invalid components
		{name: "zero retain", op: Operation{Components: []Component{Retain(0)}}, wantErr: true},
		{name: "zero delete", op: Operation{Components: []Component{Delete(0)}}, wantErr: true},
		{name: "empty insert", op: Operation{Components: []Component{Insert("")}}, wantErr: true},
		{name: "invalid UTF-8 insert", op: Operation{Components: []Component{Insert("\xff")}, TargetLength: 1}, wantErr: true},
		{name: "zero value component", op: Operation{Components: []Component{{}}}, wantErr: true},
		{name: "unknown kind", op: Operation{Components: []Component{{kind: Kind(99), count: 1}}, BaseLength: 1, TargetLength: 1}, wantErr: true},

		// Wrong lengths
		{name: "base length too small", op: Operation{Components: []Component{Retain(5)}, BaseLength: 4, TargetLength: 5}, wantErr: true},
		{name: "base length too large", op: Operation{Components: []Component{Retain(5)}, BaseLength: 6, TargetLength: 5}, wantErr: true},
		{name: "target length too small", op: Operation{Components: []Component{Retain(5)}, BaseLength: 5, TargetLength: 4}, wantErr: true},
		{name: "target length too large", op: Operation{Components: []Component{Retain(5)}, BaseLength: 5, TargetLength: 6}, wantErr: true},
		{name: "delete counted in target", op: Operation{Components: []Component{Delete(5)}, BaseLength: 5, TargetLength: 5}, wantErr: true},
		{name: "insert counted in base", op: Operation{Components: []Component{Insert("hi")}, BaseLength: 2, TargetLength: 2}, wantErr: true},
		{name: "insert length counted in bytes", op: Operation{Components: []Component{Insert("😀")}, BaseLength: 0, TargetLength: 4}, wantErr: true},
		{name: "lengths set with no components", op: Operation{BaseLength: 1, TargetLength: 1}, wantErr: true},

		// These lengths only match if the uint32 total wraps around to 0.
		{name: "length total does not overflow", op: Operation{Components: []Component{Retain(math.MaxUint32), Retain(1)}, BaseLength: 0, TargetLength: 0}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.op)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		op      Operation
		want    string
		wantErr bool
	}{
		// Single kinds
		{name: "empty operation on empty text", text: "", op: newOp(), want: ""},
		{name: "retain all", text: "hello", op: newOp(Retain(5)), want: "hello"},
		{name: "delete all", text: "hello", op: newOp(Delete(5)), want: ""},
		{name: "insert into empty text", text: "", op: newOp(Insert("hello")), want: "hello"},

		// Positions
		{name: "insert at start", text: "world", op: newOp(Insert("hello "), Retain(5)), want: "hello world"},
		{name: "insert in middle", text: "held", op: newOp(Retain(2), Insert("llo wor"), Retain(2)), want: "hello world"},
		{name: "insert at end", text: "hello", op: newOp(Retain(5), Insert(" world")), want: "hello world"},
		{name: "delete at start", text: "hello", op: newOp(Delete(2), Retain(3)), want: "llo"},
		{name: "delete in middle", text: "hello", op: newOp(Retain(1), Delete(3), Retain(1)), want: "ho"},
		{name: "delete at end", text: "hello", op: newOp(Retain(3), Delete(2)), want: "hel"},

		// Combinations
		{name: "replace by insert then delete", text: "hello", op: newOp(Insert("jelly"), Delete(5)), want: "jelly"},
		{name: "replace by delete then insert", text: "hello", op: newOp(Delete(5), Insert("jelly")), want: "jelly"},
		{name: "all three kinds", text: "hello", op: newOp(Retain(2), Delete(2), Insert("y"), Retain(1)), want: "heyo"},
		{name: "adjacent inserts keep order", text: "", op: newOp(Insert("a"), Insert("b")), want: "ab"},

		// Multi-byte text: counts must be in runes, not bytes
		{name: "retain past emoji", text: "😀b", op: newOp(Retain(1), Insert("x"), Retain(1)), want: "😀xb"},
		{name: "delete emoji", text: "a😀b", op: newOp(Retain(1), Delete(1), Retain(1)), want: "ab"},
		{name: "insert emoji", text: "ab", op: newOp(Retain(1), Insert("😀"), Retain(1)), want: "a😀b"},
		{name: "accented and CJK text", text: "é世界", op: newOp(Delete(1), Retain(1), Insert("!"), Retain(1)), want: "世!界"},

		// Text length does not match the operation
		{name: "text longer than base length", text: "hello", op: newOp(Retain(4)), wantErr: true},
		{name: "text shorter than base length", text: "hi", op: newOp(Retain(3)), wantErr: true},
		{name: "delete past end of text", text: "hi", op: newOp(Delete(3)), wantErr: true},
		{name: "non-empty text with empty operation", text: "hi", op: newOp(), wantErr: true},
		{name: "base length counted in bytes", text: "😀", op: newOp(Retain(4)), wantErr: true},

		// Text must be valid UTF-8
		{name: "invalid UTF-8 text", text: "a\xffb", op: newOp(Retain(3)), wantErr: true},

		// Apply must reject operations that fail Validate
		{name: "invalid component", text: "", op: Operation{Components: []Component{{}}}, wantErr: true},
		{name: "wrong target length", text: "hello", op: Operation{Components: []Component{Retain(5)}, BaseLength: 5, TargetLength: 6}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Apply(tc.op, tc.text)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Apply() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got != tc.want {
				t.Fatalf("Apply() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		input []Component
		want  []Component
	}{
		// Nothing to change
		{name: "empty operation", input: nil, want: nil},
		{name: "single retain", input: []Component{Retain(3)}, want: []Component{Retain(3)}},
		{name: "single delete", input: []Component{Delete(3)}, want: []Component{Delete(3)}},
		{name: "single insert", input: []Component{Insert("a")}, want: []Component{Insert("a")}},
		{name: "already normalized", input: []Component{Retain(1), Insert("a"), Delete(1), Retain(1)}, want: []Component{Retain(1), Insert("a"), Delete(1), Retain(1)}},

		// Merging adjacent components of the same kind
		{name: "merge retains", input: []Component{Retain(1), Retain(2)}, want: []Component{Retain(3)}},
		{name: "merge deletes", input: []Component{Delete(1), Delete(2)}, want: []Component{Delete(3)}},
		{name: "merge inserts in order", input: []Component{Insert("a"), Insert("b")}, want: []Component{Insert("ab")}},
		{name: "merge more than two", input: []Component{Retain(1), Retain(2), Retain(3)}, want: []Component{Retain(6)}},
		{name: "merge multi-byte inserts", input: []Component{Insert("😀"), Insert("世")}, want: []Component{Insert("😀世")}},

		// Inserts go before deletes
		{name: "move insert before delete", input: []Component{Delete(1), Insert("x")}, want: []Component{Insert("x"), Delete(1)}},
		{name: "keep insert before delete", input: []Component{Insert("x"), Delete(1)}, want: []Component{Insert("x"), Delete(1)}},
		{name: "reorder and merge interleaved edits", input: []Component{Insert("a"), Delete(1), Insert("b"), Delete(1)}, want: []Component{Insert("ab"), Delete(2)}},

		// Retains separate groups of edits and never move
		{name: "retain keeps inserts apart", input: []Component{Insert("a"), Retain(1), Insert("b")}, want: []Component{Insert("a"), Retain(1), Insert("b")}},
		{name: "retain keeps deletes apart", input: []Component{Delete(1), Retain(1), Delete(1)}, want: []Component{Delete(1), Retain(1), Delete(1)}},
		{name: "insert keeps retains apart", input: []Component{Retain(1), Insert("a"), Retain(1)}, want: []Component{Retain(1), Insert("a"), Retain(1)}},
		{name: "delete keeps retains apart", input: []Component{Retain(1), Delete(1), Retain(1)}, want: []Component{Retain(1), Delete(1), Retain(1)}},
		{name: "reorder only within each group", input: []Component{Delete(1), Insert("a"), Retain(1), Delete(1), Insert("b")}, want: []Component{Insert("a"), Delete(1), Retain(1), Insert("b"), Delete(1)}},

		// A flushed edit must not be written again later
		{name: "delete not repeated after merged retains", input: []Component{Delete(1), Retain(1), Retain(1)}, want: []Component{Delete(1), Retain(2)}},
		{name: "insert not repeated after merged retains", input: []Component{Insert("a"), Retain(1), Retain(1)}, want: []Component{Insert("a"), Retain(2)}},

		// Everything together
		{
			name:  "mixed operation",
			input: []Component{Retain(2), Delete(1), Insert("x"), Insert("y"), Delete(2), Retain(3), Retain(1)},
			want:  []Component{Retain(2), Insert("xy"), Delete(3), Retain(4)},
		},

		// The largest counts a valid operation can have
		{name: "merge retains up to max", input: []Component{Retain(math.MaxUint32 - 1), Retain(1)}, want: []Component{Retain(math.MaxUint32)}},
		{name: "merge deletes up to max", input: []Component{Delete(math.MaxUint32 - 1), Delete(1)}, want: []Component{Delete(math.MaxUint32)}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op := newOp(tc.input...)
			got := Normalize(op)
			if !slices.Equal(got.Components, tc.want) {
				t.Fatalf("Normalize() components = %v, want %v", got.Components, tc.want)
			}
			if got.BaseLength != op.BaseLength || got.TargetLength != op.TargetLength {
				t.Fatalf("Normalize() lengths = %d/%d, want %d/%d", got.BaseLength, got.TargetLength, op.BaseLength, op.TargetLength)
			}
		})
	}
}

func TestNormalizeDoesNotModifyInput(t *testing.T) {
	input := []Component{Delete(1), Insert("a"), Retain(1), Retain(1)}
	op := newOp(slices.Clone(input)...)

	Normalize(op)

	if !slices.Equal(op.Components, input) {
		t.Fatalf("Normalize() changed its input to %v, want %v", op.Components, input)
	}
}

func TestNormalizeProperties(t *testing.T) {
	// A fixed seed makes any failure repeatable.
	r := rand.New(rand.NewSource(1))

	for range 5000 {
		op := randomOp(r)
		text := randomText(r, op.BaseLength)
		normalized := Normalize(op)

		if err := Validate(normalized); err != nil {
			t.Fatalf("Normalize(%v) = %v is invalid: %v", op.Components, normalized.Components, err)
		}
		assertNormalForm(t, normalized)

		want, err := Apply(op, text)
		if err != nil {
			t.Fatalf("Apply(%v, %q) failed: %v", op.Components, text, err)
		}
		got, err := Apply(normalized, text)
		if err != nil {
			t.Fatalf("Apply(%v, %q) failed: %v", normalized.Components, text, err)
		}
		if got != want {
			t.Fatalf("Normalize(%v) changed the result on %q: got %q, want %q", op.Components, text, got, want)
		}

		if twice := Normalize(normalized); !slices.Equal(twice.Components, normalized.Components) {
			t.Fatalf("Normalize is not stable for %v: first %v, then %v", op.Components, normalized.Components, twice.Components)
		}
	}
}

// assertNormalForm fails if two neighboring components have the same kind or
// a delete comes directly before an insert.
func assertNormalForm(t *testing.T, op Operation) {
	t.Helper()
	for i := 1; i < len(op.Components); i++ {
		previous, current := op.Components[i-1].kind, op.Components[i].kind
		if previous == current || (previous == DeleteKind && current == InsertKind) {
			t.Fatalf("%v is not in normal form at component %d", op.Components, i)
		}
	}
}

// randomAlphabet includes multi-byte runes so tests catch byte/rune mix-ups.
var randomAlphabet = []rune("abcé世😀")

// randomOp builds a valid operation that is usually not normalized.
func randomOp(r *rand.Rand) Operation {
	components := make([]Component, 0, 10)
	for range r.Intn(11) {
		switch r.Intn(3) {
		case 0:
			components = append(components, Retain(uint32(r.Intn(4)+1)))
		case 1:
			components = append(components, Delete(uint32(r.Intn(4)+1)))
		case 2:
			components = append(components, Insert(randomText(r, uint32(r.Intn(3)+1))))
		}
	}
	return newOp(components...)
}

// randomText returns a string of exactly length runes.
func randomText(r *rand.Rand, length uint32) string {
	runes := make([]rune, length)
	for i := range runes {
		runes[i] = randomAlphabet[r.Intn(len(randomAlphabet))]
	}
	return string(runes)
}

func TestTransform(t *testing.T) {
	tests := []struct {
		name       string
		doc        string
		a, b       []Component
		wantAPrime []Component
		wantBPrime []Component
		want       string
	}{
		{name: "both empty", doc: "", a: nil, b: nil, wantAPrime: nil, wantBPrime: nil, want: ""},
		{
			name:       "inserts at different positions",
			doc:        "cat",
			a:          []Component{Retain(3), Insert("s")},
			b:          []Component{Insert("a "), Retain(3)},
			wantAPrime: []Component{Retain(5), Insert("s")},
			wantBPrime: []Component{Insert("a "), Retain(4)},
			want:       "a cats",
		},

		// When both insert at the same position, a's text goes first
		{
			name:       "tied inserts in empty document",
			doc:        "",
			a:          []Component{Insert("x")},
			b:          []Component{Insert("y")},
			wantAPrime: []Component{Insert("x"), Retain(1)},
			wantBPrime: []Component{Retain(1), Insert("y")},
			want:       "xy",
		},
		{
			name:       "tied inserts in the middle",
			doc:        "ab",
			a:          []Component{Retain(1), Insert("x"), Retain(1)},
			b:          []Component{Retain(1), Insert("y"), Retain(1)},
			wantAPrime: []Component{Retain(1), Insert("x"), Retain(2)},
			wantBPrime: []Component{Retain(2), Insert("y"), Retain(1)},
			want:       "axyb",
		},

		// Deletes
		{
			name:       "deletes at different positions",
			doc:        "abc",
			a:          []Component{Delete(1), Retain(2)},
			b:          []Component{Retain(2), Delete(1)},
			wantAPrime: []Component{Delete(1), Retain(1)},
			wantBPrime: []Component{Retain(1), Delete(1)},
			want:       "b",
		},
		{
			name:       "both delete the same text",
			doc:        "abc",
			a:          []Component{Retain(1), Delete(1), Retain(1)},
			b:          []Component{Retain(1), Delete(1), Retain(1)},
			wantAPrime: []Component{Retain(2)},
			wantBPrime: []Component{Retain(2)},
			want:       "ac",
		},
		{
			name:       "overlapping deletes",
			doc:        "abcd",
			a:          []Component{Delete(3), Retain(1)},
			b:          []Component{Retain(1), Delete(3)},
			wantAPrime: []Component{Delete(1)},
			wantBPrime: []Component{Delete(1)},
			want:       "",
		},
		{
			name:       "both delete everything",
			doc:        "abc",
			a:          []Component{Delete(3)},
			b:          []Component{Delete(3)},
			wantAPrime: nil,
			wantBPrime: nil,
			want:       "",
		},

		// Inserts mixed with deletes
		{
			name:       "insert where the other deletes",
			doc:        "abc",
			a:          []Component{Retain(1), Insert("x"), Retain(2)},
			b:          []Component{Retain(1), Delete(2)},
			wantAPrime: []Component{Retain(1), Insert("x")},
			wantBPrime: []Component{Retain(2), Delete(2)},
			want:       "ax",
		},
		{
			name:       "insert inside a deleted range survives",
			doc:        "abc",
			a:          []Component{Delete(3)},
			b:          []Component{Retain(1), Insert("x"), Retain(2)},
			wantAPrime: []Component{Delete(1), Retain(1), Delete(2)},
			wantBPrime: []Component{Insert("x")},
			want:       "x",
		},
		{
			name:       "insert after the other deletes everything",
			doc:        "ab",
			a:          []Component{Delete(2)},
			b:          []Component{Retain(2), Insert("z")},
			wantAPrime: []Component{Delete(2), Retain(1)},
			wantBPrime: []Component{Insert("z")},
			want:       "z",
		},

		// Lengths are counted in runes, not bytes
		{
			name:       "multi-byte text",
			doc:        "😀世",
			a:          []Component{Retain(2), Insert("é")},
			b:          []Component{Insert("世"), Retain(2)},
			wantAPrime: []Component{Retain(3), Insert("é")},
			wantBPrime: []Component{Insert("世"), Retain(3)},
			want:       "世😀世é",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, b := newOp(tc.a...), newOp(tc.b...)
			aPrime, bPrime, err := Transform(a, b)
			if err != nil {
				t.Fatalf("Transform() failed: %v", err)
			}
			if !slices.Equal(aPrime.Components, tc.wantAPrime) {
				t.Errorf("aPrime = %v, want %v", aPrime.Components, tc.wantAPrime)
			}
			if !slices.Equal(bPrime.Components, tc.wantBPrime) {
				t.Errorf("bPrime = %v, want %v", bPrime.Components, tc.wantBPrime)
			}
			assertConverges(t, a, b, aPrime, bPrime, tc.doc, tc.want)
		})
	}
}

func TestTransformErrors(t *testing.T) {
	tests := []struct {
		name string
		a, b Operation
	}{
		{name: "invalid a", a: newOp(Retain(0)), b: newOp()},
		{name: "invalid b", a: newOp(), b: newOp(Insert(""))},
		{name: "a has wrong lengths", a: Operation{Components: []Component{Retain(2)}, BaseLength: 1, TargetLength: 1}, b: newOp(Retain(1))},
		{name: "base lengths differ", a: newOp(Retain(1)), b: newOp(Retain(2))},
		{
			// Each op is valid on its own, but together they produce a
			// document one rune longer than a uint32 can count.
			name: "result too long",
			a:    newOp(Retain(math.MaxUint32-1), Insert("x")),
			b:    newOp(Retain(math.MaxUint32-1), Insert("y")),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			aPrime, bPrime, err := Transform(tc.a, tc.b)
			if err == nil {
				t.Fatalf("Transform() = %v, %v, want an error", aPrime, bPrime)
			}
		})
	}
}

// The tie rule (a's insert goes first) must hold however the inputs are
// written. Otherwise a client holding its op as sent and a server holding the
// normalized op break the same tie differently, and their documents diverge.
func TestTransformTieIgnoresComponentOrder(t *testing.T) {
	tests := []struct {
		name string
		a, b []Component
	}{
		{name: "a deletes before inserting", a: []Component{Delete(1), Insert("x")}, b: []Component{Insert("y"), Retain(1)}},
		{name: "both delete before inserting", a: []Component{Delete(1), Insert("x")}, b: []Component{Delete(1), Insert("y")}},
		{name: "b inserts before deleting", a: []Component{Delete(1), Insert("x")}, b: []Component{Insert("y"), Delete(1)}},
		{name: "already normalized", a: []Component{Insert("x"), Delete(1)}, b: []Component{Insert("y"), Retain(1)}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, b := newOp(tc.a...), newOp(tc.b...)
			aPrime, bPrime, err := Transform(a, b)
			if err != nil {
				t.Fatalf("Transform() failed: %v", err)
			}
			// a's "x" must come before b's "y" in both orders.
			assertConverges(t, a, b, aPrime, bPrime, "a", "xy")
		})
	}
}

func TestTransformProperties(t *testing.T) {
	// A fixed seed makes any failure repeatable.
	r := rand.New(rand.NewSource(1))

	for range 5000 {
		base := uint32(r.Intn(10))
		a, b := randomOpOnBase(r, base), randomOpOnBase(r, base)
		doc := randomText(r, base)
		aComponents, bComponents := slices.Clone(a.Components), slices.Clone(b.Components)

		aPrime, bPrime, err := Transform(a, b)
		if err != nil {
			t.Fatalf("Transform(%v, %v) failed: %v", a.Components, b.Components, err)
		}
		if !slices.Equal(a.Components, aComponents) || !slices.Equal(b.Components, bComponents) {
			t.Fatalf("Transform changed its inputs: a %v -> %v, b %v -> %v", aComponents, a.Components, bComponents, b.Components)
		}
		assertNormalForm(t, aPrime)
		assertNormalForm(t, bPrime)

		assertConverges(t, a, b, aPrime, bPrime, doc, "")

		// How the inputs are written must not change the result.
		normalAPrime, normalBPrime, err := Transform(Normalize(a), Normalize(b))
		if err != nil {
			t.Fatalf("Transform of normalized %v, %v failed: %v", a.Components, b.Components, err)
		}
		if !slices.Equal(aPrime.Components, normalAPrime.Components) || !slices.Equal(bPrime.Components, normalBPrime.Components) {
			t.Fatalf("Transform(%v, %v) = %v, %v, but with normalized inputs = %v, %v",
				a.Components, b.Components, aPrime.Components, bPrime.Components, normalAPrime.Components, normalBPrime.Components)
		}

		// Transforming against an op that changes nothing leaves a as it was.
		identity := newOp()
		if base > 0 {
			identity = newOp(Retain(base))
		}
		unchanged, _, err := Transform(a, identity)
		if err != nil {
			t.Fatalf("Transform(%v, identity) failed: %v", a.Components, err)
		}
		if normalized := Normalize(a); !slices.Equal(unchanged.Components, normalized.Components) {
			t.Fatalf("Transform(%v, identity) = %v, want %v", a.Components, unchanged.Components, normalized.Components)
		}
	}
}

// assertConverges checks that aPrime and bPrime have the right lengths and that
// applying a then bPrime gives the same text as b then aPrime. If want is not
// empty, that text must also equal want.
func assertConverges(t *testing.T, a, b, aPrime, bPrime Operation, doc, want string) {
	t.Helper()
	if aPrime.BaseLength != b.TargetLength {
		t.Fatalf("aPrime.BaseLength = %d, want b.TargetLength %d", aPrime.BaseLength, b.TargetLength)
	}
	if bPrime.BaseLength != a.TargetLength {
		t.Fatalf("bPrime.BaseLength = %d, want a.TargetLength %d", bPrime.BaseLength, a.TargetLength)
	}

	left := mustApply(t, bPrime, mustApply(t, a, doc))
	right := mustApply(t, aPrime, mustApply(t, b, doc))
	if left != right {
		t.Fatalf("a=%v b=%v on %q: a then bPrime = %q, b then aPrime = %q", a.Components, b.Components, doc, left, right)
	}
	if want != "" && left != want {
		t.Fatalf("a=%v b=%v on %q: got %q, want %q", a.Components, b.Components, doc, left, want)
	}
}

// mustApply applies op to text and fails the test if that returns an error.
// Apply validates op first, so this also catches wrong lengths.
func mustApply(t *testing.T, op Operation, text string) string {
	t.Helper()
	result, err := Apply(op, text)
	if err != nil {
		t.Fatalf("Apply(%v, %q) failed: %v", op.Components, text, err)
	}
	return result
}

// randomOpOnBase builds a valid operation whose BaseLength is base. It mixes
// inserts between the retains and deletes, so the result is usually not
// normalized.
func randomOpOnBase(r *rand.Rand, base uint32) Operation {
	var components []Component
	for remaining := base; remaining > 0; {
		if r.Intn(3) == 0 {
			components = append(components, Insert(randomText(r, uint32(r.Intn(3)+1))))
		}
		n := min(uint32(r.Intn(4)+1), remaining)
		if r.Intn(2) == 0 {
			components = append(components, Retain(n))
		} else {
			components = append(components, Delete(n))
		}
		remaining -= n
	}
	if r.Intn(2) == 0 {
		components = append(components, Insert(randomText(r, uint32(r.Intn(3)+1))))
	}
	return newOp(components...)
}
