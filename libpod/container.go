package libpod

import (
	"fmt"
	"sync"

	"github.com/containers/storage"
	"github.com/docker/docker/pkg/stringid"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/ulule/deepcopier"
)

var (
	// ErrNotImplemented indicates that functionality is not yet implemented
	errNotImplemented = fmt.Errorf("NOT IMPLEMENTED")
)

// Container is a single OCI container
type Container struct {
	id   string
	name string

	spec *spec.Spec
	pod  *Pod

	valid bool
	lock  sync.RWMutex
}

// ID returns the container's ID
func (c *Container) ID() string {
	// No locking needed, ID will never mutate after a container is created
	return c.id
}

// Name returns the container's name
func (c *Container) Name() string {
	// Name can potentially be changed while a container is running
	// So lock access to it
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.name
}

// Spec returns the container's OCI runtime spec
func (c *Container) Spec() *spec.Spec {
	// The spec can potentially be altered when storage is configured and to
	// add annotations at container create time
	// As such, access to it is locked
	c.lock.RLock()
	defer c.lock.RUnlock()

	spec := new(spec.Spec)
	deepcopier.Copy(c.spec).To(spec)

	return spec
}

// Make a new container
func newContainer(rspec *spec.Spec) (*Container, error) {
	if rspec == nil {
		return nil, fmt.Errorf("must provide a valid spec to construct container")
	}

	ctr := new(Container)
	ctr.id = stringid.GenerateNonCryptoID()
	ctr.name = ctr.id // TODO generate unique human-readable names

	ctr.spec = new(spec.Spec)
	deepcopier.Copy(rspec).To(ctr.spec)

	return ctr, nil
}

// Create creates a container in the OCI runtime
func (c *Container) Create() error {
	return errNotImplemented
}

// Start starts a container
func (c *Container) Start() error {
	return errNotImplemented
}

// Stop stops a container
func (c *Container) Stop() error {
	return errNotImplemented
}

// Kill sends a signal to a container
func (c *Container) Kill(signal uint) error {
	return errNotImplemented
}

// Exec starts a new process inside the container
// Returns fully qualified URL of streaming server for executed process
func (c *Container) Exec(cmd []string, tty bool, stdin bool) (string, error) {
	return "", errNotImplemented
}

// Attach attaches to a container
// Returns fully qualified URL of streaming server for the container
func (c *Container) Attach(stdin, tty bool) (string, error) {
	return "", errNotImplemented
}

// Mount mounts a container's filesystem on the host
// The path where the container has been mounted is returned
func (c *Container) Mount() (string, error) {
	return "", errNotImplemented
}

// Status gets a container's status
// TODO this should return relevant information about container state
func (c *Container) Status() error {
	return errNotImplemented
}

// Export exports a container's root filesystem as a tar archive
// The archive will be saved as a file at the given path
func (c *Container) Export(path string) error {
	return errNotImplemented
}

// Commit commits the changes between a container and its image, creating a new
// image
// If the container was not created from an image (for example,
// WithRootFSFromPath will create a container from a directory on the system),
// a new base image will be created from the contents of the container's
// filesystem
func (c *Container) Commit() (*storage.Image, error) {
	return nil, errNotImplemented
}
