package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) RemoveImage(
	ctx context.Context, req *pb.RemoveImageRequest,
) (*pb.RemoveImageResponse, error) {
	r := &types.RemoveImageRequest{}
	if req.Image != nil {
		r.Image = &types.ImageSpec{
			Image:       req.Image.Image,
			Annotations: req.Image.Annotations,
		}
	}
	if err := s.server.RemoveImage(ctx, r); err != nil {
		return nil, err
	}
	return &pb.RemoveImageResponse{}, nil
}
