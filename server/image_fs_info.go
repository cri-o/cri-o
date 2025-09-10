package server

import (
	"context"
	"fmt"
	"path"
	"time"

	"go.podman.io/storage"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	crioStorage "github.com/cri-o/cri-o/utils"
)

func getStorageFsInfo(store storage.Store) (*types.ImageFsInfoResponse, error) {
	rootPath := store.GraphRoot()
	imagePath := store.ImageStore()
	storageDriver := store.GraphDriverName()

	var graphRootPath string

	if imagePath == "" {
		graphRootPath = path.Join(rootPath, storageDriver+"-images")
	} else {
		graphRootPath = path.Join(rootPath, storageDriver+"-containers")
	}

	graphUsage, err := getUsage(graphRootPath)
	if err != nil {
		return nil, fmt.Errorf("unable to get usage for %s: %w", graphRootPath, err)
	}

	if imagePath == "" {
		return &types.ImageFsInfoResponse{
			ImageFilesystems:     []*types.FilesystemUsage{graphUsage},
			ContainerFilesystems: []*types.FilesystemUsage{graphUsage},
		}, nil
	}

	resp := &types.ImageFsInfoResponse{
		ContainerFilesystems: []*types.FilesystemUsage{graphUsage},
	}

	imageRoot := path.Join(imagePath, storageDriver+"-images")

	imageUsage, err := getUsage(imageRoot)
	if err != nil {
		return nil, fmt.Errorf("unable to get usage for %s: %w", imageRoot, err)
	}

	resp.ImageFilesystems = []*types.FilesystemUsage{imageUsage}

	return resp, nil
}

// ImageFsInfo returns information of the filesystem that is used to store images.
func (s *Server) ImageFsInfo(context.Context, *types.ImageFsInfoRequest) (*types.ImageFsInfoResponse, error) {
	store := s.ContainerServer.StorageImageServer().GetStore()

	fsUsage, err := getStorageFsInfo(store)
	if err != nil {
		return nil, fmt.Errorf("get image fs info %w", err)
	}

	return fsUsage, nil
}

func getUsage(containerPath string) (*types.FilesystemUsage, error) {
	bytes, inodes, err := crioStorage.GetDiskUsageStats(containerPath)
	if err != nil {
		return nil, fmt.Errorf("get disk usage for path %s: %w", containerPath, err)
	}

	return &types.FilesystemUsage{
		Timestamp:  time.Now().UnixNano(),
		FsId:       &types.FilesystemIdentifier{Mountpoint: containerPath},
		UsedBytes:  &types.UInt64Value{Value: bytes},
		InodesUsed: &types.UInt64Value{Value: inodes},
	}, nil
}
