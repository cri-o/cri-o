package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) StopPodSandbox(
	ctx context.Context, req *pb.StopPodSandboxRequest,
) (*pb.StopPodSandboxResponse, error) {
	return nil, nil
}
