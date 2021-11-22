package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ListContainers(
	ctx context.Context, req *pb.ListContainersRequest,
) (*pb.ListContainersResponse, error) {
	resp, err := s.server.ListContainers(ctx, (*v1.ListContainersRequest)(unsafe.Pointer(req)))
	return (*pb.ListContainersResponse)(unsafe.Pointer(resp)), err
}
