package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) CreateContainer(
	ctx context.Context, req *pb.CreateContainerRequest,
) (*pb.CreateContainerResponse, error) {
	resp, err := s.server.CreateContainer(ctx, (*v1.CreateContainerRequest)(unsafe.Pointer(req)))
	return (*pb.CreateContainerResponse)(unsafe.Pointer(resp)), err
}
