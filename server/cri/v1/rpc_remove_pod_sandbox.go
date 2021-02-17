package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) RemovePodSandbox(
	ctx context.Context, req *pb.RemovePodSandboxRequest,
) (*pb.RemovePodSandboxResponse, error) {
	return nil, nil
}
