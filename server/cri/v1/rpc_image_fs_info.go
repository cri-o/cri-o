package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ImageFsInfo(
	ctx context.Context, req *pb.ImageFsInfoRequest,
) (*pb.ImageFsInfoResponse, error) {
	resp, err := s.server.ImageFsInfo(ctx)
	if err != nil {
		return nil, err
	}
	imageFilesystems := []*pb.FilesystemUsage{}
	for _, x := range resp.ImageFilesystems {
		item := &pb.FilesystemUsage{
			Timestamp: x.Timestamp,
		}
		if x.FsID != nil {
			item.FsId = &pb.FilesystemIdentifier{Mountpoint: x.FsID.Mountpoint}
		}
		if x.UsedBytes != nil {
			item.UsedBytes = &pb.UInt64Value{Value: x.UsedBytes.Value}
		}
		if x.InodesUsed != nil {
			item.InodesUsed = &pb.UInt64Value{Value: x.InodesUsed.Value}
		}
		imageFilesystems = append(imageFilesystems, item)
	}
	return &pb.ImageFsInfoResponse{
		ImageFilesystems: imageFilesystems,
	}, nil
}
