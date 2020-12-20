package server

import (
	"context"
	"path"
	"time"

	"github.com/containers/storage"
	"github.com/cri-o/cri-o/server/cri/types"
	crioStorage "github.com/cri-o/cri-o/utils"
)

func getStorageFsInfo(store storage.Store) (*types.FilesystemUsage, error) {
	rootPath := store.GraphRoot()
	storageDriver := store.GraphDriverName()
	imagesPath := path.Join(rootPath, storageDriver+"-images")

	bytesUsed, inodesUsed, err := crioStorage.GetDiskUsageStats(imagesPath)
	if err != nil {
		return nil, err
	}

	usage := types.FilesystemUsage{
		Timestamp:  time.Now().UnixNano(),
		FsID:       &types.FilesystemIdentifier{Mountpoint: imagesPath},
		UsedBytes:  &types.UInt64Value{Value: bytesUsed},
		InodesUsed: &types.UInt64Value{Value: inodesUsed},
	}

	return &usage, nil
}

// ImageFsInfo returns information of the filesystem that is used to store images.
func (s *Server) ImageFsInfo(context.Context) (*types.ImageFsInfoResponse, error) {
	store := s.StorageImageServer().GetStore()
	fsUsage, err := getStorageFsInfo(store)
	if err != nil {
		return nil, err
	}

	return &types.ImageFsInfoResponse{
		ImageFilesystems: []*types.FilesystemUsage{fsUsage},
	}, nil
}
