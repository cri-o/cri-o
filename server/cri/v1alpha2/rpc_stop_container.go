package v1alpha2

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (c *service) StopContainer(
	ctx context.Context, req *pb.StopContainerRequest,
) (*pb.StopContainerResponse, error) {
	return nil, nil
}
