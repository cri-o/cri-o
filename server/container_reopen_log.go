package server

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/oci"
	"golang.org/x/net/context"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// ReopenContainerLog reopens the containers log file
func (s *Server) ReopenContainerLog(ctx context.Context, req *pb.ReopenContainerLogRequest) (*pb.ReopenContainerLogResponse, error) {
	containerID := req.ContainerId
	c := s.GetContainer(containerID)

	if c == nil {
		return nil, fmt.Errorf("could not find container %q", containerID)
	}

	if err := s.ContainerServer.Runtime().UpdateContainerStatus(c); err != nil {
		return nil, err
	}

	cState := c.State()
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return nil, fmt.Errorf("container is not created or running")
	}

	if err := s.ContainerServer.Runtime().ReopenContainerLog(c); err != nil {
		return nil, err
	}
	return &pb.ReopenContainerLogResponse{}, nil
}
