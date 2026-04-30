package storage

import (
	"context"

	"github.com/cri-o/cri-o/pkg/config"
)

// The runtimeServiceManager object is responsible for maintaining different
// instances of runtimeService.
// It allows for easy switching between different runtime services using different
// image service managers in the backend.
type RuntimeServiceManager struct {
	serverConfig     *config.Config
	runtimeService   RuntimeServer
	runtimeServiceVM RuntimeServer
}

func (r *RuntimeServiceManager) GetRuntimeService(runtimeHandler string) RuntimeServer {
	isRuntimePullImage := false

	if runtimeHandler != "" {
		r, ok := r.serverConfig.Runtimes[runtimeHandler]
		if ok {
			isRuntimePullImage = r.RuntimePullImage
		}
	}

	if isRuntimePullImage {
		return r.runtimeServiceVM
	}

	return r.runtimeService
}

func GetRuntimeServiceManager(ctx context.Context, imageServiceMgr *ImageServiceManager, storageTransport StorageTransport, serverConfig *config.Config) *RuntimeServiceManager {
	rs := GetRuntimeService(ctx, imageServiceMgr.imageService, storageTransport)
	rs_vm := GetRuntimeServiceVM(ctx, rs, imageServiceMgr.imageServiceVM.(*imageServiceVM), storageTransport)

	return &RuntimeServiceManager{
		serverConfig:     serverConfig,
		runtimeService:   rs.(*runtimeService),
		runtimeServiceVM: rs_vm.(*runtimeServiceVM),
	}
}

func (m *RuntimeServiceManager) SetStorageRuntimeServer(server RuntimeServer) {
	m.runtimeService = server
}
