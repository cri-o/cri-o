package v1alpha2

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (c *service) UpdateContainerResources(
	ctx context.Context, req *pb.UpdateContainerResourcesRequest,
) (*pb.UpdateContainerResourcesResponse, error) {
	return nil, nil
}
