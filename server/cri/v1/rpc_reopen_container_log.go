package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ReopenContainerLog(
	ctx context.Context, req *pb.ReopenContainerLogRequest,
) (*pb.ReopenContainerLogResponse, error) {
	if err := s.server.ReopenContainerLog(ctx, req); err != nil {
		return nil, err
	}
	return &pb.ReopenContainerLogResponse{}, nil
}
