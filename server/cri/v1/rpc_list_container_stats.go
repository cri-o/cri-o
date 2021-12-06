package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ListContainerStats(
	ctx context.Context, req *pb.ListContainerStatsRequest,
) (*pb.ListContainerStatsResponse, error) {
	return s.server.ListContainerStats(ctx, req)
}
