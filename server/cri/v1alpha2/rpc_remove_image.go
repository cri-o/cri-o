package v1alpha2

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (c *service) RemoveImage(
	ctx context.Context, req *pb.RemoveImageRequest,
) (*pb.RemoveImageResponse, error) {
	return nil, nil
}
