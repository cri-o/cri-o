package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) StopPodSandbox(
	ctx context.Context, req *pb.StopPodSandboxRequest,
) (*pb.StopPodSandboxResponse, error) {
	r := &types.StopPodSandboxRequest{PodSandboxID: req.PodSandboxId}
	if err := s.server.StopPodSandbox(ctx, r); err != nil {
		return nil, err
	}
	return &pb.StopPodSandboxResponse{}, nil
}
