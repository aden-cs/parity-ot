package document

import "sync"

// Manager holds documents by ID. Its lock guards only the map; each Document
// has its own lock for edits.
type Manager struct {
	mu   sync.RWMutex
	docs map[string]*Document
}

func NewManager() *Manager {
	return &Manager{
		docs: make(map[string]*Document),
	}
}

func (m *Manager) Get(id string) (*Document, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	doc, ok := m.docs[id]
	return doc, ok
}

// GetOrCreate returns the document with the given ID, creating it with initial
// as its content if needed. initial is ignored if the document already exists.
func (m *Manager) GetOrCreate(id, initial string) *Document {
	if doc, ok := m.Get(id); ok {
		return doc
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Another goroutine may have created it between the RUnlock and Lock.
	if doc, ok := m.docs[id]; ok {
		return doc
	}
	doc := New(initial)
	m.docs[id] = doc
	return doc
}
