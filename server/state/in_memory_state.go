package state

import (
	"fmt"
	"sync"

	"github.com/docker/docker/pkg/registrar"
	"github.com/docker/docker/pkg/truncindex"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/server/sandbox"
)

// TODO: make operations atomic to greatest extent possible

// InMemoryState is an in-memory state store, suitable for use when no other
// programs are expected to interact with the server
type InMemoryState struct {
	lock         sync.Mutex
	sandboxes    map[string]*sandbox.Sandbox
	containers   oci.Store
	podNameIndex *registrar.Registrar
	podIDIndex   *truncindex.TruncIndex
	ctrNameIndex *registrar.Registrar
	ctrIDIndex   *truncindex.TruncIndex
}

// NewInMemoryState creates a new, empty server state
func NewInMemoryState() Store {
	state := new(InMemoryState)
	state.sandboxes = make(map[string]*sandbox.Sandbox)
	state.containers = oci.NewMemoryStore()
	state.podNameIndex = registrar.NewRegistrar()
	state.podIDIndex = truncindex.NewTruncIndex([]string{})
	state.ctrNameIndex = registrar.NewRegistrar()
	state.ctrIDIndex = truncindex.NewTruncIndex([]string{})

	return state
}

// AddSandbox adds a sandbox and any containers in it to the state
func (s *InMemoryState) AddSandbox(sandbox *sandbox.Sandbox) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, exist := s.sandboxes[sandbox.ID()]; exist {
		return fmt.Errorf("sandbox with ID %v already exists", sandbox.ID())
	}

	// We shouldn't share ID with any containers, either
	// Our pod infra container will share our ID and we don't want it to conflict with anything
	if ctrCheck := s.containers.Get(sandbox.ID()); ctrCheck != nil {
		return fmt.Errorf("requested sandbox ID %v conflicts with existing container ID", sandbox.ID())
	}

	s.sandboxes[sandbox.ID()] = sandbox
	if err := s.podNameIndex.Reserve(sandbox.Name(), sandbox.ID()); err != nil {
		return fmt.Errorf("error registering sandbox name: %v", err)
	}
	if err := s.podIDIndex.Add(sandbox.ID()); err != nil {
		return fmt.Errorf("error registering sandbox ID: %v", err)
	}

	// If there are containers in the sandbox add them to the mapping
	containers := sandbox.Containers()
	for _, ctr := range containers {
		if err := s.addContainerMappings(ctr, true); err != nil {
			return fmt.Errorf("error adding container %v mappings in sandbox %v", ctr.ID(), sandbox.ID())
		}
	}

	// Add the pod infrastructure container to mappings
	// TODO: Right now, we don't add it to the all containers listing. We may want to change this.
	if err := s.addContainerMappings(sandbox.InfraContainer(), false); err != nil {
		return fmt.Errorf("error adding infrastructure container %v to mappings: %v", sandbox.InfraContainer().ID(), err)
	}

	return nil
}

// HasSandbox determines if a given sandbox exists in the state
func (s *InMemoryState) HasSandbox(id string) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	_, exist := s.sandboxes[id]

	return exist
}

// DeleteSandbox removes a sandbox from the state
func (s *InMemoryState) DeleteSandbox(id string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, exist := s.sandboxes[id]; !exist {
		return fmt.Errorf("no sandbox with ID %v exists, cannot delete", id)
	}

	name := s.sandboxes[id].Name()
	containers := s.sandboxes[id].Containers()
	infraContainer := s.sandboxes[id].InfraContainer()

	delete(s.sandboxes, id)
	s.podNameIndex.Release(name)
	if err := s.podIDIndex.Delete(id); err != nil {
		return fmt.Errorf("error unregistering sandbox ID: %v", err)
	}

	// If there are containers left in the sandbox delete them from the mappings
	for _, ctr := range containers {
		if err := s.deleteContainerMappings(ctr, true); err != nil {
			return fmt.Errorf("error removing container %v mappings: %v", ctr.ID(), err)
		}
	}

	// Delete infra container from mappings
	if err := s.deleteContainerMappings(infraContainer, false); err != nil {
		return fmt.Errorf("error removing infra container %v from mappings: %v", infraContainer.ID(), err)
	}

	return nil
}

// GetSandbox returns a sandbox given its full ID
func (s *InMemoryState) GetSandbox(id string) (*sandbox.Sandbox, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	sandbox, ok := s.sandboxes[id]
	if !ok {
		return nil, fmt.Errorf("no sandbox with id %v exists", id)
	}

	return sandbox, nil
}

// LookupSandboxByName returns a sandbox given its full or partial name
func (s *InMemoryState) LookupSandboxByName(name string) (*sandbox.Sandbox, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	id, err := s.podNameIndex.Get(name)
	if err != nil {
		return nil, fmt.Errorf("could not resolve sandbox name %v: %v", name, err)
	}

	sandbox, ok := s.sandboxes[id]
	if !ok {
		// This should never happen
		return nil, fmt.Errorf("cannot find sandbox %v in sandboxes map", id)
	}

	return sandbox, nil
}

// LookupSandboxByID returns a sandbox given its full or partial ID
// An error will be returned if the partial ID given is not unique
func (s *InMemoryState) LookupSandboxByID(id string) (*sandbox.Sandbox, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	fullID, err := s.podIDIndex.Get(id)
	if err != nil {
		return nil, fmt.Errorf("could not resolve sandbox id %v: %v", id, err)
	}

	sandbox, ok := s.sandboxes[fullID]
	if !ok {
		// This should never happen
		return nil, fmt.Errorf("cannot find sandbox %v in sandboxes map", fullID)
	}

	return sandbox, nil
}

// GetAllSandboxes returns all sandboxes in the state
func (s *InMemoryState) GetAllSandboxes() ([]*sandbox.Sandbox, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	sandboxes := make([]*sandbox.Sandbox, 0, len(s.sandboxes))
	for _, sb := range s.sandboxes {
		sandboxes = append(sandboxes, sb)
	}

	return sandboxes, nil
}

// AddContainer adds a single container to a given sandbox in the state
func (s *InMemoryState) AddContainer(c *oci.Container, sandboxID string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if c.Sandbox() != sandboxID {
		return fmt.Errorf("cannot add container to sandbox %v as it is part of sandbox %v", sandboxID, c.Sandbox())
	}

	sandbox, ok := s.sandboxes[sandboxID]
	if !ok {
		return fmt.Errorf("sandbox with ID %v does not exist, cannot add container", sandboxID)
	}

	if ctr := sandbox.GetContainer(c.ID()); ctr != nil {
		return fmt.Errorf("container with ID %v already exists in sandbox %v", c.ID(), sandboxID)
	}

	sandbox.AddContainer(c)

	return s.addContainerMappings(c, true)
}

// Add container ID, Name and Sandbox mappings
func (s *InMemoryState) addContainerMappings(c *oci.Container, addToContainers bool) error {
	if addToContainers && s.containers.Get(c.ID()) != nil {
		return fmt.Errorf("container with ID %v already exists in containers store", c.ID())
	}

	// TODO: if not a pod infra container, check if it conflicts with existing sandbox ID?
	// Does this matter?

	if addToContainers {
		s.containers.Add(c.ID(), c)
	}
	if err := s.ctrNameIndex.Reserve(c.Name(), c.ID()); err != nil {
		s.containers.Delete(c.ID())
		return fmt.Errorf("error registering container name: %v", err)
	}
	if err := s.ctrIDIndex.Add(c.ID()); err != nil {
		s.containers.Delete(c.ID())
		s.ctrNameIndex.Release(c.ID())
		return fmt.Errorf("error registering container ID: %v", err)
	}

	return nil
}

// HasContainer checks if a container with the given ID exists in a given sandbox
func (s *InMemoryState) HasContainer(id, sandboxID string) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	sandbox, ok := s.sandboxes[sandboxID]
	if !ok {
		return false
	}

	ctr := sandbox.GetContainer(id)

	return ctr != nil
}

// DeleteContainer removes the container with given ID from the given sandbox
func (s *InMemoryState) DeleteContainer(id, sandboxID string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	sandbox, ok := s.sandboxes[sandboxID]
	if !ok {
		return fmt.Errorf("sandbox with ID %v does not exist", sandboxID)
	}

	ctr := sandbox.GetContainer(id)
	if ctr == nil {
		return fmt.Errorf("sandbox %v has no container with ID %v", sandboxID, id)
	}

	sandbox.RemoveContainer(id)

	return s.deleteContainerMappings(ctr, true)
}

// Deletes container from the ID and Name mappings and optionally from the global containers list
func (s *InMemoryState) deleteContainerMappings(ctr *oci.Container, deleteFromContainers bool) error {
	if deleteFromContainers && s.containers.Get(ctr.ID()) == nil {
		return fmt.Errorf("container ID %v does not exist in containers store", ctr.ID())
	}

	if deleteFromContainers {
		s.containers.Delete(ctr.ID())
	}
	s.ctrNameIndex.Release(ctr.Name())
	if err := s.ctrIDIndex.Delete(ctr.ID()); err != nil {
		return fmt.Errorf("error unregistering container ID: %v", err)
	}

	return nil
}

// GetContainer returns the container with given ID in the given sandbox
func (s *InMemoryState) GetContainer(id, sandboxID string) (*oci.Container, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.getContainerFromSandbox(id, sandboxID)
}

// GetContainerSandbox returns the ID of a container's sandbox from the full container ID
// May not find the ID of pod infrastructure containers
func (s *InMemoryState) GetContainerSandbox(id string) (string, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	ctr := s.containers.Get(id)
	if ctr == nil {
		return "", fmt.Errorf("no container with ID %v found", id)
	}

	return ctr.Sandbox(), nil
}

// LookupContainerByName returns the full ID of a container given its full or partial name
func (s *InMemoryState) LookupContainerByName(name string) (*oci.Container, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	fullID, err := s.ctrNameIndex.Get(name)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve container name %v: %v", name, err)
	}

	return s.getContainer(fullID)
}

// LookupContainerByID returns the full ID of a container given a full or partial ID
// If the given ID is not unique, an error is returned
func (s *InMemoryState) LookupContainerByID(id string) (*oci.Container, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	fullID, err := s.ctrIDIndex.Get(id)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve container ID %v: %v", id, err)
	}

	return s.getContainer(fullID)
}

// GetAllContainers returns all containers in the state, regardless of which sandbox they belong to
// Pod Infra containers are not included
func (s *InMemoryState) GetAllContainers() ([]*oci.Container, error) {
	return s.containers.List(), nil
}

// Returns a single container from any sandbox based on full ID
// TODO: is it worth making this public as an alternative to GetContainer
func (s *InMemoryState) getContainer(id string) (*oci.Container, error) {
	ctr := s.containers.Get(id)
	if ctr == nil {
		return nil, fmt.Errorf("cannot find container with ID %v", id)
	}

	return s.getContainerFromSandbox(id, ctr.Sandbox())
}

// Returns a single container from a sandbox based on its full ID
// Internal implementation of GetContainer() but does not lock so it can be used in other functions
func (s *InMemoryState) getContainerFromSandbox(id, sandboxID string) (*oci.Container, error) {
	sandbox, ok := s.sandboxes[sandboxID]
	if !ok {
		return nil, fmt.Errorf("sandbox with ID %v does not exist", sandboxID)
	}

	ctr := sandbox.GetContainer(id)
	if ctr == nil {
		return nil, fmt.Errorf("cannot find container %v in sandbox %v", id, sandboxID)
	}

	return ctr, nil
}
