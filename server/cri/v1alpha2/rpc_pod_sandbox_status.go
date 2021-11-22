package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) PodSandboxStatus(
	ctx context.Context, req *pb.PodSandboxStatusRequest,
) (*pb.PodSandboxStatusResponse, error) {
	resp, err := s.server.PodSandboxStatus(ctx, (*v1.PodSandboxStatusRequest)(unsafe.Pointer(req)))
	return (*pb.PodSandboxStatusResponse)(unsafe.Pointer(resp)), err
}
