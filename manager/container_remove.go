package manager

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kubernetes-incubator/cri-o/oci"
)

// RemoveContainer removes the container. If the container is running, the container
// should be force removed.
func (m *Manager) RemoveContainer(ctrID string) error {
	c, err := m.getContainerWithPartialID(ctrID)
	if err != nil {
		return err
	}

	if err := m.runtime.UpdateStatus(c); err != nil {
		return fmt.Errorf("failed to update container state: %v", err)
	}

	cState := m.runtime.ContainerStatus(c)
	if cState.Status == oci.ContainerStateCreated || cState.Status == oci.ContainerStateRunning {
		if err := m.runtime.StopContainer(c); err != nil {
			return fmt.Errorf("failed to stop container %s: %v", c.ID(), err)
		}
	}

	if err := m.runtime.DeleteContainer(c); err != nil {
		return fmt.Errorf("failed to delete container %s: %v", c.ID(), err)
	}

	containerDir := filepath.Join(m.runtime.ContainerDir(), c.ID())
	if err := os.RemoveAll(containerDir); err != nil {
		return fmt.Errorf("failed to remove container %s directory: %v", c.ID(), err)
	}

	m.releaseContainerName(c.Name())
	m.removeContainer(c)

	if err := m.ctrIDIndex.Delete(c.ID()); err != nil {
		return err
	}

	return nil
}
