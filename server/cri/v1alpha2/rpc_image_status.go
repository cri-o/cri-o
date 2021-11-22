package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ImageStatus(
	ctx context.Context, req *pb.ImageStatusRequest,
) (*pb.ImageStatusResponse, error) {
	resp, err := s.server.ImageStatus(ctx, (*v1.ImageStatusRequest)(unsafe.Pointer(req)))
	return (*pb.ImageStatusResponse)(unsafe.Pointer(resp)), err
}
