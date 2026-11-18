package upload

import (
	"slices"
	"sync"
)

// State is what a Store keeps of an upload on its way, so a Run that failed,
// or a page that reloaded, sends the parts the server is missing.
type State struct {
	UploadID string
	PartSize int64
	Size     int64
	Parts    []Part
}

// Store keeps the state of the uploads on their way, by key. An app implements
// it where it keeps its data: localStorage in a browser, a file in a window.
type Store interface {
	// Load returns the state of key, and false when it keeps none.
	Load(key string) (State, bool)
	// Save keeps the state of key.
	Save(key string, state State) error
	// Delete forgets key.
	Delete(key string) error
}

// Memory is a Store in memory: it resumes while the app runs, and no longer.
type Memory struct {
	mu    sync.Mutex
	state map[string]State
}

// NewMemory returns an empty Store in memory.
func NewMemory() *Memory {
	return &Memory{state: map[string]State{}}
}

// Load returns the state of key, copied.
func (m *Memory) Load(key string) (State, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.state[key]
	if !ok {
		return State{}, false
	}
	state.Parts = slices.Clone(state.Parts)
	return state, true
}

// Save keeps the state of key, copied.
func (m *Memory) Save(key string, state State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state.Parts = slices.Clone(state.Parts)
	m.state[key] = state
	return nil
}

// Delete forgets key.
func (m *Memory) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state, key)
	return nil
}
