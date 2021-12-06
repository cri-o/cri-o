package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) ListImages(
	ctx context.Context, req *pb.ListImagesRequest,
) (*pb.ListImagesResponse, error) {
	return s.server.ListImages(ctx, req)
}
