package oci

import (
	"sync"

	"github.com/cri-o/cri-o/pkg/oci"
)

// memoryStore implements a Store in memory.
type memoryStore struct {
	s map[string]*oci.Container
	sync.RWMutex
}

// NewMemoryStore initializes a new memory store.
func NewMemoryStore() ContainerStorer {
	return &memoryStore{
		s: make(map[string]*oci.Container),
	}
}

// Add appends a new container to the memory store.
// It overrides the id if it existed before.
func (c *memoryStore) Add(id string, cont *oci.Container) {
	c.Lock()
	c.s[id] = cont
	c.Unlock()
}

// Get returns a container from the store by id.
func (c *memoryStore) Get(id string) *oci.Container {
	var res *oci.Container
	c.RLock()
	res = c.s[id]
	c.RUnlock()
	return res
}

// Delete removes a container from the store by id.
func (c *memoryStore) Delete(id string) {
	c.Lock()
	delete(c.s, id)
	c.Unlock()
}

// List returns a sorted list of containers from the store.
// The containers are ordered by creation date.
func (c *memoryStore) List() []*oci.Container {
	containers := History(c.all())
	containers.sort()
	return containers
}

// Size returns the number of containers in the store.
func (c *memoryStore) Size() int {
	c.RLock()
	defer c.RUnlock()
	return len(c.s)
}

// First returns the first container found in the store by a given filter.
func (c *memoryStore) First(filter StoreFilter) *oci.Container {
	for _, cont := range c.all() {
		if filter == nil || filter(cont) {
			return cont
		}
	}
	return nil
}

// ApplyAll calls the reducer function with every container in the store.
// This operation is asynchronous in the memory store.
// NOTE: Modifications to the store MUST NOT be done by the StoreReducer.
func (c *memoryStore) ApplyAll(apply StoreReducer) {
	if apply == nil {
		return
	}
	wg := new(sync.WaitGroup)
	for _, cont := range c.all() {
		wg.Add(1)
		go func(container *oci.Container) {
			apply(container)
			wg.Done()
		}(cont)
	}

	wg.Wait()
}

func (c *memoryStore) all() []*oci.Container {
	c.RLock()
	containers := make([]*oci.Container, 0, len(c.s))
	for _, cont := range c.s {
		containers = append(containers, cont)
	}
	c.RUnlock()
	return containers
}

var _ ContainerStorer = &memoryStore{}
