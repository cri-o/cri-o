package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) CreateContainer(
	ctx context.Context, req *pb.CreateContainerRequest,
) (*pb.CreateContainerResponse, error) {
	return s.server.CreateContainer(ctx, req)
}
