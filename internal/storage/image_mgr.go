package storage

import (
	"context"
	"errors"
	"reflect"

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
	// Add other methods as needed
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
}

func (i *ImageServiceManager) GetImageService(sb SandboxInfo) ImageServer {
	if sb != nil && !reflect.ValueOf(sb).IsNil() {
		r, ok := i.serverConfig.Runtimes[sb.RuntimeHandler()]
		if ok && r.RuntimePullImage {
			is := i.imageServiceRP[sb.ID()]
			if is == nil {
				regularIS, ok := i.imageService.(*imageService)
				if !ok {
					// this *really* should never happen
					log.Warnf(i.ctx, "Failed to get an imageService")
				}

				var err error
				is, err = GetRuntimePulledImageService(i.ctx, regularIS)
				if err != nil {
					// if we can't get the specific image server, return the default
					log.Warnf(i.ctx, "Failed to get ImageServiceVM for sandbox %s: %v", sb.ID(), err)

					return i.imageService
				}

				i.imageServiceRP[sb.ID()] = is
			}

			return is
		}
	}

	return i.imageService
}

// DeleteImage deletes the image with the given ID from all storage backends.
// This is needed as there is no way, most of the time, to know which runtime
// is handling the image, and kubernetes expects the image to exist in a single
// store anyway.
func (i *ImageServiceManager) DeleteImage(systemContext *types.SystemContext, id StorageImageID) error {
	err := i.imageService.DeleteImage(systemContext, id)

	ok := (err == nil)

	for index := range i.imageServiceRP {
		err = i.imageServiceRP[index].DeleteImage(systemContext, id)
		if !ok {
			ok = (err == nil)
		}
	}

	if ok {
		// the image was successfully removed from one of the stores
		// consider it a success
		return nil
	}
	// ignore any error from the "runtimePulled" store, as it is expected to not
	// have images there most of the time
	return err
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
