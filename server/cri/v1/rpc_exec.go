package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (c *service) Exec(
	ctx context.Context, req *pb.ExecRequest,
) (*pb.ExecResponse, error) {
	return nil, nil
}
