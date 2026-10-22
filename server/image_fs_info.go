package server

import (
	"context"
	"fmt"
	"path"
	"time"

	"go.podman.io/storage"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// layerUsage is the aggregated disk usage of a set of storage layers.
type layerUsage struct {
	bytes  uint64
	inodes uint64
}

// getStorageFsInfo reports the disk usage of the image and container
// filesystems by summing the actual content of every storage layer, matching
// how container runtimes account for image filesystem usage.
//
// Image layers and container writable layers are summed separately. When a
// dedicated image store is configured (store.ImageStore() != ""), image layers
// live under the image store and container writable layers under the graph
// root, so each kind is attributed to its own store location. Otherwise all
// layers share the graph root filesystem and their combined usage is reported
// for both.
func getStorageFsInfo(store storage.Store) (*types.ImageFsInfoResponse, error) {
	image, container, err := layerUsageByKind(store)
	if err != nil {
		return nil, err
	}

	storageDriver := store.GraphDriverName()
	imagePath := store.ImageStore()

	if imagePath == "" {
		total := layerUsage{
			bytes:  image.bytes + container.bytes,
			inodes: image.inodes + container.inodes,
		}
		usage := filesystemUsage(path.Join(store.GraphRoot(), storageDriver+"-images"), total)

		return &types.ImageFsInfoResponse{
			ImageFilesystems:     []*types.FilesystemUsage{usage},
			ContainerFilesystems: []*types.FilesystemUsage{usage},
		}, nil
	}

	return &types.ImageFsInfoResponse{
		ImageFilesystems: []*types.FilesystemUsage{
			filesystemUsage(path.Join(imagePath, storageDriver+"-images"), image),
		},
		ContainerFilesystems: []*types.FilesystemUsage{
			filesystemUsage(path.Join(store.GraphRoot(), storageDriver+"-containers"), container),
		},
	}, nil
}

// layerUsageByKind sums the disk usage of every storage layer, partitioning it
// into image layers and container writable layers. A layer is a container
// writable layer when it is the read-write layer of a known container.
func layerUsageByKind(store storage.Store) (image, container layerUsage, _ error) {
	layers, err := store.Layers()
	if err != nil {
		return layerUsage{}, layerUsage{}, fmt.Errorf("list layers: %w", err)
	}

	containers, err := store.Containers()
	if err != nil {
		return layerUsage{}, layerUsage{}, fmt.Errorf("list containers: %w", err)
	}

	driver, err := store.GraphDriver()
	if err != nil {
		return layerUsage{}, layerUsage{}, fmt.Errorf("get graph driver: %w", err)
	}

	containerLayers := make(map[string]struct{}, len(containers))
	for i := range containers {
		containerLayers[containers[i].LayerID] = struct{}{}
	}

	for i := range layers {
		layer := &layers[i]

		usage, err := driver.ReadWriteDiskUsage(layer.ID)
		if err != nil {
			return layerUsage{}, layerUsage{}, fmt.Errorf(
				"get disk usage for layer %s: %w",
				layer.ID,
				err,
			)
		}

		bucket := &image
		if _, ok := containerLayers[layer.ID]; ok {
			bucket = &container
		}

		bucket.bytes += uint64(usage.Size)
		bucket.inodes += uint64(usage.InodeCount)
	}

	return image, container, nil
}

func filesystemUsage(mountpoint string, usage layerUsage) *types.FilesystemUsage {
	return &types.FilesystemUsage{
		Timestamp:  time.Now().UnixNano(),
		FsId:       &types.FilesystemIdentifier{Mountpoint: mountpoint},
		UsedBytes:  &types.UInt64Value{Value: usage.bytes},
		InodesUsed: &types.UInt64Value{Value: usage.inodes},
	}
}

// ImageFsInfo returns information of the filesystem that is used to store images.
func (s *Server) ImageFsInfo(
	context.Context,
	*types.ImageFsInfoRequest,
) (*types.ImageFsInfoResponse, error) {
	// Get the default ImageServer's store. There is no need to manage filesystem
	// info for images managed by the runtime.
	imageServer, err := s.StorageImageServer(nil)
	if err != nil {
		return nil, fmt.Errorf("get image server: %w", err)
	}

	store := imageServer.GetStore()

	fsUsage, err := getStorageFsInfo(store)
	if err != nil {
		return nil, fmt.Errorf("get image fs info: %w", err)
	}

	return fsUsage, nil
}
