package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) RunPodSandbox(
	ctx context.Context, req *pb.RunPodSandboxRequest,
) (*pb.RunPodSandboxResponse, error) {
	resp, err := s.server.RunPodSandbox(ctx, (*v1.RunPodSandboxRequest)(unsafe.Pointer(req)))
	return (*pb.RunPodSandboxResponse)(unsafe.Pointer(resp)), err
}
