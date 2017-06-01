package server

import (
	"fmt"

	"github.com/kubernetes-incubator/cri-o/oci"
)

func (s *Server) getContainerFromRequest(cid string) (*oci.Container, error) {
	if cid == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}

	containerID, err := s.ctrIDIndex.Get(cid)
	if err != nil {
		return nil, fmt.Errorf("container with ID starting with %s not found: %v", cid, err)
	}

	c := s.state.containers.Get(containerID)
	if c == nil {
		return nil, fmt.Errorf("specified container not found: %s", containerID)
	}
	return c, nil
}
