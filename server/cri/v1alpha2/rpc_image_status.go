package v1alpha2

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (c *service) ImageStatus(
	ctx context.Context, req *pb.ImageStatusRequest,
) (*pb.ImageStatusResponse, error) {
	return nil, nil
}
