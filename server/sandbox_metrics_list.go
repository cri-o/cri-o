package server

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ListPodSandboxMetrics lists all pod sandbox metrics
func (s *Server) ListPodSandboxMetrics(ctx context.Context, req *types.ListPodSandboxMetricsRequest) (*types.ListPodSandboxMetricsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method unimplemented")
}
