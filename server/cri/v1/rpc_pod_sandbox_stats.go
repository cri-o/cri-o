package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) PodSandboxStats(
	ctx context.Context, req *pb.PodSandboxStatsRequest,
) (*pb.PodSandboxStatsResponse, error) {
	return s.server.PodSandboxStats(ctx, req)
}
