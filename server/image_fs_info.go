package server

import (
	"path"
	"time"

	"github.com/containers/storage"
	crioStorage "github.com/cri-o/cri-o/utils"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func getStorageFsInfo(store storage.Store) (*pb.FilesystemUsage, error) {
	rootPath := store.GraphRoot()
	storageDriver := store.GraphDriverName()
	imagesPath := path.Join(rootPath, storageDriver+"-images")

	bytesUsed, inodesUsed, err := crioStorage.GetDiskUsageStats(imagesPath)
	if err != nil {
		return nil, err
	}

	usage := pb.FilesystemUsage{
		Timestamp:  time.Now().UnixNano(),
		FsId:       &pb.FilesystemIdentifier{Mountpoint: imagesPath},
		UsedBytes:  &pb.UInt64Value{Value: bytesUsed},
		InodesUsed: &pb.UInt64Value{Value: inodesUsed},
	}

	return &usage, nil
}

// ImageFsInfo returns information of the filesystem that is used to store images.
func (s *Server) ImageFsInfo(ctx context.Context, req *pb.ImageFsInfoRequest) (*pb.ImageFsInfoResponse, error) {
	store := s.StorageImageServer().GetStore()
	fsUsage, err := getStorageFsInfo(store)

	if err != nil {
		return nil, err
	}

	return &pb.ImageFsInfoResponse{
		ImageFilesystems: []*pb.FilesystemUsage{fsUsage},
	}, nil
}
