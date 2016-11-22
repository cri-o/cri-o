package server

import (
	"fmt"

	"github.com/kubernetes-incubator/cri-o/oci"
)

type containerRequest interface {
	GetContainerId() string
}

func (s *Server) getContainerFromRequest(req containerRequest) (*oci.Container, error) {
	ctrID := req.GetContainerId()
	if ctrID == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}

	containerID, err := s.ctrIDIndex.Get(ctrID)
	if err != nil {
		return nil, fmt.Errorf("container with ID starting with %s not found: %v", ctrID, err)
	}

	c := s.state.containers.Get(containerID)
	if c == nil {
		return nil, fmt.Errorf("specified container not found: %s", containerID)
	}
	return c, nil
}
