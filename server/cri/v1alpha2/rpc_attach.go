package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) Attach(
	ctx context.Context, req *pb.AttachRequest,
) (*pb.AttachResponse, error) {
	resp, err := s.server.Attach(ctx, (*v1.AttachRequest)(unsafe.Pointer(req)))
	return (*pb.AttachResponse)(unsafe.Pointer(resp)), err
}
