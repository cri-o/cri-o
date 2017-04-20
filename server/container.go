package server

import (
	"fmt"

	"github.com/kubernetes-incubator/cri-o/oci"
)

const (
	// containerTypeSandbox represents a pod sandbox container
	containerTypeSandbox = "sandbox"
	// containerTypeContainer represents a container running within a pod
	containerTypeContainer = "container"
)

func (s *Server) getContainerFromRequest(containerID string) (*oci.Container, error) {
	if containerID == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}

	c, err := s.state.LookupContainerByID(containerID)
	if err != nil {
		return nil, fmt.Errorf("container with ID starting with %v could not be retrieved: %v", containerID, err)
	}

	return c, nil
}
