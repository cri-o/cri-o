package lib

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/registrar"
)

// GetContainerFromShortID gets an oci container matching the specified full or partial id
func (c *ContainerServer) GetContainerFromShortID(cid string) (*oci.Container, error) {
	if cid == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}

	containerID, err := c.ctrIDIndex.Get(cid)
	if err != nil {
		return nil, fmt.Errorf("container with ID starting with %s not found: %v", cid, err)
	}

	ctr := c.GetContainer(containerID)
	if ctr == nil {
		return nil, fmt.Errorf("specified container not found: %s", containerID)
	}

	if !ctr.Created() {
		return nil, fmt.Errorf("specified container %s is not yet created", containerID)
	}

	return ctr, nil
}

// LookupContainer returns the container with the given name or full or partial id
func (c *ContainerServer) LookupContainer(idOrName string) (*oci.Container, error) {
	if idOrName == "" {
		return nil, fmt.Errorf("container ID or name should not be empty")
	}

	ctrID, err := c.ctrNameIndex.Get(idOrName)
	if err != nil {
		if err == registrar.ErrNameNotReserved {
			ctrID = idOrName
		} else {
			return nil, err
		}
	}

	return c.GetContainerFromShortID(ctrID)
}
