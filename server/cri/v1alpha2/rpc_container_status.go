package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ContainerStatus(
	ctx context.Context, req *pb.ContainerStatusRequest,
) (*pb.ContainerStatusResponse, error) {
	resp, err := s.server.ContainerStatus(ctx, (*v1.ContainerStatusRequest)(unsafe.Pointer(req)))
	return (*pb.ContainerStatusResponse)(unsafe.Pointer(resp)), err
}
