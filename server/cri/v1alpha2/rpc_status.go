package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) Status(
	ctx context.Context, req *pb.StatusRequest,
) (*pb.StatusResponse, error) {
	resp, err := s.server.Status(ctx, (*v1.StatusRequest)(unsafe.Pointer(req)))
	return (*pb.StatusResponse)(unsafe.Pointer(resp)), err
}
