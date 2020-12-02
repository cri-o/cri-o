package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) RemoveContainer(
	ctx context.Context, req *pb.RemoveContainerRequest,
) (*pb.RemoveContainerResponse, error) {
	return nil, nil
}
