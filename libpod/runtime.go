package libpod

import (
	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/libpod/ctr"
	"github.com/kubernetes-incubator/cri-o/libpod/pod"
	spec "github.com/opencontainers/runtime-spec/specs-go"
)

// Runtime API

// A RuntimeOption is a functional option which alters the Runtime created by
// NewRuntime
type RuntimeOption func(*Runtime) error

// Runtime is the core libpod runtime
type Runtime struct {
	// TODO populate
}

// NewRuntime creates a new container runtime
func NewRuntime(options ...RuntimeOption) (*Runtime, error) {
	return nil, ctr.ErrNotImplemented
}

// Container API

// A CtrCreateOption is a functional option which alters the Container created
// by NewContainer
type CtrCreateOption func(*ctr.Container) error

// ContainerFilter is a function to determine whether a container is included
// in command output. Containers to be outputted are tested using the function.
// A true return will include the container, a false return will exclude it.
type ContainerFilter func(*ctr.Container) bool

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

// Pod API

// PodFilter is a function to determine whether a pod is included in command
// output. Pods to be outputted are tested using the function. A true return
// will include the pod, a false return will exclude it.
type PodFilter func(*pod.Pod) bool

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

// Image API

// ImageFilter is a function to determine whether an image is included in
// command output. Images to be outputted are tested using the function. A true
// return will include the image, a false return will exclude it.
type ImageFilter func(*storage.Image) bool

// PullImage pulls an image from configured registries
// By default, only the latest tag (or a specific tag if requested) will be
// pulled. If allTags is true, all tags for the requested image will be pulled.
// Signature validation will be performed if the Runtime has been appropriately
// configured
func (r *Runtime) PullImage(image string, allTags bool) (*storage.Image, error) {
	return nil, ctr.ErrNotImplemented
}

// PushImage pushes the given image to a location described by the given path
func (r *Runtime) PushImage(image *storage.Image, destination string) error {
	return ctr.ErrNotImplemented
}

// TagImage adds a tag to the given image
func (r *Runtime) TagImage(image *storage.Image, tag string) error {
	return ctr.ErrNotImplemented
}

// UntagImage removes a tag from the given image
func (r *Runtime) UntagImage(image *storage.Image, tag string) error {
	return ctr.ErrNotImplemented
}

// RemoveImage deletes an image from local storage
// Images being used by running containers cannot be removed
func (r *Runtime) RemoveImage(image *storage.Image) error {
	return ctr.ErrNotImplemented
}

// GetImage retrieves an image matching the given name or hash from system
// storage
// If no matching image can be found, an error is returned
func (r *Runtime) GetImage(image string) (*storage.Image, error) {
	return nil, ctr.ErrNotImplemented
}

// GetImages retrieves all images present in storage
// Filters can be provided which will determine which images are included in the
// output. Multiple filters are handled by ANDing their output, so only images
// matching all filters are included
func (r *Runtime) GetImages(filter ...ImageFilter) ([]*storage.Image, error) {
	return nil, ctr.ErrNotImplemented
}

// CommitContainer commits the changes between a container and its image,
// creating a new image
// If the container was not created from an image (for example,
// WithRootFSFromPath will create a container from a directory on the system),
// a new base image will be created from the contents of the container's
// filesystem
func (r *Runtime) CommitContainer(c *ctr.Container) (*storage.Image, error) {
	return nil, ctr.ErrNotImplemented
}
