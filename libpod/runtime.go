package libpod

import (
	"github.com/kubernetes-incubator/cri-o/libpod/ctr"
	"github.com/kubernetes-incubator/cri-o/libpod/pod"
	spec "github.com/opencontainers/runtime-spec/specs-go"
)

// ContainerFilter is a function to determine whether a container is included
// in command output. Containers to be outputted are tested using the function.
// A true return will include the container, a false return will exclude it.
type ContainerFilter func(*ctr.Container) bool

// PodFilter is a function to determine whether a pod is included in command
// output. Pods to be outputted are tested using the function. A true return
// will include the pod, a false return will exclude it.
type PodFilter func(*pod.Pod) bool

// A RuntimeOption is a functional option which alters the Runtime created by
// NewRuntime
type RuntimeOption func(*Runtime) error

// A CtrCreateOption is a functional option which alters the Container created
// by NewContainer
type CtrCreateOption func(*ctr.Container) error

// Runtime is the core libpod runtime
type Runtime struct {
	// TODO populate
}

// NewRuntime creates a new container runtime
func NewRuntime(options ...RuntimeOption) (*Runtime, error) {
	return nil, ctr.ErrNotImplemented
}

// NewContainer creates a new container from a given OCI config
func (r *Runtime) NewContainer(spec *spec.Spec, options ...CtrCreateOption) (*ctr.Container, error) {
	return nil, ctr.ErrNotImplemented
}

// RemoveContainer removes the given container
// If force is specified, the container will be stopped first
// Otherwise, RemoveContainer will return an error if the container is running
func (r *Runtime) RemoveContainer(c *ctr.Container, force bool) error {
	return ctr.ErrNotImplemented
}

// GetContainer retrieves a container by its ID
func (r *Runtime) GetContainer(id string) (*ctr.Container, error) {
	return nil, ctr.ErrNotImplemented
}

// LookupContainer looks up a container by its name or a partial ID
// If a partial ID is not unique, an error will be returned
func (r *Runtime) LookupContainer(idOrName string) (*ctr.Container, error) {
	return nil, ctr.ErrNotImplemented
}

// GetContainers retrieves all containers from the state
// Filters can be provided which will determine what containers are included in
// the output. Multiple filters are handled by ANDing their output, so only
// containers matching all filters are returned
func (r *Runtime) GetContainers(filters ...ContainerFilter) ([]*ctr.Container, error) {
	return nil, ctr.ErrNotImplemented
}

// NewPod makes a new, empty pod
func (r *Runtime) NewPod() (*pod.Pod, error) {
	return nil, ctr.ErrNotImplemented
}

// RemovePod removes a pod and all containers in it
// If force is specified, all containers in the pod will be stopped first
// Otherwise, RemovePod will return an error if any container in the pod is running
// Remove acts atomically, removing all containers or no containers
func (r *Runtime) RemovePod(p *pod.Pod, force bool) error {
	return ctr.ErrNotImplemented
}

// GetPod retrieves a pod by its ID
func (r *Runtime) GetPod(id string) (*pod.Pod, error) {
	return nil, ctr.ErrNotImplemented
}

// LookupPod retrieves a pod by its name or a partial ID
// If a partial ID is not unique, an error will be returned
func (r *Runtime) LookupPod(idOrName string) (*pod.Pod, error) {
	return nil, ctr.ErrNotImplemented
}

// GetPods retrieves all pods
// Filters can be provided which will determine which pods are included in the
// output. Multiple filters are handled by ANDing their output, so only pods
// matching all filters are returned
func (r *Runtime) GetPods(filters ...PodFilter) ([]*pod.Pod, error) {
	return nil, ctr.ErrNotImplemented
}

// TODO Add image API
