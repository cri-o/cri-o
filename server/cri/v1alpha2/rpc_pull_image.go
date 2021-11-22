package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) PullImage(
	ctx context.Context, req *pb.PullImageRequest,
) (*pb.PullImageResponse, error) {
	resp, err := s.server.PullImage(ctx, (*v1.PullImageRequest)(unsafe.Pointer(req)))
	return (*pb.PullImageResponse)(unsafe.Pointer(resp)), err
}
