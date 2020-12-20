package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) PortForward(
	ctx context.Context, req *pb.PortForwardRequest,
) (*pb.PortForwardResponse, error) {
	r := &types.PortForwardRequest{
		PodSandboxID: req.PodSandboxId,
		Port:         req.Port,
	}
	res, err := s.server.PortForward(ctx, r)
	if err != nil {
		return nil, err
	}
	return &pb.PortForwardResponse{Url: res.URL}, nil
}
