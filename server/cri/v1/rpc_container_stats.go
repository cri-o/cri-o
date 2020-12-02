package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) ContainerStats(
	ctx context.Context, req *pb.ContainerStatsRequest,
) (*pb.ContainerStatsResponse, error) {
	return nil, nil
}
