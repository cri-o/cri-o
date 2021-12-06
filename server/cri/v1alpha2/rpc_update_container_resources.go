package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) UpdateContainerResources(
	ctx context.Context, req *pb.UpdateContainerResourcesRequest,
) (*pb.UpdateContainerResourcesResponse, error) {
	if err := s.server.UpdateContainerResources(ctx, (*v1.UpdateContainerResourcesRequest)(unsafe.Pointer(req))); err != nil {
		return nil, err
	}
	return &pb.UpdateContainerResourcesResponse{}, nil
}
