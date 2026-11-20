package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"go.podman.io/storage"
	"go.podman.io/storage/pkg/directory"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// getStorageFsInfo reports the disk usage of the image and container
// filesystems by measuring the storage layer content under the graph driver's
// data directory (<root>/<driver>). When a dedicated image store is configured,
// the image filesystem is measured at <imageStore>/<driver> and the container
// filesystem at <root>/<driver>; otherwise both report the graph root usage.
func getStorageFsInfo(store storage.Store) (*types.ImageFsInfoResponse, error) {
	rootPath := store.GraphRoot()
	imagePath := store.ImageStore()
	storageDriver := store.GraphDriverName()

	graphUsage, err := getUsage(filepath.Join(rootPath, storageDriver), storageDriver)
	if err != nil {
		return nil, err
	}

	if imagePath == "" {
		return &types.ImageFsInfoResponse{
			ImageFilesystems:     []*types.FilesystemUsage{graphUsage},
			ContainerFilesystems: []*types.FilesystemUsage{graphUsage},
		}, nil
	}

	imageUsage, err := getUsage(filepath.Join(imagePath, storageDriver), storageDriver)
	if err != nil {
		return nil, err
	}

	return &types.ImageFsInfoResponse{
		ImageFilesystems:     []*types.FilesystemUsage{imageUsage},
		ContainerFilesystems: []*types.FilesystemUsage{graphUsage},
	}, nil
}

// getUsage sums the disk usage of every layer's content under driverHome. The
// overlay driver stores content in each layer's "diff" subdirectory, so walking
// only "diff" measures the stored content without descending into a layer's
// "merged" mount view; other drivers store content directly in the layer
// directory. Hard links are counted once.
func getUsage(driverHome, driverName string) (*types.FilesystemUsage, error) {
	entries, err := os.ReadDir(driverHome)
	if err != nil {
		return nil, fmt.Errorf("read storage driver directory %s: %w", driverHome, err)
	}

	var totalBytes, totalInodes int64

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		layerPath := filepath.Join(driverHome, entry.Name())
		if driverName == "overlay" {
			layerPath = filepath.Join(layerPath, "diff")
		}

		usage, err := directory.Usage(layerPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}

			return nil, fmt.Errorf("get disk usage for path %s: %w", layerPath, err)
		}

		totalBytes += usage.Size
		totalInodes += usage.InodeCount
	}

	return &types.FilesystemUsage{
		Timestamp:  time.Now().UnixNano(),
		FsId:       &types.FilesystemIdentifier{Mountpoint: driverHome},
		UsedBytes:  &types.UInt64Value{Value: uint64(totalBytes)},
		InodesUsed: &types.UInt64Value{Value: uint64(totalInodes)},
	}, nil
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
