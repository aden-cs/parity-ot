Structure:

# internal/ot:
Owns pure OT algorithm

- Apply(document, operation)
- Transform(opA, opB)
- Compose(opA, opB)
- Validate(operation)
- Normalize(operation)

# internal/document:
Authoritiative server state:
type Document struct {
    mu sync.Mutex
    content string
    revision uint64
    history []CommittedOperation
}

Handles current doc content, revisions, op history, stal submissiong, applying committed operations, duplicate operation IDs

Manager holds multiple documents:
type Manager struct {
    mu sync.RWMutex ?
    documents map[string]*Document
}

# internal/server
Everything related to communication:
- HTTP server
- Websocket
- reading/writing messages
- protobuf encoding/decoding
- joining docs
- calling document.Manager
- broadcasting committed ops

# message flow:
websocket msg -> server decodes protobuf -> document.Manager finds doc -> doc transforms and commits op -> server sends ack -> server broadcasts op