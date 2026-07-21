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

	// runtimePulledRuntimeService instances mapped to sandbox ID
	runtimeServiceRP     map[string]RuntimeServer
	runtimeServiceRPLock sync.RWMutex
}

func (r *RuntimeServiceManager) GetRuntimeService(sb SandboxInfo) RuntimeServer {
	if sb == nil {
		return r.runtimeService
	}

	rt, ok := r.serverConfig.Runtimes[sb.RuntimeHandler()]
	if !ok || !rt.RuntimePullImage {
		return r.runtimeService
	}

	id := sb.ID()

	r.runtimeServiceRPLock.RLock()
	rs := r.runtimeServiceRP[id]
	r.runtimeServiceRPLock.RUnlock()

	if rs != nil {
		return rs
	}

	is := r.imageServiceMgr.GetImageService(sb)

	// GetRuntimeService() is called first at sandbox creation,
	// at a time where the sandbox creation is not complete, and we
	// don't have a root dir to work with.
	// The ImageServer we get in this case is the default one, which
	// can be used for this early sandbox creation step, but should
	// not be cached.
	// The next call will get the proper runtimePulledImageService
	// that we can use to create and cache our runtimePulledRuntimeService.
	if _, ok = is.(*runtimePulledImageService); !ok {
		return r.runtimeService
	}

	rs = GetRuntimePulledRuntimeService(r.ctx, r.runtimeService, is)

	r.runtimeServiceRPLock.Lock()
	defer r.runtimeServiceRPLock.Unlock()

	// double-check that an instance was not created in parallel
	if existing := r.runtimeServiceRP[id]; existing != nil {
		return existing
	}

	r.runtimeServiceRP[id] = rs

	return rs
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
// given sandbox ID, freeing the associated in-memory state.
func (m *RuntimeServiceManager) RemoveRuntimeService(sandboxID string) {
	m.runtimeServiceRPLock.Lock()
	delete(m.runtimeServiceRP, sandboxID)
	m.runtimeServiceRPLock.Unlock()
}
