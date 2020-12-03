package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) PodSandboxStatus(
	ctx context.Context, req *pb.PodSandboxStatusRequest,
) (*pb.PodSandboxStatusResponse, error) {
	return nil, nil
}
