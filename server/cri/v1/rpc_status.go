package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) Status(
	ctx context.Context, req *pb.StatusRequest,
) (*pb.StatusResponse, error) {
	return nil, nil
}
