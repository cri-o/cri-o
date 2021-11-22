package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ListPodSandbox(
	ctx context.Context, req *pb.ListPodSandboxRequest,
) (*pb.ListPodSandboxResponse, error) {
	resp, err := s.server.ListPodSandbox(ctx, (*v1.ListPodSandboxRequest)(unsafe.Pointer(req)))
	return (*pb.ListPodSandboxResponse)(unsafe.Pointer(resp)), err
}
