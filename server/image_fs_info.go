package server

import (
	"path"
	"time"

	"github.com/containers/storage"
	crioStorage "github.com/kubernetes-incubator/cri-o/utils"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

func getStorageFsInfo(store storage.Store) (*pb.FilesystemUsage, error) {
	rootPath := store.GraphRoot()
	storageDriver := store.GraphDriverName()
	imagesPath := path.Join(rootPath, storageDriver+"-images")

	deviceName, err := crioStorage.GetDeviceNameFromPath(imagesPath)

	uuid, err := crioStorage.GetDeviceUUIDFromPath(deviceName)
	if err != nil {
		return nil, err
	}

	bytesUsed, inodesUsed, err := crioStorage.GetDiskUsageStats(imagesPath)
	if err != nil {
		return nil, err
	}

	usage := pb.FilesystemUsage{
		Timestamp:  time.Now().UnixNano(),
		StorageId:  &pb.StorageIdentifier{uuid},
		UsedBytes:  &pb.UInt64Value{bytesUsed},
		InodesUsed: &pb.UInt64Value{inodesUsed},
	}

	return &usage, nil
}

// ImageFsInfo returns information of the filesystem that is used to store images.
func (s *Server) ImageFsInfo(ctx context.Context, req *pb.ImageFsInfoRequest) (resp *pb.ImageFsInfoResponse, err error) {
	const operation = "image_fs_info"
	defer func() {
		recordOperation(operation, time.Now())
		recordError(operation, err)
	}()

	store := s.StorageImageServer().GetStore()
	fsUsage, err := getStorageFsInfo(store)

	if err != nil {
		return nil, err
	}

	return &pb.ImageFsInfoResponse{
		ImageFilesystems: []*pb.FilesystemUsage{fsUsage},
	}, nil
}
