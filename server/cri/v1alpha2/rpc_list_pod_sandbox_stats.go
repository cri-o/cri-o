package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ListPodSandboxStats(
	ctx context.Context, req *pb.ListPodSandboxStatsRequest,
) (*pb.ListPodSandboxStatsResponse, error) {
	resp, err := s.server.ListPodSandboxStats(ctx, (*v1.ListPodSandboxStatsRequest)(unsafe.Pointer(req)))
	return (*pb.ListPodSandboxStatsResponse)(unsafe.Pointer(resp)), err
}
