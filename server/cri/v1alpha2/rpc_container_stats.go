package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ContainerStats(
	ctx context.Context, req *pb.ContainerStatsRequest,
) (*pb.ContainerStatsResponse, error) {
	resp, err := s.server.ContainerStats(ctx, (*v1.ContainerStatsRequest)(unsafe.Pointer(req)))
	return (*pb.ContainerStatsResponse)(unsafe.Pointer(resp)), err
}
