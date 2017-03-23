package server

import (
	"fmt"

	"github.com/kubernetes-incubator/cri-o/oci"
)

func (s *Server) getContainerFromRequest(cid string) (*oci.Container, error) {
	if cid == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}

	c, err := s.state.LookupContainerByID(cid)
	if err != nil {
		return nil, fmt.Errorf("container with ID starting with %s could not be retrieved: %v", cid, err)
	}

	return c, nil
}
