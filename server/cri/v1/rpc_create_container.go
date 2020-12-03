package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) CreateContainer(
	ctx context.Context, req *pb.CreateContainerRequest,
) (res *pb.CreateContainerResponse, retErr error) {
	return nil, nil
}
