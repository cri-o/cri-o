package server

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListMetricDescriptors lists all metric descriptors
func (s *Server) ListMetricDescriptors(ctx context.Context, req *types.ListMetricDescriptorsRequest) (*types.ListMetricDescriptorsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method unimplemented")
}
