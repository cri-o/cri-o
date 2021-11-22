package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) RemoveImage(
	ctx context.Context, req *pb.RemoveImageRequest,
) (*pb.RemoveImageResponse, error) {
	if err := s.server.RemoveImage(ctx, (*v1.RemoveImageRequest)(unsafe.Pointer(req))); err != nil {
		return nil, err
	}
	return &pb.RemoveImageResponse{}, nil
}
