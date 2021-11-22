package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ImageStatus(
	ctx context.Context, req *pb.ImageStatusRequest,
) (*pb.ImageStatusResponse, error) {
	return s.server.ImageStatus(ctx, req)
}
