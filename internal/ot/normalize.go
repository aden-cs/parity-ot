package ot

import "strings"

// Normalize merges adjacent components of the same kind and places inserts
// before deletes between retains. The operation must already pass Validate.
func Normalize(op Operation) Operation {
	result := Operation{BaseLength: op.BaseLength, TargetLength: op.TargetLength}
	var pendingInsert strings.Builder
	var pendingDelete, pendingRetain uint32

	flushEdits := func() {
		if pendingInsert.Len() > 0 {
			result.Components = append(result.Components, Insert(pendingInsert.String()))
			pendingInsert.Reset()
		}
		if pendingDelete > 0 {
			result.Components = append(result.Components, Delete(pendingDelete))
			pendingDelete = 0
		}
	}
	flushRetain := func() {
		if pendingRetain > 0 {
			result.Components = append(result.Components, Retain(pendingRetain))
			pendingRetain = 0
		}
	}

	for _, c := range op.Components {
		switch c.kind {
		case RetainKind:
			flushEdits()
			pendingRetain += c.count
		case DeleteKind:
			flushRetain()
			pendingDelete += c.count
		case InsertKind:
			flushRetain()
			pendingInsert.WriteString(c.text)
		}
	}

	flushRetain()
	flushEdits()
	return result
}