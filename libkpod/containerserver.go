package libkpod

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/types"
	cstorage "github.com/containers/storage"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/registrar"
	"github.com/docker/docker/pkg/truncindex"
	"github.com/kubernetes-incubator/cri-o/libkpod/sandbox"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/pkg/storage"
)

// ContainerServer implements the ImageServer
type ContainerServer struct {
	runtime            *oci.Runtime
	store              cstorage.Store
	storageImageServer storage.ImageServer
	ctrNameIndex       *registrar.Registrar
	ctrIDIndex         *truncindex.TruncIndex
	podNameIndex       *registrar.Registrar
	podIDIndex         *truncindex.TruncIndex

	imageContext *types.SystemContext
	stateLock    sync.Locker
	state        *containerServerState
}

// Runtime returns the oci runtime for the ContainerServer
func (c *ContainerServer) Runtime() *oci.Runtime {
	return c.runtime
}

// Store returns the Store for the ContainerServer
func (c *ContainerServer) Store() cstorage.Store {
	return c.store
}

// StorageImageServer returns the ImageServer for the ContainerServer
func (c *ContainerServer) StorageImageServer() storage.ImageServer {
	return c.storageImageServer
}

// CtrNameIndex returns the Registrar for the ContainerServer
func (c *ContainerServer) CtrNameIndex() *registrar.Registrar {
	return c.ctrNameIndex
}

// CtrIDIndex returns the TruncIndex for the ContainerServer
func (c *ContainerServer) CtrIDIndex() *truncindex.TruncIndex {
	return c.ctrIDIndex
}

// PodNameIndex returns the index of pod names
func (c *ContainerServer) PodNameIndex() *registrar.Registrar {
	return c.podNameIndex
}

// PodIDIndex returns the index of pod IDs
func (c *ContainerServer) PodIDIndex() *truncindex.TruncIndex {
	return c.podIDIndex
}

// ImageContext returns the SystemContext for the ContainerServer
func (c *ContainerServer) ImageContext() *types.SystemContext {
	return c.imageContext
}

// New creates a new ContainerServer with options provided
func New(runtime *oci.Runtime, store cstorage.Store, imageService storage.ImageServer, signaturePolicyPath string) *ContainerServer {
	return &ContainerServer{
		runtime:            runtime,
		store:              store,
		storageImageServer: imageService,
		ctrNameIndex:       registrar.NewRegistrar(),
		ctrIDIndex:         truncindex.NewTruncIndex([]string{}),
		podNameIndex:       registrar.NewRegistrar(),
		podIDIndex:         truncindex.NewTruncIndex([]string{}),
		imageContext:       &types.SystemContext{SignaturePolicyPath: signaturePolicyPath},
		stateLock:          new(sync.Mutex),
		state: &containerServerState{
			containers: oci.NewMemoryStore(),
			sandboxes:  make(map[string]*sandbox.Sandbox),
		},
	}
}

// ContainerStateFromDisk retrieves information on the state of a running container
// from the disk
func (c *ContainerServer) ContainerStateFromDisk(ctr *oci.Container) error {
	if err := ctr.FromDisk(); err != nil {
		return err
	}
	// ignore errors, this is a best effort to have up-to-date info about
	// a given container before its state gets stored
	c.runtime.UpdateStatus(ctr)

	return nil
}

// ContainerStateToDisk writes the container's state information to a JSON file
// on disk
func (c *ContainerServer) ContainerStateToDisk(ctr *oci.Container) error {
	// ignore errors, this is a best effort to have up-to-date info about
	// a given container before its state gets stored
	c.Runtime().UpdateStatus(ctr)

	jsonSource, err := ioutils.NewAtomicFileWriter(ctr.StatePath(), 0644)
	if err != nil {
		return err
	}
	defer jsonSource.Close()
	enc := json.NewEncoder(jsonSource)
	return enc.Encode(c.runtime.ContainerStatus(ctr))
}

// ReserveContainerName holds a name for a container that is being created
func (c *ContainerServer) ReserveContainerName(id, name string) (string, error) {
	if err := c.ctrNameIndex.Reserve(name, id); err != nil {
		if err == registrar.ErrNameReserved {
			id, err := c.ctrNameIndex.Get(name)
			if err != nil {
				logrus.Warnf("conflict, ctr name %q already reserved", name)
				return "", err
			}
			return "", fmt.Errorf("conflict, name %q already reserved for ctr %q", name, id)
		}
		return "", fmt.Errorf("error reserving ctr name %s", name)
	}
	return name, nil
}

// ReleaseContainerName releases a container name from the index so that it can
// be used by other containers
func (c *ContainerServer) ReleaseContainerName(name string) {
	c.ctrNameIndex.Release(name)
}

// ReservePodName holds a name for a pod that is being created
func (c *ContainerServer) ReservePodName(id, name string) (string, error) {
	if err := c.podNameIndex.Reserve(name, id); err != nil {
		if err == registrar.ErrNameReserved {
			id, err := c.podNameIndex.Get(name)
			if err != nil {
				logrus.Warnf("conflict, pod name %q already reserved", name)
				return "", err
			}
			return "", fmt.Errorf("conflict, name %q already reserved for pod %q", name, id)
		}
		return "", fmt.Errorf("error reserving pod name %q", name)
	}
	return name, nil
}

// ReleasePodName releases a pod name from the index so it can be used by other
// pods
func (c *ContainerServer) ReleasePodName(name string) {
	c.podNameIndex.Release(name)
}

// Shutdown attempts to shut down the server's storage cleanly
func (c *ContainerServer) Shutdown() error {
	_, err := c.store.Shutdown(false)
	return err
}

type containerServerState struct {
	containers oci.ContainerStorer
	sandboxes  map[string]*sandbox.Sandbox
}

// AddContainer adds a container to the container state store
func (c *ContainerServer) AddContainer(ctr *oci.Container) {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	sandbox := c.state.sandboxes[ctr.Sandbox()]
	sandbox.AddContainer(ctr)
	c.state.containers.Add(ctr.ID(), ctr)
}

// GetContainer returns a container by its ID
func (c *ContainerServer) GetContainer(id string) *oci.Container {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	return c.state.containers.Get(id)
}

// HasContainer checks if a container exists in the state
func (c *ContainerServer) HasContainer(id string) bool {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	ctr := c.state.containers.Get(id)
	return ctr != nil
}

// RemoveContainer removes a container from the container state store
func (c *ContainerServer) RemoveContainer(ctr *oci.Container) {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	sbID := ctr.Sandbox()
	sb := c.state.sandboxes[sbID]
	sb.RemoveContainer(ctr)
	c.state.containers.Delete(ctr.ID())
}

// ListContainers returns a list of all containers stored by the server state
func (c *ContainerServer) ListContainers() []*oci.Container {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	return c.state.containers.List()
}

// AddSandbox adds a sandbox to the sandbox state store
func (c *ContainerServer) AddSandbox(sb *sandbox.Sandbox) {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	c.state.sandboxes[sb.ID()] = sb
}

// GetSandbox returns a sandbox by its ID
func (c *ContainerServer) GetSandbox(id string) *sandbox.Sandbox {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	return c.state.sandboxes[id]
}

// GetSandboxContainer returns a sandbox's infra container
func (c *ContainerServer) GetSandboxContainer(id string) *oci.Container {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	sb, ok := c.state.sandboxes[id]
	if !ok {
		return nil
	}
	return sb.InfraContainer()
}

// HasSandbox checks if a sandbox exists in the state
func (c *ContainerServer) HasSandbox(id string) bool {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	_, ok := c.state.sandboxes[id]
	return ok
}

// RemoveSandbox removes a sandbox from the state store
func (c *ContainerServer) RemoveSandbox(id string) {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	delete(c.state.sandboxes, id)
}

// ListSandboxes lists all sandboxes in the state store
func (c *ContainerServer) ListSandboxes() []*sandbox.Sandbox {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	sbArray := make([]*sandbox.Sandbox, 0, len(c.state.sandboxes))
	for _, sb := range c.state.sandboxes {
		sbArray = append(sbArray, sb)
	}

	return sbArray
}
