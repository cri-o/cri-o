package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) Attach(
	ctx context.Context, req *pb.AttachRequest,
) (*pb.AttachResponse, error) {
	return s.server.Attach(ctx, req)
}
