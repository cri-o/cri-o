package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) RemoveContainer(
	ctx context.Context, req *pb.RemoveContainerRequest,
) (*pb.RemoveContainerResponse, error) {
	if err := s.server.RemoveContainer(ctx, (*v1.RemoveContainerRequest)(unsafe.Pointer(req))); err != nil {
		return nil, err
	}
	return &pb.RemoveContainerResponse{}, nil
}
