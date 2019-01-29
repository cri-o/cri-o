package lock

import (
	"sync"

	"github.com/pkg/errors"
)

// Mutex holds a single mutex and whether it has been allocated.
type Mutex struct {
	id        uint32
	lock      sync.Mutex
	allocated bool
}

// ID retrieves the ID of the mutex
func (m *Mutex) ID() uint32 {
	return m.id
}

// Lock locks the mutex
func (m *Mutex) Lock() {
	m.lock.Lock()
}

// Unlock unlocks the mutex
func (m *Mutex) Unlock() {
	m.lock.Unlock()
}

// Free deallocates the mutex to allow its reuse
func (m *Mutex) Free() error {
	m.allocated = false

	return nil
}

// InMemoryManager is a lock manager that allocates and retrieves local-only
// locks - that is, they are not multiprocess. This lock manager is intended
// purely for unit and integration testing and should not be used in production
// deployments.
type InMemoryManager struct {
	locks     []*Mutex
	numLocks  uint32
	localLock sync.Mutex
}

// NewInMemoryManager creates a new in-memory lock manager with the given number
// of locks.
func NewInMemoryManager(numLocks uint32) (Manager, error) {
	if numLocks == 0 {
		return nil, errors.Errorf("must provide a non-zero number of locks!")
	}

	manager := new(InMemoryManager)
	manager.numLocks = numLocks
	manager.locks = make([]*Mutex, numLocks)

	var i uint32
	for i = 0; i < numLocks; i++ {
		lock := new(Mutex)
		lock.id = i
		manager.locks[i] = lock
	}

	return manager, nil
}

// AllocateLock allocates a lock from the manager.
func (m *InMemoryManager) AllocateLock() (Locker, error) {
	m.localLock.Lock()
	defer m.localLock.Unlock()

	for _, lock := range m.locks {
		if !lock.allocated {
			lock.allocated = true
			return lock, nil
		}
	}

	return nil, errors.Errorf("all locks have been allocated")
}

// RetrieveLock retrieves a lock from the manager.
func (m *InMemoryManager) RetrieveLock(id uint32) (Locker, error) {
	if id >= m.numLocks {
		return nil, errors.Errorf("given lock ID %d is too large - this manager only supports lock indexes up to %d", id, m.numLocks-1)
	}

	return m.locks[id], nil
}
