package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"go.podman.io/image/v5/types"
	"go.podman.io/storage"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/pkg/config"
)

// SandboxInfo provides the minimal interface for accessing sandbox information
// needed by the ImageServiceManager.
type SandboxInfo interface {
	RuntimeHandler() string
}

// The ImageServiceManager object is responsible for maintaining different
// implementations of the ImageServer interface.
// It allows for easy switching between different image storage backends
// depending on the configuration or environment.
type ImageServiceManager struct {
	ctx          context.Context
	serverConfig *config.Config
	imageService ImageServer

	// runtimePulledImageService instances mapped to the runtime handler using them
	imageServiceRP map[string]*runtimePulledImageService

	// imageServiceRPLock protects concurrent access to imageServiceRP
	imageServiceRPLock sync.RWMutex
}

// GetImageService returns the ImageServer to be used by the given runtime
// handler. Runtimes configured with runtime_pull_image get their own
// runtimePulledImageService, every other handler (and an empty handler, which
// means "no sandbox context") uses the main image service.
//
// Note: unlike upstream, the runtime-pulled services are keyed by the runtime
// handler instead of by the sandbox, as release-1.35 has no per-sandbox
// plumbing to rely on.
func (i *ImageServiceManager) GetImageService(runtimeHandler string) (ImageServer, error) {
	if runtimeHandler == "" {
		return i.imageService, nil
	}

	r, ok := i.serverConfig.Runtimes[runtimeHandler]
	if !ok || !r.RuntimePullImage {
		return i.imageService, nil
	}

	i.imageServiceRPLock.RLock()
	is := i.imageServiceRP[runtimeHandler]
	i.imageServiceRPLock.RUnlock()

	if is != nil {
		return is, nil
	}

	rootDir, err := i.runtimePulledImageServiceRootDir(runtimeHandler)
	if err != nil {
		// Without a location to store the runtime-pulled images, we can't
		// create the specific ImageServer for this runtime handler, and have
		// to fall back to the main one.
		log.Warnf(i.ctx, "Failed to retrieve root dir for runtime handler %s: %v", runtimeHandler, err)

		return i.imageService, nil
	}

	regularIS, ok := i.imageService.(*imageService)
	if !ok {
		log.Warnf(i.ctx, "Failed to get an imageService")

		return i.imageService, nil
	}

	is, err = GetRuntimePulledImageService(i.ctx, regularIS, rootDir)
	if err != nil {
		// if we can't get the specific image server, return an error
		return nil, fmt.Errorf("failed to get ImageServiceVM for runtime handler %s: %w", runtimeHandler, err)
	}

	i.imageServiceRPLock.Lock()
	defer i.imageServiceRPLock.Unlock()

	// double-check that an instance was not created in parallel
	if existing := i.imageServiceRP[runtimeHandler]; existing != nil {
		return existing, nil
	}

	i.imageServiceRP[runtimeHandler] = is

	return is, nil
}

// runtimePulledImageServiceRootDir returns the directory holding the OCI
// artifact store used by the runtime-pulled image service of the given runtime
// handler.
//
// Upstream keys the service, and therefore its artifact store, per sandbox, and
// uses the sandbox' storage run directory. The release-1.35 model keeps a single
// service per runtime handler instead, so a dedicated directory below the
// storage run root is used. It lives as long as the rest of the runtime state,
// which is required to restore the list of images the runtime already pulled
// after a CRI-O restart.
func (i *ImageServiceManager) runtimePulledImageServiceRootDir(runtimeHandler string) (string, error) {
	store := i.imageService.GetStore()
	if store == nil {
		return "", errors.New("the image service has no storage")
	}

	runRoot := store.RunRoot()
	if runRoot == "" {
		runRoot = i.serverConfig.RunRoot
	}

	if runRoot == "" {
		return "", errors.New("no run root configured")
	}

	return filepath.Join(runRoot, "crio-runtime-pulled", runtimeHandler), nil
}

// DeleteImage deletes the image with the given ID from all storage backends.
// This is needed as there is no way, most of the time, to know which runtime
// is handling the image, and kubernetes expects the image to exist in a single
// store anyway.
func (i *ImageServiceManager) DeleteImage(ctx context.Context, systemContext *types.SystemContext, id StorageImageID) error {
	err := i.imageService.DeleteImage(systemContext, id)
	if err != nil {
		log.Debugf(ctx, "Failed to delete image %s from main store: %v", id, err)
	}

	ok := (err == nil)

	i.imageServiceRPLock.RLock()
	defer i.imageServiceRPLock.RUnlock()

	for index := range i.imageServiceRP {
		e := i.imageServiceRP[index].DeleteImage(systemContext, id)
		if e != nil {
			log.Debugf(ctx, "Failed to delete image %s from runtime-pulled store %s: %v", id, index, e)
		}

		if !ok {
			ok = (e == nil)
		}
	}

	if ok {
		// the image was successfully removed from one of the stores
		// consider it a success.
		return nil
	}
	// When an error occurred on both default and runtime-pulled stores,
	// we report the error from the default one, as we expect this is the most
	// used and relevant for troubleshooting.
	return err
}

// IDPrefixMatch pairs a resolved image ID with the ImageServer that holds it.
type IDPrefixMatch struct {
	ID     *StorageImageID
	Server ImageServer
}

// HeuristicallyTryResolvingStringAsIDPrefix calls the same function on each
// image store and returns all matches, each paired with the ImageServer it was
// found in, allowing direct calls to the right store.
func (i *ImageServiceManager) HeuristicallyTryResolvingStringAsIDPrefix(heuristicInput string) []IDPrefixMatch {
	var matches []IDPrefixMatch

	if id := i.imageService.HeuristicallyTryResolvingStringAsIDPrefix(heuristicInput); id != nil {
		matches = append(matches, IDPrefixMatch{ID: id, Server: i.imageService})
	}

	i.imageServiceRPLock.RLock()
	defer i.imageServiceRPLock.RUnlock()

	for index := range i.imageServiceRP {
		if id := i.imageServiceRP[index].HeuristicallyTryResolvingStringAsIDPrefix(heuristicInput); id != nil {
			matches = append(matches, IDPrefixMatch{ID: id, Server: i.imageServiceRP[index]})
		}
	}

	return matches
}

func GetImageServiceManager(ctx context.Context, store storage.Store, storageTransport StorageTransport, serverConfig *config.Config) (*ImageServiceManager, error) {
	is, err := GetImageService(ctx, store, storageTransport, serverConfig)
	if err != nil {
		return nil, err
	}

	imgSvc, ok := is.(*imageService)
	if !ok {
		return nil, errors.New("failed to assert imageService type")
	}

	return &ImageServiceManager{
		ctx:            ctx,
		serverConfig:   serverConfig,
		imageService:   imgSvc,
		imageServiceRP: make(map[string]*runtimePulledImageService),
	}, nil
}

func (m *ImageServiceManager) SetStorageImageServer(server ImageServer) {
	m.imageService = server
}

// RemoveImageService removes the cached runtimePulledImageService for the
// given runtime handler, freeing the associated in-memory state.
func (m *ImageServiceManager) RemoveImageService(runtimeHandler string) {
	m.imageServiceRPLock.Lock()
	delete(m.imageServiceRP, runtimeHandler)
	m.imageServiceRPLock.Unlock()
}
