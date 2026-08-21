//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package lib

import (
	cstorage "go.podman.io/storage"

	"github.com/cri-o/cri-o/internal/storage"
)

// NewContainerServerForTest creates a minimal ContainerServer for unit tests.
func NewContainerServerForTest(store cstorage.Store) *ContainerServer {
	return &ContainerServer{
		store:                     store,
		mountOperationsInProgress: make(map[string]*mountOperation),
	}
}

// SetStorageRuntimeServer sets the runtime server for the ContainerServer.
func (c *ContainerServer) SetStorageRuntimeServer(server storage.RuntimeServer) {
	c.storageRuntimeServer = server
}

// SetStorageImageServer sets the ImageServer for the ContainerServer.
func (c *ContainerServer) SetStorageImageServer(server storage.ImageServer) {
	c.storageImageServer = server
}
