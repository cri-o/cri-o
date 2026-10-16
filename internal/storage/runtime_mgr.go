package storage

import (
	"context"
	"errors"
	"sync"

	"github.com/cri-o/cri-o/pkg/config"
)

// The runtimeServiceManager object is responsible for maintaining different
// instances of runtimeService.
// It allows for easy switching between different runtime services using different
// image service managers in the backend.
type RuntimeServiceManager struct {
	serverConfig    *config.Config
	runtimeService  RuntimeServer
	imageServiceMgr *ImageServiceManager
	ctx             context.Context

	// runtimePulledRuntimeService instances mapped to the runtime handler using them
	runtimeServiceRP     map[string]RuntimeServer
	runtimeServiceRPLock sync.RWMutex
}

// GetRuntimeService returns the RuntimeServer to be used by the given runtime
// handler. Runtimes configured with runtime_pull_image get their own
// runtimePulledRuntimeService, every other handler (and an empty handler, which
// means "no sandbox context") uses the main runtime service.
//
// Note: unlike upstream, the runtime-pulled services are keyed by the runtime
// handler instead of by the sandbox, as release-1.35 has no per-sandbox
// plumbing to rely on.
func (r *RuntimeServiceManager) GetRuntimeService(runtimeHandler string) (RuntimeServer, error) {
	if runtimeHandler == "" {
		return r.runtimeService, nil
	}

	rt, ok := r.serverConfig.Runtimes[runtimeHandler]
	if !ok || !rt.RuntimePullImage {
		return r.runtimeService, nil
	}

	r.runtimeServiceRPLock.RLock()
	rs := r.runtimeServiceRP[runtimeHandler]
	r.runtimeServiceRPLock.RUnlock()

	if rs != nil {
		return rs, nil
	}

	is, err := r.imageServiceMgr.GetImageService(runtimeHandler)
	if err != nil {
		return nil, err
	}

	// The ImageServer we get here may still be the default one, which can be
	// used for this call, but should not be cached: the next call may get the
	// proper runtimePulledImageService that we need to create and cache our
	// runtimePulledRuntimeService.
	if _, ok = is.(*runtimePulledImageService); !ok {
		return r.runtimeService, nil
	}

	rs = GetRuntimePulledRuntimeService(r.ctx, r.runtimeService, is)

	r.runtimeServiceRPLock.Lock()
	defer r.runtimeServiceRPLock.Unlock()

	// double-check that an instance was not created in parallel
	if existing := r.runtimeServiceRP[runtimeHandler]; existing != nil {
		return existing, nil
	}

	r.runtimeServiceRP[runtimeHandler] = rs

	return rs, nil
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
		runtimeServiceRP: make(map[string]RuntimeServer),
	}, nil
}

func (m *RuntimeServiceManager) SetStorageRuntimeServer(server RuntimeServer) {
	m.runtimeService = server
}

// RemoveRuntimeService removes the cached runtimePulledRuntimeService for the
// given runtime handler, freeing the associated in-memory state.
func (m *RuntimeServiceManager) RemoveRuntimeService(runtimeHandler string) {
	m.runtimeServiceRPLock.Lock()
	delete(m.runtimeServiceRP, runtimeHandler)
	m.runtimeServiceRPLock.Unlock()
}
