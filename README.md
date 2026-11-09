# parity-ot

A real-time collaborative text editing server in Go, built on operational transformation (OT).
Clients send edits over WebSocket, the server rebases each edit against any edits committed since the client last synced, and then broadcasts the result. Every client ends up with the same document.

## Status

| Package             | Status          |
| ------------------- | --------------- |
| `internal/ot`       | Implemented, tested |
| `internal/document` | Implemented, tested |
| `internal/server`   | Not started     |
| `proto/`            | Not started     |

## Getting started

Requires Go 1.26.5 or later.

```sh
go build ./...
go test ./...              # all packages
go test -race -v ./...     # with race detector and per-test output
```

## Project layout

```
cmd/parity-ot/       entry point
internal/ot/         pure OT algorithm, no I/O or state
internal/document/   authoritative server-side document state
internal/server/     HTTP, WebSocket, and protobuf messaging (planned)
proto/               wire protocol definitions (planned)
```

## Packages

### `internal/ot`

Pure functions over operations. An operation is a sequence of components that walks the whole document from start to end.

```go
type Component struct { /* unexported: kind, count, text */ }

func Retain(count uint32) Component // keep the next count runes
func Delete(count uint32) Component // remove the next count runes
func Insert(text string) Component  // insert text at the current position

type Operation struct {
    Components   []Component
    BaseLength   uint32 // rune length of the document the op applies to
    TargetLength uint32 // rune length of the document after applying
}
```

```go
// Validate checks that components are well formed and that their lengths
// add up to BaseLength and TargetLength.
func Validate(op Operation) error

// Normalize merges adjacent components of the same kind and orders inserts
// before deletes. op must already pass Validate.
func Normalize(op Operation) Operation

// Apply applies op to text. len([]rune(text)) must equal op.BaseLength.
func Apply(op Operation, text string) (string, error)

// Transform takes a and b made against the same document and returns
// aPrime (a rebased to apply after b) and bPrime (b rebased to apply after a),
// so that Apply(bPrime, Apply(a, doc)) == Apply(aPrime, Apply(b, doc)).
func Transform(a, b Operation) (aPrime, bPrime Operation, err error)
```

### `internal/document`

The authoritative copy of one document: its content, revision counter, and the history of committed operations. All methods are safe to call from multiple goroutines.

```go
type CommittedOperation struct {
    Revision    uint64       // revision after applying this op
    ClientID    string
    OperationID string
    Operation   ot.Operation // already rebased to apply at Revision-1
}

// New returns a document at revision 0 with the given content.
func New(content string) *Document

// Snapshot returns the current content and revision, for clients joining the document.
func (d *Document) Snapshot() (content string, revision uint64)

// Submit rebases op from baseRevision to the current revision, applies it,
// and appends it to history. It returns the committed operation, which the
// server sends back as the ack and broadcasts to other clients.
func (d *Document) Submit(clientID, opID string, baseRevision uint64, op ot.Operation) (CommittedOperation, error)
```

`Submit` returns an error, and leaves the document unchanged, if the op is invalid, if `baseRevision` is ahead of the current revision, if the op's `BaseLength` doesn't match the document at `baseRevision`, or if `(clientID, opID)` was already committed with a different `baseRevision` or op (`ErrOperationIDReused`).

`Manager` holds many documents by ID. Its `sync.RWMutex` only guards the map, so different documents can be edited in parallel.

```go
// NewManager returns an empty Manager.
func NewManager() *Manager

// Get returns the document with the given ID, or nil and false if it doesn't exist.
func (m *Manager) Get(id string) (*Document, bool)

// GetOrCreate returns the document with the given ID, creating it with initial
// as its content if it doesn't exist. If it already exists, initial is ignored.
func (m *Manager) GetOrCreate(id, initial string) *Document
```

### `internal/server` (planned)

Handles all communication: the HTTP server, WebSocket connections, protobuf encoding and decoding, joining documents, calling into `document`, and broadcasting committed operations.

## Design decisions

- **Lengths and positions count runes, not bytes**, so multi-byte characters can't be split.
- **Revision numbering:** a document at revision `r` has `r` committed ops. A `CommittedOperation` with `Revision == N` turns revision `N-1` into revision `N`. A client at revision `r` applies a broadcast op when its `Revision == r+1`.
- **Tie-breaking:** when two ops insert at the same position, `Transform` puts `a`'s text first. `Submit` passes the already-committed op as `a`, so **the earlier commit goes first**. Clients must make the same choice by calling `ot.Transform(serverOp, pendingOp)`.
- **Idempotent submits:** resending the same `(clientID, opID)` with the same `baseRevision` and op returns the original `CommittedOperation` instead of applying the op twice. A client that loses an ack (for example, after a disconnect) can safely resend, but it must resend the op exactly as first sent, not rebased onto newer revisions. Reusing an ID with a different base or op is rejected.
- **History stores rebased ops only.** Each op's effective base is its position in history. A separate index keyed by `(clientID, opID)` keeps each submission's original `baseRevision` and normalized op, which is used to check retries.

## Message flow

```
client ──op──▶ server: decode protobuf
                 │
                 ▼
          Manager: look up document
                 │
                 ▼
          Document.Submit: validate → rebase → apply → append
                 │
                 ├──▶ ack to sender (CommittedOperation)
                 └──▶ broadcast to other clients
```
