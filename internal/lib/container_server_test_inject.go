//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package lib

import (
	"github.com/cri-o/cri-o/internal/storage"
)

// SetStorageRuntimeServer sets the runtime server for the ContainerServer.
func (c *ContainerServer) SetStorageRuntimeServer(server storage.RuntimeServer) {
	c.storageRuntimeSvcMgr.SetStorageRuntimeServer(server)
}

// SetStorageImageServer sets the ImageServer for the ContainerServer.
func (c *ContainerServer) SetStorageImageServer(server storage.ImageServer) {
	c.storageImgSvcMgr.SetStorageImageServer(server)
}
