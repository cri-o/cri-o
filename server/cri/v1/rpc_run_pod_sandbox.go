package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) RunPodSandbox(
	ctx context.Context, req *pb.RunPodSandboxRequest,
) (*pb.RunPodSandboxResponse, error) {
	return nil, nil
}
