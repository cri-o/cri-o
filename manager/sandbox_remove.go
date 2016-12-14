package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/opencontainers/runc/libcontainer/label"
)

// RemovePodSandbox deletes the sandbox. If there are any running containers in the
// sandbox, they should be force deleted.
func (m *Manager) RemovePodSandbox(sbID string) error {
	sb, err := m.getPodSandboxWithPartialID(sbID)
	if err != nil {
		if err == errSandboxIDEmpty {
			return err
		}

		logrus.Warnf("could not get sandbox %s, it's probably been removed already: %v", sbID, err)
		return nil
	}

	podInfraContainer := sb.infraContainer
	containers := sb.containers.List()
	containers = append(containers, podInfraContainer)

	// Delete all the containers in the sandbox
	for _, c := range containers {
		if err := m.runtime.UpdateStatus(c); err != nil {
			return fmt.Errorf("failed to update container state: %v", err)
		}

		cState := m.runtime.ContainerStatus(c)
		if cState.Status == oci.ContainerStateCreated || cState.Status == oci.ContainerStateRunning {
			if err := m.runtime.StopContainer(c); err != nil {
				return fmt.Errorf("failed to stop container %s: %v", c.Name(), err)
			}
		}

		if err := m.runtime.DeleteContainer(c); err != nil {
			return fmt.Errorf("failed to delete container %s in sandbox %s: %v", c.Name(), sb.id, err)
		}

		if c == podInfraContainer {
			continue
		}

		containerDir := filepath.Join(m.runtime.ContainerDir(), c.ID())
		if err := os.RemoveAll(containerDir); err != nil {
			return fmt.Errorf("failed to remove container %s directory: %v", c.Name(), err)
		}

		m.releaseContainerName(c.Name())
		m.removeContainer(c)
	}

	if err := label.UnreserveLabel(sb.processLabel); err != nil {
		return err
	}

	// unmount the shm for the pod
	if sb.shmPath != "/dev/shm" {
		if err := syscall.Unmount(sb.shmPath, syscall.MNT_DETACH); err != nil {
			return err
		}
	}

	if err := sb.netNsRemove(); err != nil {
		return fmt.Errorf("failed to remove networking namespace for sandbox %s: %v", sb.id, err)
	}

	// Remove the files related to the sandbox
	podSandboxDir := filepath.Join(m.config.SandboxDir, sb.id)
	if err := os.RemoveAll(podSandboxDir); err != nil {
		return fmt.Errorf("failed to remove sandbox %s directory: %v", sb.id, err)
	}
	m.releaseContainerName(podInfraContainer.Name())
	m.removeContainer(podInfraContainer)
	sb.infraContainer = nil

	m.releasePodName(sb.name)
	m.removeSandbox(sb.id)

	return nil
}
