package server

import (
	"golang.org/x/net/context"

	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1"
)

// ContainerStats returns stats of the container. If the container does not
// exist, the call returns an error.
func (s *Server) ContainerStats(ctx context.Context, req *pb.ContainerStatsRequest) (*pb.ContainerStatsResponse, error) {
	return nil, nil
}

// ListContainerStats returns stats of all running containers.
func (s *Server) ListContainerStats(ctx context.Context, req *pb.ListContainerStatsRequest) (*pb.ListContainerStatsResponse, error) {
	return nil, nil
}
