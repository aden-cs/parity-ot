package document

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/aden-cs/parity-ot/internal/ot"
)

type CommittedOperation struct {
	Revision    uint64
	ClientID    string
	OperationID string
	Operation   ot.Operation
}

// clone copies Components so callers can't modify history through the result.
func (c CommittedOperation) clone() CommittedOperation {
	c.Operation.Components = slices.Clone(c.Operation.Components)
	return c
}

// ErrOperationIDReused means a committed (clientID, opID) was resubmitted with
// a different base revision or op.
var ErrOperationIDReused = errors.New("operation ID reused with different submission")

type submissionKey struct {
	clientID string
	opID     string
}

// submission is the original request, kept to check retries.
type submission struct {
	historyIndex int
	baseRevision uint64
	op           ot.Operation // normalized, before rebasing
}

type Document struct {
	mu          sync.Mutex
	content     string
	revision    uint64
	history     []CommittedOperation
	submissions map[submissionKey]submission
}

func New(content string) *Document {
	return &Document{content: content, submissions: make(map[submissionKey]submission)}
}

func (d *Document) Snapshot() (content string, revision uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.content, d.revision
}

func (d *Document) Submit(clientID, opID string, baseRevision uint64, op ot.Operation) (CommittedOperation, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if baseRevision > d.revision {
		return CommittedOperation{}, fmt.Errorf("base revision %d is greater than current revision %d", baseRevision, d.revision)
	}

	if err := ot.Validate(op); err != nil {
		return CommittedOperation{}, fmt.Errorf("invalid operation: %w", err)
	}

	op = ot.Normalize(op)
	original := op

	key := submissionKey{clientID, opID}
	if prev, ok := d.submissions[key]; ok {
		if prev.baseRevision != baseRevision || !operationsEqual(prev.op, op) {
			return CommittedOperation{}, fmt.Errorf("%w: client %q, op %q was committed at revision %d", ErrOperationIDReused, clientID, opID, d.history[prev.historyIndex].Revision)
		}
		return d.history[prev.historyIndex].clone(), nil
	}

	var err error
	for i := baseRevision; i < uint64(len(d.history)); i++ {
		prevOp := d.history[i]
		_, op, err = ot.Transform(prevOp.Operation, op)
		if err != nil {
			return CommittedOperation{}, err
		}
	}

	updatedContent, err := ot.Apply(op, d.content)
	if err != nil {
		return CommittedOperation{}, err
	}

	d.content = updatedContent
	d.revision++

	committedOp := CommittedOperation{
		Revision:    d.revision,
		ClientID:    clientID,
		OperationID: opID,
		Operation:   op,
	}

	d.history = append(d.history, committedOp)
	d.submissions[key] = submission{
		historyIndex: len(d.history) - 1,
		baseRevision: baseRevision,
		op:           original,
	}

	return committedOp.clone(), nil
}

func operationsEqual(a, b ot.Operation) bool {
	return a.BaseLength == b.BaseLength &&
		a.TargetLength == b.TargetLength &&
		slices.Equal(a.Components, b.Components)
}
