package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ImageFsInfo(
	ctx context.Context, req *pb.ImageFsInfoRequest,
) (*pb.ImageFsInfoResponse, error) {
	return s.server.ImageFsInfo(ctx)
}
