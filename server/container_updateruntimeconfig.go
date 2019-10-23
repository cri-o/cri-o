package server

import (
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// UpdateRuntimeConfig updates the configuration of a running container.
func (s *Server) UpdateRuntimeConfig(ctx context.Context, req *pb.UpdateRuntimeConfigRequest) (resp *pb.UpdateRuntimeConfigResponse, err error) {
	return &pb.UpdateRuntimeConfigResponse{}, nil
}
