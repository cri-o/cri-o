package server

import (
	"context"
	"path"
	"time"

	"github.com/containers/storage"
	crioStorage "github.com/cri-o/cri-o/utils"
	"github.com/pkg/errors"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
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
		FsId:       &types.FilesystemIdentifier{Mountpoint: imagesPath},
		UsedBytes:  &types.UInt64Value{Value: bytesUsed},
		InodesUsed: &types.UInt64Value{Value: inodesUsed},
	}

	return &usage, nil
}

// ImageFsInfo returns information of the filesystem that is used to store images.
func (s *Server) ImageFsInfo(context.Context) (*types.ImageFsInfoResponse, error) {
	var resp types.ImageFsInfoResponse
	var lastError error
	store := s.GetAllStores()
	for _, s := range store {
		fsUsage, err := getStorageFsInfo(s)
		if err != nil {
			if lastError == nil {
				lastError = err
			} else {
				lastError = errors.Wrap(lastError, err.Error())
			}
			continue
		}
		resp.ImageFilesystems = append(resp.ImageFilesystems, fsUsage)
	}
	if lastError != nil {
		return nil, lastError
	}
	return &resp, nil
}
