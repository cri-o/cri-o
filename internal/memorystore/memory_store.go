package memorystore

import (
	"sync"
	"time"
)

// AnyCreated is the interface for values where the creation time can be retrieved.
type AnyCreated[T any] interface {
	// CreatedAt returns the creation time of the value.
	CreatedAt() time.Time
}

// Storer defines an interface that any store must implement.
type Storer[T AnyCreated[T]] interface {
	// Add appends a new value to the store.
	Add(string, T)
	// Get returns a value from the store by the identifier it was stored with.
	Get(string) T
	// Delete removes a value from the store by the identifier it was stored with.
	Delete(string)
	// List returns a list of values from the store.
	List() []T
	// First returns the first value found in the store by a given filter.
	First(StoreFilter[T]) T
	// ApplyAll calls the reducer function with every value in the store.
	ApplyAll(StoreReducer[T])
}

// StoreFilter defines a function to filter values in the store.
type StoreFilter[T AnyCreated[T]] func(T) bool

// StoreReducer defines a function to manipulate values in the store.
type StoreReducer[T AnyCreated[T]] func(T)

// memoryStore implements a Store in memory.
type memoryStore[T AnyCreated[T]] struct {
	s sync.Map
}

// New initializes a new memory store.
func New[T AnyCreated[T]]() Storer[T] {
	return &memoryStore[T]{s: sync.Map{}}
}

// Add appends a new value to the memory store.
// It overrides the id if it existed before.
func (c *memoryStore[T]) Add(id string, value T) {
	c.s.Store(id, value)
}

// Get returns a value from the store by id.
func (c *memoryStore[T]) Get(id string) (res T) {
	v, _ := c.s.Load(id)

	typedValue, ok := v.(T)
	if ok {
		return typedValue
	}

	return res
}

// Delete removes a value from the store by id.
func (c *memoryStore[T]) Delete(id string) {
	c.s.Delete(id)
}

// List returns a sorted list of values from the store.
// The values are ordered by creation date.
func (c *memoryStore[T]) List() []T {
	values := History[T](c.all())
	values.sort()

	return values
}

// First returns the first value found in the store by a given filter.
func (c *memoryStore[T]) First(filter StoreFilter[T]) (res T) {
	for _, value := range c.all() {
		if filter == nil || filter(value) {
			return value
		}
	}

	return res
}

// ApplyAll calls the reducer function with every value in the store.
// This operation is asynchronous in the memory store.
func (c *memoryStore[T]) ApplyAll(apply StoreReducer[T]) {
	if apply == nil {
		return
	}

	wg := new(sync.WaitGroup)
	for _, value := range c.all() {
		wg.Add(1)

		go func(value T) {
			apply(value)
			wg.Done()
		}(value)
	}

	wg.Wait()
}

func (c *memoryStore[T]) all() (values []T) {
	for _, v := range c.s.Range {
		typedValue, ok := v.(T)
		if ok {
			values = append(values, typedValue)
		}
	}

	return values
}
