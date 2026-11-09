package document

import (
	"fmt"
	"sync"

	"github.com/aden-cs/parity-ot/internal/ot"
)

type CommittedOperation struct {
	Revision    uint64
	ClientID    string
	OperationID string
	Operation   ot.Operation
}

type Document struct {
	mu       sync.Mutex
	content  string
	revision uint64
	history  []CommittedOperation
}

func New(content string) *Document {
	return &Document{content: content}
}

func (d *Document) Snapshot() (content string, revision uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.content, d.revision
}

// findCommitted returns the committed operation matching clientID and opID,
// if one exists. A retried op can only have been committed after its own base
// revision, so the scan starts there. The caller must hold d.mu.
func (d *Document) findCommitted(clientID, opID string, baseRevision uint64) (CommittedOperation, bool) {
	for i := baseRevision; i < uint64(len(d.history)); i++ {
		committed := d.history[i]
		if committed.ClientID == clientID && committed.OperationID == opID {
			return committed, true
		}
	}
	return CommittedOperation{}, false
}

func (d *Document) Submit(clientID, opID string, baseRevision uint64, op ot.Operation) (CommittedOperation, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if baseRevision > d.revision {
		return CommittedOperation{}, fmt.Errorf("base revision %d is greater than current revision %d", baseRevision, d.revision)
	}

	if committed, ok := d.findCommitted(clientID, opID, baseRevision); ok {
		return committed, nil
	}

	if err := ot.Validate(op); err != nil {
		return CommittedOperation{}, fmt.Errorf("invalid operation: %w", err)
	}

	op = ot.Normalize(op)

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

	return committedOp, nil
}
