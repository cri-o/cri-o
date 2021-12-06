package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ExecSync(
	ctx context.Context, req *pb.ExecSyncRequest,
) (*pb.ExecSyncResponse, error) {
	resp, err := s.server.ExecSync(ctx, (*v1.ExecSyncRequest)(unsafe.Pointer(req)))
	return (*pb.ExecSyncResponse)(unsafe.Pointer(resp)), err
}
