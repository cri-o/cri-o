package storage

import (
	"context"

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
	imageServiceVM ImageServer
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
		return i.imageServiceVM
	}

	return i.imageService
}

// DeleteImage deletes the image with the given ID from all storage backends.
// This is needed as there is no way, most of the time, to know which runtime
// is handling the image, and kubernetes expects the image to exist in a single
// store anyway.
func (i *ImageServiceManager) DeleteImage(systemContext *types.SystemContext, id StorageImageID) error {
	err := i.imageService.DeleteImage(systemContext, id)
	errVM := i.imageServiceVM.DeleteImage(systemContext, id)

	if err == nil || errVM == nil {
		// the image was successfully removed from one of the stores
		// consider it a success
		return nil
	}
	// ignore any error from the "VM" store, as it is expected to not have images
	// there most of the time
	return err
}

func GetImageServiceManager(ctx context.Context, store storage.Store, storageTransport StorageTransport, serverConfig *config.Config) (*ImageServiceManager, error) {
	is, err := GetImageService(ctx, store, storageTransport, serverConfig)
	if err != nil {
		return nil, err
	}

	is_vm, err := GetImageServiceVM(ctx, is.(*imageService))
	if err != nil {
		return nil, err
	}

	return &ImageServiceManager{
		serverConfig:   serverConfig,
		imageService:   is.(*imageService),
		imageServiceVM: is_vm.(*imageServiceVM),
	}, nil
}

func (m *ImageServiceManager) SetStorageImageServer(server ImageServer) {
	m.imageService = server
}
