package manager

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

func (m *Manager) getContainerWithPartialID(ctrID string) (*oci.Container, error) {
	if ctrID == "" {
		return nil, fmt.Errorf("container ID should not be empty")
	}

	containerID, err := m.ctrIDIndex.Get(ctrID)
	if err != nil {
		return nil, fmt.Errorf("container with ID starting with %s not found: %v", ctrID, err)
	}

	c := m.state.containers.Get(containerID)
	if c == nil {
		return nil, fmt.Errorf("specified container not found: %s", containerID)
	}
	return c, nil
}
