package libpod

import (
	"github.com/containers/storage"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

// Contains the public Runtime API for containers

// A CtrCreateOption is a functional option which alters the Container created
// by NewContainer
type CtrCreateOption func(*Container) error

// ContainerFilter is a function to determine whether a container is included
// in command output. Containers to be outputted are tested using the function.
// A true return will include the container, a false return will exclude it.
type ContainerFilter func(*Container) bool

// NewContainer creates a new container from a given OCI config
func (r *Runtime) NewContainer(spec *spec.Spec, options ...CtrCreateOption) (*Container, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if !r.valid {
		return nil, ErrRuntimeStopped
	}

	ctr, err := newContainer(spec)
	if err != nil {
		return nil, err
	}

	for _, option := range options {
		if err := option(ctr); err != nil {
			return nil, errors.Wrapf(err, "error running container create option")
		}
	}

	ctr.valid = true

	if err := r.state.AddContainer(ctr); err != nil {
		// If we joined a pod, remove ourself from it
		if ctr.pod != nil {
			if err2 := ctr.pod.removeContainer(ctr); err2 != nil {
				return nil, errors.Wrapf(err, "error adding new container to state, container could not be removed from pod %s", ctr.pod.ID())
			}
		}

		// TODO: Might be worth making an effort to detect duplicate IDs
		// We can recover from that by generating a new ID for the
		// container
		return nil, errors.Wrapf(err, "error adding new container to state")
	}

	return ctr, nil
}

// RemoveContainer removes the given container
// If force is specified, the container will be stopped first
// Otherwise, RemoveContainer will return an error if the container is running
func (r *Runtime) RemoveContainer(c *Container, force bool) error {
	return ErrNotImplemented
}

// GetContainer retrieves a container by its ID
func (r *Runtime) GetContainer(id string) (*Container, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if !r.valid {
		return nil, ErrRuntimeStopped
	}

	return r.state.GetContainer(id)
}

// HasContainer checks if a container with the given ID is present
func (r *Runtime) HasContainer(id string) (bool, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if !r.valid {
		return false, ErrRuntimeStopped
	}

	return r.state.HasContainer(id)
}

// LookupContainer looks up a container by its name or a partial ID
// If a partial ID is not unique, an error will be returned
func (r *Runtime) LookupContainer(idOrName string) (*Container, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if !r.valid {
		return nil, ErrRuntimeStopped
	}

	return r.state.LookupContainer(idOrName)
}

// GetContainers retrieves all containers from the state
// Filters can be provided which will determine what containers are included in
// the output. Multiple filters are handled by ANDing their output, so only
// containers matching all filters are returned
func (r *Runtime) GetContainers(filters ...ContainerFilter) ([]*Container, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if !r.valid {
		return nil, ErrRuntimeStopped
	}

	ctrs, err := r.state.GetAllContainers()
	if err != nil {
		return nil, err
	}

	ctrsFiltered := make([]*Container, 0, len(ctrs))

	for _, ctr := range ctrs {
		include := true
		for _, filter := range filters {
			include = include && filter(ctr)
		}

		if include {
			ctrsFiltered = append(ctrsFiltered, ctr)
		}
	}

	return ctrsFiltered, nil
}

// getContainersWithImage returns a list of containers referencing imageID
func (r *Runtime) getContainersWithImage(imageID string) ([]storage.Container, error) {
	var matchingContainers []storage.Container
	containers, err := r.store.Containers()
	if err != nil {
		return nil, err
	}

	for _, ctr := range containers {
		if ctr.ImageID == imageID {
			matchingContainers = append(matchingContainers, ctr)
		}
	}
	return matchingContainers, nil
}

// removeMultipleContainers deletes a list of containers from the store
func (r *Runtime) removeMultipleContainers(containers []storage.Container) error {
	for _, ctr := range containers {
		if err := r.store.DeleteContainer(ctr.ID); err != nil {
			return errors.Wrapf(err, "could not remove container %q", ctr)
		}
	}
	return nil
}
