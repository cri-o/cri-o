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

// New creates a new ContainerServer with options provided
func New(runtime *oci.Runtime, store cstorage.Store, imageService storage.ImageServer, signaturePolicyPath string) *ContainerServer {
	return &ContainerServer{
		runtime:            runtime,
		store:              store,
		storageImageServer: imageService,
		ctrNameIndex:       registrar.NewRegistrar(),
		ctrIDIndex:         truncindex.NewTruncIndex([]string{}),
		imageContext:       &types.SystemContext{SignaturePolicyPath: signaturePolicyPath},
		stateLock:          new(sync.Mutex),
		state: &containerServerState{
			containers: oci.NewMemoryStore(),
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

// Shutdown attempts to shut down the server's storage cleanly
func (c *ContainerServer) Shutdown() error {
	_, err := c.store.Shutdown(false)
	return err
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
