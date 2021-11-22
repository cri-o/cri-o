package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) StopContainer(
	ctx context.Context, req *pb.StopContainerRequest,
) (*pb.StopContainerResponse, error) {
	if err := s.server.StopContainer(ctx, (*v1.StopContainerRequest)(unsafe.Pointer(req))); err != nil {
		return nil, err
	}
	return &pb.StopContainerResponse{}, nil
}
