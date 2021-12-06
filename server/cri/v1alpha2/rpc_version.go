package v1alpha2

import (
	"context"
	"unsafe"

	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) Version(
	ctx context.Context, req *pb.VersionRequest,
) (*pb.VersionResponse, error) {
	resp, err := s.server.Version(ctx, "v1alpha2")
	return (*pb.VersionResponse)(unsafe.Pointer(resp)), err
}
