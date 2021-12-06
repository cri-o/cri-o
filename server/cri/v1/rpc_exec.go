package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) Exec(
	ctx context.Context, req *pb.ExecRequest,
) (*pb.ExecResponse, error) {
	return s.server.Exec(ctx, req)
}
