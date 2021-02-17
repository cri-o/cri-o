package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) ListPodSandbox(
	ctx context.Context, req *pb.ListPodSandboxRequest,
) (*pb.ListPodSandboxResponse, error) {
	return nil, nil
}
