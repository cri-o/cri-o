package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) PullImage(
	ctx context.Context, req *pb.PullImageRequest,
) (*pb.PullImageResponse, error) {
	return nil, nil
}
