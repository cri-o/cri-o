package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) StopPodSandbox(
	ctx context.Context, req *pb.StopPodSandboxRequest,
) (*pb.StopPodSandboxResponse, error) {
	if err := s.server.StopPodSandbox(ctx, req); err != nil {
		return nil, err
	}
	return &pb.StopPodSandboxResponse{}, nil
}
