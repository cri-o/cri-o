package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ListPodSandboxStats(
	ctx context.Context, req *pb.ListPodSandboxStatsRequest,
) (*pb.ListPodSandboxStatsResponse, error) {
	return s.server.ListPodSandboxStats(ctx, req)
}
