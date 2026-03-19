package keystore

import "sync"

// MemoryMasterKey holds a validated master key only in process RAM (never persisted).
// Used after the user unlocks the keyring from the browser; server handlers may fall
// back to this when a request does not include an explicit password.
type MemoryMasterKey struct {
	mu   sync.RWMutex
	pass string
}

// Set stores the master key in memory, replacing any previous value.
func (m *MemoryMasterKey) Set(p string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pass = p
}

// Get returns the stored key and whether one is present.
func (m *MemoryMasterKey) Get() (string, bool) {
	if m == nil {
		return "", false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.pass == "" {
		return "", false
	}
	return m.pass, true
}

// Clear removes the key from memory.
func (m *MemoryMasterKey) Clear() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pass = ""
}
