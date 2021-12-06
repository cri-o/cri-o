package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) PortForward(
	ctx context.Context, req *pb.PortForwardRequest,
) (*pb.PortForwardResponse, error) {
	return s.server.PortForward(ctx, req)
}
