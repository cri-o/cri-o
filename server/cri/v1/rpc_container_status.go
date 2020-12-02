package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) ContainerStatus(
	ctx context.Context, req *pb.ContainerStatusRequest,
) (*pb.ContainerStatusResponse, error) {
	return nil, nil
}
