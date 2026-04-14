package storage

import (
	"context"
	"errors"

	"go.podman.io/image/v5/types"
	"go.podman.io/storage"

	"github.com/cri-o/cri-o/pkg/config"
)

// The ImageServiceManager object is responsible for maintaining different
// implementations of the ImageServer interface.
// It allows for easy switching between different image storage backends
// depending on the configuration or environment.
type ImageServiceManager struct {
	serverConfig   *config.Config
	imageService   ImageServer
	imageServiceRP ImageServer
}

func (i *ImageServiceManager) GetImageService(runtimeHandler string) ImageServer {
	isRuntimePullImage := false

	if runtimeHandler != "" {
		r, ok := i.serverConfig.Runtimes[runtimeHandler]
		if ok {
			isRuntimePullImage = r.RuntimePullImage
		}
	}

	if isRuntimePullImage {
		return i.imageServiceRP
	}

	return i.imageService
}

// DeleteImage deletes the image with the given ID from all storage backends.
// This is needed as there is no way, most of the time, to know which runtime
// is handling the image, and kubernetes expects the image to exist in a single
// store anyway.
func (i *ImageServiceManager) DeleteImage(systemContext *types.SystemContext, id StorageImageID) error {
	err := i.imageService.DeleteImage(systemContext, id)
	errRP := i.imageServiceRP.DeleteImage(systemContext, id)

	if err == nil || errRP == nil {
		// the image was successfully removed from one of the stores
		// consider it a success.
		return nil
	}
	// When an error occurred on both default and runtime-pulled stores,
	// we report the error from the default one, as we expect this is the most
	// used and relevant for troubleshooting.
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

	is_rp, err := GetRuntimePulledImageService(ctx, imgSvc)
	if err != nil {
		return nil, err
	}

	imgSvcRP, ok := is_rp.(*runtimePulledImageService)
	if !ok {
		return nil, errors.New("failed to assert runtimePulledImageService type")
	}

	return &ImageServiceManager{
		serverConfig:   serverConfig,
		imageService:   imgSvc,
		imageServiceRP: imgSvcRP,
	}, nil
}

func (m *ImageServiceManager) SetStorageImageServer(server ImageServer) {
	m.imageService = server
}
