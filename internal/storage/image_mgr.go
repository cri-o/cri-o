package storage

import (
	"context"
	"errors"
	"reflect"
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
	ID() string
}

// The ImageServiceManager object is responsible for maintaining different
// implementations of the ImageServer interface.
// It allows for easy switching between different image storage backends
// depending on the configuration or environment.
type ImageServiceManager struct {
	ctx          context.Context
	serverConfig *config.Config
	imageService ImageServer

	// runtimePulledImageService instances mapped to the sandbox ID using them
	imageServiceRP map[string]*runtimePulledImageService

	// imageServiceRPLock protects concurrent access to imageServiceRP
	imageServiceRPLock sync.RWMutex
}

func (i *ImageServiceManager) GetImageService(sb SandboxInfo) ImageServer {
	v := reflect.ValueOf(sb)
	if sb == nil || v.Kind() != reflect.Pointer || v.IsNil() {
		return i.imageService
	}

	r, ok := i.serverConfig.Runtimes[sb.RuntimeHandler()]
	if !ok || !r.RuntimePullImage {
		return i.imageService
	}

	i.imageServiceRPLock.RLock()
	is := i.imageServiceRP[sb.ID()]
	i.imageServiceRPLock.RUnlock()

	if is != nil {
		return is
	}

	rootDir, err := i.imageService.GetStore().ContainerRunDirectory(sb.ID())
	if err != nil {
		// This will happen when the sandbox is being created.
		// After that, we will get the proper information allowing
		// to retrieve the sandbox and create the appropriate ImageServer
		// for it.
		log.Warnf(i.ctx, "Failed to retrieve root dir for sandbox %s: %v", sb.ID(), err)

		return i.imageService
	}

	regularIS, ok := i.imageService.(*imageService)
	if !ok {
		log.Warnf(i.ctx, "Failed to get an imageService")

		return i.imageService
	}

	is, err = GetRuntimePulledImageService(i.ctx, regularIS, rootDir)
	if err != nil {
		// if we can't get the specific image server, return the default
		log.Warnf(i.ctx, "Failed to get ImageServiceVM for sandbox %s: %v", sb.ID(), err)

		return i.imageService
	}

	i.imageServiceRPLock.Lock()
	defer i.imageServiceRPLock.Unlock()

	// double-check that an instance was not created in parallel
	if existing := i.imageServiceRP[sb.ID()]; existing != nil {
		return existing
	}

	i.imageServiceRP[sb.ID()] = is

	return is
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
// given sandbox ID, freeing the associated in-memory state.
func (m *ImageServiceManager) RemoveImageService(sandboxID string) {
	m.imageServiceRPLock.Lock()
	delete(m.imageServiceRP, sandboxID)
	m.imageServiceRPLock.Unlock()
}
