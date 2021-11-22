package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) PodSandboxStats(
	ctx context.Context, req *pb.PodSandboxStatsRequest,
) (*pb.PodSandboxStatsResponse, error) {
	resp, err := s.server.PodSandboxStats(ctx, (*v1.PodSandboxStatsRequest)(unsafe.Pointer(req)))
	return (*pb.PodSandboxStatsResponse)(unsafe.Pointer(resp)), err
}
