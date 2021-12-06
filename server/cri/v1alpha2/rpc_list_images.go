package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ListImages(
	ctx context.Context, req *pb.ListImagesRequest,
) (*pb.ListImagesResponse, error) {
	resp, err := s.server.ListImages(ctx, (*v1.ListImagesRequest)(unsafe.Pointer(req)))
	return (*pb.ListImagesResponse)(unsafe.Pointer(resp)), err
}
