package v1

import (
	"context"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) RemoveImage(
	ctx context.Context, req *pb.RemoveImageRequest,
) (*pb.RemoveImageResponse, error) {
	if err := s.server.RemoveImage(ctx, req); err != nil {
		return nil, err
	}
	return &pb.RemoveImageResponse{}, nil
}
