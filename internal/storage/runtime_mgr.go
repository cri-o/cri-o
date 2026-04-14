package storage

import (
	"context"
	"errors"

	"github.com/cri-o/cri-o/pkg/config"
)

// The runtimeServiceManager object is responsible for maintaining different
// instances of runtimeService.
// It allows for easy switching between different runtime services using different
// image service managers in the backend.
type RuntimeServiceManager struct {
	serverConfig     *config.Config
	runtimeService   RuntimeServer
	runtimeServiceRP RuntimeServer
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
		return r.runtimeServiceRP
	}

	return r.runtimeService
}

func GetRuntimeServiceManager(ctx context.Context, imageServiceMgr *ImageServiceManager, storageTransport StorageTransport, serverConfig *config.Config) (*RuntimeServiceManager, error) {
	rs := GetRuntimeService(ctx, imageServiceMgr.imageService, storageTransport)

	runtimeSvc, ok := rs.(*runtimeService)
	if !ok {
		return nil, errors.New("failed to assert runtimeService type")
	}

	imgSvcRP, ok := imageServiceMgr.imageServiceRP.(*runtimePulledImageService)
	if !ok {
		return nil, errors.New("failed to assert runtimePulledImageService type")
	}

	rs_rp := GetRuntimePulledRuntimeService(ctx, rs, imgSvcRP, storageTransport)

	runtimeSvcRP, ok := rs_rp.(*runtimePulledRuntimeService)
	if !ok {
		return nil, errors.New("failed to assert runtimePulledRuntimeService type")
	}

	return &RuntimeServiceManager{
		serverConfig:     serverConfig,
		runtimeService:   runtimeSvc,
		runtimeServiceRP: runtimeSvcRP,
	}, nil
}

func (m *RuntimeServiceManager) SetStorageRuntimeServer(server RuntimeServer) {
	m.runtimeService = server
}
