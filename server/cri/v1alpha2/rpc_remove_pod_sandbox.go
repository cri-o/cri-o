package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) RemovePodSandbox(
	ctx context.Context, req *pb.RemovePodSandboxRequest,
) (*pb.RemovePodSandboxResponse, error) {
	r := &types.RemovePodSandboxRequest{
		PodSandboxID: req.PodSandboxId,
	}
	if err := s.server.RemovePodSandbox(ctx, r); err != nil {
		return nil, err
	}
	return &pb.RemovePodSandboxResponse{}, nil
}
