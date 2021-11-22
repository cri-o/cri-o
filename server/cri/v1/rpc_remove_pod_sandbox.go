package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) RemovePodSandbox(
	ctx context.Context, req *pb.RemovePodSandboxRequest,
) (*pb.RemovePodSandboxResponse, error) {
	if err := s.server.RemovePodSandbox(ctx, req); err != nil {
		return nil, err
	}
	return &pb.RemovePodSandboxResponse{}, nil
}
