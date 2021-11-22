package v1alpha2

import (
	"context"
	"unsafe"

	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) ReopenContainerLog(
	ctx context.Context, req *pb.ReopenContainerLogRequest,
) (*pb.ReopenContainerLogResponse, error) {
	if err := s.server.ReopenContainerLog(ctx, (*v1.ReopenContainerLogRequest)(unsafe.Pointer(req))); err != nil {
		return nil, err
	}
	return &pb.ReopenContainerLogResponse{}, nil
}
