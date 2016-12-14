package manager

import (
	"fmt"

	"github.com/kubernetes-incubator/cri-o/oci"
)

// StopContainer stops a running container with a grace period (i.e., timeout).
func (m *Manager) StopContainer(ctrID string, timeout int64) error {
	c, err := m.getContainerWithPartialID(ctrID)
	if err != nil {
		return err
	}

	if err := m.runtime.UpdateStatus(c); err != nil {
		return err
	}
	cStatus := m.runtime.ContainerStatus(c)
	if cStatus.Status != oci.ContainerStateStopped {
		if err := m.runtime.StopContainer(c); err != nil {
			return fmt.Errorf("failed to stop container %s: %v", c.ID(), err)
		}
	}

	return nil
}
