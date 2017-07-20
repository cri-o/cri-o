package libkpod

import (
	"sync"

	"github.com/containers/image/types"
	cstorage "github.com/containers/storage"
	"github.com/docker/docker/pkg/registrar"
	"github.com/docker/docker/pkg/truncindex"
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
	imageContext       *types.SystemContext
	stateLock          sync.Locker
	state              *containerServerState
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

// ImageContext returns the SystemContext for the ContainerServer
func (c *ContainerServer) ImageContext() *types.SystemContext {
	return c.imageContext
}

// New creates a new ContainerServer
func New(runtime *oci.Runtime, store cstorage.Store, storageImageServer storage.ImageServer, ctrNameIndex *registrar.Registrar, ctrIDIndex *truncindex.TruncIndex, imageContext *types.SystemContext) *ContainerServer {
	containers := oci.NewMemoryStore()
	return &ContainerServer{
		runtime:            runtime,
		store:              store,
		storageImageServer: storageImageServer,
		ctrNameIndex:       ctrNameIndex,
		ctrIDIndex:         ctrIDIndex,
		imageContext:       imageContext,
		stateLock:          new(sync.Mutex),
		state: &containerServerState{
			containers: containers,
		},
	}
}

type containerServerState struct {
	containers oci.ContainerStorer
}

// AddContainer adds a container to the container state store
func (c *ContainerServer) AddContainer(ctr *oci.Container) {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	c.state.containers.Add(ctr.ID(), ctr)
}

// GetContainer returns a container by its ID
func (c *ContainerServer) GetContainer(id string) *oci.Container {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	return c.state.containers.Get(id)
}

// RemoveContainer removes a container from the container state store
func (c *ContainerServer) RemoveContainer(ctr *oci.Container) {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	c.state.containers.Delete(ctr.ID())
}

// ListContainers returns a list of all containers stored by the server state
func (c *ContainerServer) ListContainers() []*oci.Container {
	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	return c.state.containers.List()
}
