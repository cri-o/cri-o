package oci

import (
	"github.com/cri-o/cri-o/pkg/oci"
)

// StoreFilter defines a function to filter
// container in the store.
type StoreFilter func(*oci.Container) bool

// StoreReducer defines a function to
// manipulate containers in the store
type StoreReducer func(*oci.Container)

// ContainerStorer defines an interface that any container store must implement.
type ContainerStorer interface {
	// Add appends a new container to the store.
	Add(string, *oci.Container)
	// Get returns a container from the store by the identifier it was stored with.
	Get(string) *oci.Container
	// Delete removes a container from the store by the identifier it was stored with.
	Delete(string)
	// List returns a list of containers from the store.
	List() []*oci.Container
	// Size returns the number of containers in the store.
	Size() int
	// First returns the first container found in the store by a given filter.
	First(StoreFilter) *oci.Container
	// ApplyAll calls the reducer function with every container in the store.
	ApplyAll(StoreReducer)
}
