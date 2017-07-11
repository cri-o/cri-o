package state

import (
	"fmt"
	"sync"

	"github.com/Sirupsen/logrus"
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
	containers   map[string]*oci.Container
	podNameIndex *registrar.Registrar
	podIDIndex   *truncindex.TruncIndex
	ctrNameIndex *registrar.Registrar
	ctrIDIndex   *truncindex.TruncIndex
}

// NewInMemoryState creates a new, empty server state
func NewInMemoryState() RuntimeStateStorer {
	state := new(InMemoryState)
	state.sandboxes = make(map[string]*sandbox.Sandbox)
	state.containers = make(map[string]*oci.Container)
	state.podNameIndex = registrar.NewRegistrar()
	state.podIDIndex = truncindex.NewTruncIndex([]string{})
	state.ctrNameIndex = registrar.NewRegistrar()
	state.ctrIDIndex = truncindex.NewTruncIndex([]string{})

	return state
}

// AddSandbox adds a sandbox and any containers in it to the state
func (s *InMemoryState) AddSandbox(sandbox *sandbox.Sandbox) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if sandbox == nil {
		return fmt.Errorf("nil passed as sandbox to AddSandbox")
	}

	// We shouldn't share ID with any containers
	// Our pod infra container will share our ID and we don't want it to conflict with anything
	// An equivalent check for sandbox IDs is done in addSandboxMappings()
	if _, exist := s.containers[sandbox.ID()]; exist {
		return fmt.Errorf("requested sandbox ID %v conflicts with existing container ID", sandbox.ID())
	}

	if err := s.addSandboxMappings(sandbox); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if err2 := s.deleteSandboxMappings(sandbox); err2 != nil {
				logrus.Errorf("Error removing mappings for incompletely-added sandbox %s: %v", sandbox.ID(), err2)
			}
		}
	}()

	// If there are containers in the sandbox add them to the mapping
	for _, ctr := range sandbox.Containers() {
		if err := s.addContainerMappings(ctr, true); err != nil {
			return fmt.Errorf("error adding container %v mappings in sandbox %v", ctr.ID(), sandbox.ID())
		}

		defer func(c *oci.Container) {
			if err != nil {
				if err2 := s.deleteContainerMappings(c, true); err2 != nil {
					logrus.Errorf("Error removing container %s mappings: %v", c.ID(), err2)
				}
			}
		}(ctr)
	}

	// Add the pod infrastructure container to mappings
	// TODO: Right now, we don't add it to the all containers listing. We may want to change this.
	if err := s.addContainerMappings(sandbox.InfraContainer(), false); err != nil {
		return fmt.Errorf("error adding infrastructure container %v to mappings: %v", sandbox.InfraContainer().ID(), err)
	}

	return nil
}

// Add sandbox name, ID to appropriate mappings
func (s *InMemoryState) addSandboxMappings(sb *sandbox.Sandbox) error {
	if _, exist := s.sandboxes[sb.ID()]; exist {
		return fmt.Errorf("sandbox with ID %s already exists in sandboxes map", sb.ID())
	}

	if err := s.podNameIndex.Reserve(sb.Name(), sb.ID()); err != nil {
		return fmt.Errorf("error registering sandbox name: %v", err)
	}
	if err := s.podIDIndex.Add(sb.ID()); err != nil {
		s.podNameIndex.Release(sb.Name())
		return fmt.Errorf("error registering sandbox ID: %v", err)
	}
	s.sandboxes[sb.ID()] = sb

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
func (s *InMemoryState) DeleteSandbox(id string) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, exist := s.sandboxes[id]; !exist {
		return &NoSuchSandboxError{id: id}
	}

	sb := s.sandboxes[id]

	if err := s.deleteSandboxMappings(sb); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			if err2 := s.addSandboxMappings(sb); err2 != nil {
				logrus.Errorf("Error re-adding sandbox mappings: %v", err2)
			}
		}
	}()

	// If there are containers left in the sandbox delete them from the mappings
	for _, ctr := range sb.Containers() {
		if err := s.deleteContainerMappings(ctr, true); err != nil {
			return fmt.Errorf("error removing container %v mappings: %v", ctr.ID(), err)
		}

		defer func(c *oci.Container) {
			if err != nil {
				if err2 := s.addContainerMappings(c, true); err2 != nil {
					logrus.Errorf("Error re-adding mappings for container %s: %v", c.ID(), err2)
				}
			}
		}(ctr)
	}

	// Delete infra container from mappings
	if err := s.deleteContainerMappings(sb.InfraContainer(), false); err != nil {
		return fmt.Errorf("error removing infra container %v from mappings: %v", sb.InfraContainer().ID(), err)
	}

	return nil
}

// Remove sandbox name, ID to appropriate mappings
func (s *InMemoryState) deleteSandboxMappings(sb *sandbox.Sandbox) error {
	if _, exist := s.sandboxes[sb.ID()]; !exist {
		return fmt.Errorf("sandbox with ID %s does not exist in sandboxes map", sb.ID())
	}

	if err := s.podIDIndex.Delete(sb.ID()); err != nil {
		return fmt.Errorf("error unregistering sandbox %s: %v", sb.ID(), err)
	}
	delete(s.sandboxes, sb.ID())
	s.podNameIndex.Release(sb.Name())

	return nil
}

// GetSandbox returns a sandbox given its full ID
func (s *InMemoryState) GetSandbox(id string) (*sandbox.Sandbox, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	sandbox, ok := s.sandboxes[id]
	if !ok {
		return nil, &NoSuchSandboxError{id: id}
	}

	return sandbox, nil
}

// LookupSandboxByName returns a sandbox given its full or partial name
func (s *InMemoryState) LookupSandboxByName(name string) (*sandbox.Sandbox, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	id, err := s.podNameIndex.Get(name)
	if err != nil {
		return nil, &NoSuchSandboxError{
			name:  name,
			inner: err,
		}
	}

	sandbox, ok := s.sandboxes[id]
	if !ok {
		// This should never happen - our internal state must be desynced
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
		return nil, &NoSuchSandboxError{
			id:    id,
			inner: err,
		}
	}

	sandbox, ok := s.sandboxes[fullID]
	if !ok {
		// This should never happen, internal state must be desynced
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
func (s *InMemoryState) AddContainer(c *oci.Container) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if c == nil {
		return fmt.Errorf("nil passed as container to AddContainer")
	}

	sandbox, ok := s.sandboxes[c.Sandbox()]
	if !ok {
		return &NoSuchSandboxError{id: c.Sandbox()}
	}

	if ctr := sandbox.GetContainer(c.ID()); ctr != nil {
		return fmt.Errorf("container with ID %v already exists in sandbox %v", c.ID(), c.Sandbox())
	}

	if sandbox.InfraContainer().ID() == c.ID() {
		return fmt.Errorf("container is infra container of sandbox %s, refusing to add to containers list", c.Sandbox())
	}

	if err := s.addContainerMappings(c, true); err != nil {
		return err
	}

	sandbox.AddContainer(c)

	return nil
}

// Add container ID, Name and Sandbox mappings
func (s *InMemoryState) addContainerMappings(c *oci.Container, addToContainers bool) error {
	if addToContainers {
		if _, exist := s.containers[c.ID()]; exist {
			return fmt.Errorf("container with ID %v already exists in containers store", c.ID())
		}
	}

	// TODO: if not a pod infra container, check if it conflicts with existing sandbox ID?
	// Does this matter?

	if err := s.ctrNameIndex.Reserve(c.Name(), c.ID()); err != nil {
		return fmt.Errorf("error registering name for container %s: %v", c.ID(), err)
	}

	if err := s.ctrIDIndex.Add(c.ID()); err != nil {
		s.ctrNameIndex.Release(c.ID())
		return fmt.Errorf("error registering ID for container %s: %v", c.ID(), err)
	}

	if addToContainers {
		s.containers[c.ID()] = c
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
		return &NoSuchSandboxError{id: sandboxID}
	}

	ctr := sandbox.GetContainer(id)
	if ctr == nil {
		return &NoSuchCtrError{
			id:      id,
			sandbox: id,
		}
	}

	if err := s.deleteContainerMappings(ctr, true); err != nil {
		return nil
	}

	sandbox.RemoveContainer(id)

	return nil
}

// Deletes container from the ID and Name mappings and optionally from the global containers list
func (s *InMemoryState) deleteContainerMappings(ctr *oci.Container, deleteFromContainers bool) error {
	if deleteFromContainers {
		if _, exist := s.containers[ctr.ID()]; !exist {
			return fmt.Errorf("container ID %v does not exist in containers store", ctr.ID())
		}
	}

	if err := s.ctrIDIndex.Delete(ctr.ID()); err != nil {
		return fmt.Errorf("error unregistering container ID for %s: %v", ctr.ID(), err)
	}

	s.ctrNameIndex.Release(ctr.Name())

	if deleteFromContainers {
		delete(s.containers, ctr.ID())
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

	ctr, exist := s.containers[id]
	if !exist {
		return "", &NoSuchCtrError{id: id}
	}

	return ctr.Sandbox(), nil
}

// LookupContainerByName returns the full ID of a container given its full or partial name
func (s *InMemoryState) LookupContainerByName(name string) (*oci.Container, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	fullID, err := s.ctrNameIndex.Get(name)
	if err != nil {
		return nil, &NoSuchCtrError{
			name:  name,
			inner: err,
		}
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
		return nil, &NoSuchCtrError{
			id:    id,
			inner: err,
		}
	}

	return s.getContainer(fullID)
}

// GetAllContainers returns all containers in the state, regardless of which sandbox they belong to
// Pod Infra containers are not included
func (s *InMemoryState) GetAllContainers() ([]*oci.Container, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	containers := make([]*oci.Container, 0, len(s.containers))
	for _, ctr := range s.containers {
		containers = append(containers, ctr)
	}

	return containers, nil
}

// Returns a single container from any sandbox based on full ID
// TODO: is it worth making this public as an alternative to GetContainer
func (s *InMemoryState) getContainer(id string) (*oci.Container, error) {
	ctr, exist := s.containers[id]
	if !exist {
		return nil, &NoSuchCtrError{id: id}
	}

	return s.getContainerFromSandbox(id, ctr.Sandbox())
}

// Returns a single container from a sandbox based on its full ID
// Internal implementation of GetContainer() but does not lock so it can be used in other functions
func (s *InMemoryState) getContainerFromSandbox(id, sandboxID string) (*oci.Container, error) {
	sandbox, ok := s.sandboxes[sandboxID]
	if !ok {
		return nil, &NoSuchSandboxError{id: sandboxID}
	}

	ctr := sandbox.GetContainer(id)
	if ctr == nil {
		return nil, &NoSuchCtrError{
			id:      id,
			sandbox: sandboxID,
		}
	}

	return ctr, nil
}
