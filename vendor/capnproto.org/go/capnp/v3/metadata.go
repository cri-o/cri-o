package capnp

import (
	"sync"
)

// Metadata is a morally a map[any]any which implements
// sync.Locker; it is used by the rpc system to attach bookkeeping
// information to various objects.
//
// The zero value is not meaningful, and the Metadata must not be copied
// after its first use.
type Metadata struct {
	mu     sync.Mutex
	values map[any]any
}

// Lock the metadata map.
func (m *Metadata) Lock() {
	m.mu.Lock()
}

// Unlock the metadata map.
func (m *Metadata) Unlock() {
	m.mu.Unlock()
}

// Look up key in the map. Returns the value, and a boolean which is
// false if the key was not present.
func (m *Metadata) Get(key any) (value any, ok bool) {
	value, ok = m.values[key]
	return
}

// Insert the key, value pair into the map.
func (m *Metadata) Put(key, value any) {
	m.values[key] = value
}

// Delete the key from the map.
func (m *Metadata) Delete(key any) {
	delete(m.values, key)
}

// Allocate and return a freshly initialized Metadata.
func NewMetadata() *Metadata {
	return &Metadata{
		values: make(map[any]any),
	}
}
