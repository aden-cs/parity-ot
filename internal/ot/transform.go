package ot

import (
	"fmt"
	"math"
	"unicode/utf8"
)

// Transform takes a and b made against the same document and returns aPrime,
// which applies a after b, and bPrime, which applies b after a. When both
// insert at the same position, a's text goes first.
func Transform(a, b Operation) (Operation, Operation, error) {
	if err:= Validate(a); err != nil {
		return Operation{}, Operation{}, fmt.Errorf("invalid a: %w", err)
	}
	if err:= Validate(b); err != nil {
		return Operation{}, Operation{}, fmt.Errorf("invalid b: %w", err)
	}
	if a.BaseLength != b.BaseLength {
		return Operation{}, Operation{}, fmt.Errorf("base length mismatch: a has %d, b has %d", a.BaseLength, b.BaseLength)
	}

	a, b = Normalize(a), Normalize(b)

	var aPrime, bPrime Operation
	
	var ca, cb Component
	var haveA, haveB bool
	ia, ib := 0, 0

	for {
		if !haveA && ia < len(a.Components) {
			ca, haveA = a.Components[ia], true
			ia++
		}
		if !haveB && ib < len(b.Components) {
			cb, haveB = b.Components[ib], true
			ib++
		}
		if !haveA && !haveB {
			break
		}

		if haveA && ca.kind == InsertKind {
			aPrime.Components = append(aPrime.Components, ca)
			bPrime.Components = append(bPrime.Components, Retain(runeCount(ca.text)))
			haveA = false
			continue
		}
		if haveB && cb.kind == InsertKind {
			aPrime.Components = append(aPrime.Components, Retain(runeCount(cb.text)))
			bPrime.Components = append(bPrime.Components, cb)
			haveB = false
			continue
		}

		if !haveA || !haveB {
			return Operation{}, Operation{}, fmt.Errorf("unexpected end of components")
		}
		n := min(ca.count, cb.count)
		switch {
			case ca.kind == RetainKind && cb.kind == RetainKind:
				aPrime.Components = append(aPrime.Components, Retain(n))
				bPrime.Components = append(bPrime.Components, Retain(n))
			case ca.kind == DeleteKind && cb.kind == RetainKind:
				aPrime.Components = append(aPrime.Components, Delete(n))
			case ca.kind == RetainKind && cb.kind == DeleteKind:
				bPrime.Components = append(bPrime.Components, Delete(n))
		}

		ca.count -= n
		cb.count -= n
		haveA = ca.count > 0
		haveB = cb.count > 0
	}

	target, err := targetLength(aPrime)
	if err != nil {
		return Operation{}, Operation{}, err
	}
	aPrime.BaseLength, aPrime.TargetLength = b.TargetLength, target
	bPrime.BaseLength, bPrime.TargetLength = a.TargetLength, target
	return Normalize(aPrime), Normalize(bPrime), nil
}

func targetLength(op Operation) (uint32, error) {
	var length uint64
	for _, c := range op.Components {
		switch c.kind {
		case RetainKind:
			length += uint64(c.count)
		case DeleteKind:
			// do nothing
		case InsertKind:
			length += uint64(runeCount(c.text))
		default:
			return 0, fmt.Errorf("invalid component kind %d", c.kind)
		}
	}
	if length > math.MaxUint32 {
		return 0, fmt.Errorf("target length exceeds maximum uint32 value")
	}
	return uint32(length), nil
}

func runeCount(s string) uint32 {
	return uint32(utf8.RuneCountInString(s))
}