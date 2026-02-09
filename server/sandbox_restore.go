package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// RestorePod restores a pod sandbox from a checkpoint.
func (s *Server) RestorePod(ctx context.Context, req *types.RestorePodRequest) (*types.RestorePodResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RestorePod not implemented")
}
