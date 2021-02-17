package v1alpha2

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (c *service) ListContainerStats(
	ctx context.Context, req *pb.ListContainerStatsRequest,
) (*pb.ListContainerStatsResponse, error) {
	return nil, nil
}
