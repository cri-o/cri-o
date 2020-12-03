package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) StartContainer(
	ctx context.Context, req *pb.StartContainerRequest,
) (resp *pb.StartContainerResponse, retErr error) {
	return nil, nil
}
