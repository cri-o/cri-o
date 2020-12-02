package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) Version(
	ctx context.Context, req *pb.VersionRequest,
) (*pb.VersionResponse, error) {
	return nil, nil
}
