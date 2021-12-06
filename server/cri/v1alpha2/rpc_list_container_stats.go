package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ListContainerStats(
	ctx context.Context, req *pb.ListContainerStatsRequest,
) (*pb.ListContainerStatsResponse, error) {
	resp, err := s.server.ListContainerStats(ctx, (*v1.ListContainerStatsRequest)(unsafe.Pointer(req)))
	return (*pb.ListContainerStatsResponse)(unsafe.Pointer(resp)), err
}
