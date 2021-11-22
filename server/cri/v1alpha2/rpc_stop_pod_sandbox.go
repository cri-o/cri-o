package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) StopPodSandbox(
	ctx context.Context, req *pb.StopPodSandboxRequest,
) (*pb.StopPodSandboxResponse, error) {
	if err := s.server.StopPodSandbox(ctx, (*v1.StopPodSandboxRequest)(unsafe.Pointer(req))); err != nil {
		return nil, err
	}
	return &pb.StopPodSandboxResponse{}, nil
}
