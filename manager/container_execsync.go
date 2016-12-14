package manager

import (
	"fmt"

	"github.com/kubernetes-incubator/cri-o/oci"
)

// ExecSync runs a command in a container synchronously.
func (m *Manager) ExecSync(ctrID string, cmd []string, timeout int64) (*oci.ExecSyncResponse, error) {
	c, err := m.getContainerWithPartialID(ctrID)
	if err != nil {
		return nil, err
	}

	if err = m.runtime.UpdateStatus(c); err != nil {
		return nil, err
	}

	cState := m.runtime.ContainerStatus(c)
	if !(cState.Status == oci.ContainerStateRunning || cState.Status == oci.ContainerStateCreated) {
		return nil, fmt.Errorf("container is not created or running")
	}

	if cmd == nil {
		return nil, fmt.Errorf("exec command cannot be empty")
	}

	execResp, err := m.runtime.ExecSync(c, cmd, timeout)
	if err != nil {
		return nil, err
	}

	return execResp, nil
}
