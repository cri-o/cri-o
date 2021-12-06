package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) StartContainer(
	ctx context.Context, req *pb.StartContainerRequest,
) (resp *pb.StartContainerResponse, retErr error) {
	if err := s.server.StartContainer(ctx, (*v1.StartContainerRequest)(unsafe.Pointer(req))); err != nil {
		return nil, err
	}
	return &pb.StartContainerResponse{}, nil
}
