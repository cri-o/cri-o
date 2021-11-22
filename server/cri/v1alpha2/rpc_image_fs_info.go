package v1alpha2

import (
	"context"
	"unsafe"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ImageFsInfo(
	ctx context.Context, req *pb.ImageFsInfoRequest,
) (*pb.ImageFsInfoResponse, error) {
	resp, err := s.server.ImageFsInfo(ctx)
	return (*pb.ImageFsInfoResponse)(unsafe.Pointer(resp)), err
}
