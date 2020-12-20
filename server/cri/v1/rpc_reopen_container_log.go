package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ReopenContainerLog(
	ctx context.Context, req *pb.ReopenContainerLogRequest,
) (*pb.ReopenContainerLogResponse, error) {
	r := &types.ReopenContainerLogRequest{
		ContainerID: req.ContainerId,
	}
	if err := s.server.ReopenContainerLog(ctx, r); err != nil {
		return nil, err
	}
	return &pb.ReopenContainerLogResponse{}, nil
}
