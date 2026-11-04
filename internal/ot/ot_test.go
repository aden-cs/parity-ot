package ot

import (
	"math"
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
