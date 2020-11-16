package server

import (
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// UpdateRuntimeConfig updates the configuration of a running container.
func (s *Server) UpdateRuntimeConfig(ctx context.Context, req *pb.UpdateRuntimeConfigRequest) (*pb.UpdateRuntimeConfigResponse, error) {
	return &pb.UpdateRuntimeConfigResponse{}, nil
}
