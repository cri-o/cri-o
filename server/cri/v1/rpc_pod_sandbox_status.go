package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) PodSandboxStatus(
	ctx context.Context, req *pb.PodSandboxStatusRequest,
) (*pb.PodSandboxStatusResponse, error) {
	return s.server.PodSandboxStatus(ctx, req)
}
