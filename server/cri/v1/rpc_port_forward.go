package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) PortForward(
	ctx context.Context, req *pb.PortForwardRequest,
) (*pb.PortForwardResponse, error) {
	return nil, nil
}
