package server

import (
	"fmt"

	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// UpdateContainerResources updates ContainerConfig of the container.
func (s *Server) UpdateContainerResources(ctx context.Context, req *pb.UpdateContainerResourcesRequest) (*pb.UpdateContainerResourcesResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
