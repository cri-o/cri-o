package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) RunPodSandbox(
	ctx context.Context, req *pb.RunPodSandboxRequest,
) (*pb.RunPodSandboxResponse, error) {
	return s.server.RunPodSandbox(ctx, req)
}
