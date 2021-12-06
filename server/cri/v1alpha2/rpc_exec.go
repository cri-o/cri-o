package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) Exec(
	ctx context.Context, req *pb.ExecRequest,
) (*pb.ExecResponse, error) {
	resp, err := s.server.Exec(ctx, (*v1.ExecRequest)(unsafe.Pointer(req)))
	return (*pb.ExecResponse)(unsafe.Pointer(resp)), err
}
