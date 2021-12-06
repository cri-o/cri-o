package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) PortForward(
	ctx context.Context, req *pb.PortForwardRequest,
) (*pb.PortForwardResponse, error) {
	resp, err := s.server.PortForward(ctx, (*v1.PortForwardRequest)(unsafe.Pointer(req)))
	return (*pb.PortForwardResponse)(unsafe.Pointer(resp)), err
}
