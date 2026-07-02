package storage

import (
	"context"
	"errors"
	"reflect"

	"github.com/cri-o/cri-o/pkg/config"
)

// The runtimeServiceManager object is responsible for maintaining different
// instances of runtimeService.
// It allows for easy switching between different runtime services using different
// image service managers in the backend.
type RuntimeServiceManager struct {
	serverConfig     *config.Config
	runtimeService   RuntimeServer
	imageServiceMgr  *ImageServiceManager
	ctx              context.Context
	storageTransport StorageTransport
}

func (r *RuntimeServiceManager) GetRuntimeService(sb SandboxInfo) RuntimeServer {
	if sb != nil && !reflect.ValueOf(sb).IsNil() {
		rt, ok := r.serverConfig.Runtimes[sb.RuntimeHandler()]
		if ok && rt.RuntimePullImage {
			is := r.imageServiceMgr.GetImageService(sb)

			return GetRuntimePulledRuntimeService(r.ctx, r.runtimeService, is, r.storageTransport)
		}
	}

	return r.runtimeService
}

func GetRuntimeServiceManager(ctx context.Context, imageServiceMgr *ImageServiceManager, storageTransport StorageTransport, serverConfig *config.Config) (*RuntimeServiceManager, error) {
	rs := GetRuntimeService(ctx, imageServiceMgr.imageService, storageTransport)

	runtimeSvc, ok := rs.(*runtimeService)
	if !ok {
		return nil, errors.New("failed to assert runtimeService type")
	}

	return &RuntimeServiceManager{
		serverConfig:     serverConfig,
		runtimeService:   runtimeSvc,
		imageServiceMgr:  imageServiceMgr,
		ctx:              ctx,
		storageTransport: storageTransport,
	}, nil
}

func (m *RuntimeServiceManager) SetStorageRuntimeServer(server RuntimeServer) {
	m.runtimeService = server
}
