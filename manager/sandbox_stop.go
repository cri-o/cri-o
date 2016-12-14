package manager

import (
	"fmt"
	"os"

	"github.com/kubernetes-incubator/cri-o/oci"
)

// StopPodSandbox stops the sandbox. If there are any running containers in the
// sandbox, they should be force terminated.
func (m *Manager) StopPodSandbox(sbID string) error {
	sb, err := m.getPodSandboxWithPartialID(sbID)
	if err != nil {
		return err
	}

	podNamespace := ""
	podInfraContainer := sb.infraContainer
	netnsPath, err := podInfraContainer.NetNsPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(netnsPath); err == nil {
		if err2 := m.netPlugin.TearDownPod(netnsPath, podNamespace, sb.id, podInfraContainer.Name()); err2 != nil {
			return fmt.Errorf("failed to destroy network for container %s in sandbox %s: %v",
				podInfraContainer.Name(), sb.id, err2)
		}
	} else if !os.IsNotExist(err) { // it's ok for netnsPath to *not* exist
		return fmt.Errorf("failed to stat netns path for container %s in sandbox %s before tearing down the network: %v",
			podInfraContainer.Name(), sb.id, err)
	}

	// Close the sandbox networking namespace.
	if err := sb.netNsRemove(); err != nil {
		return err
	}

	containers := sb.containers.List()
	containers = append(containers, podInfraContainer)

	for _, c := range containers {
		if err := m.runtime.UpdateStatus(c); err != nil {
			return err
		}
		cStatus := m.runtime.ContainerStatus(c)
		if cStatus.Status != oci.ContainerStateStopped {
			if err := m.runtime.StopContainer(c); err != nil {
				return fmt.Errorf("failed to stop container %s in sandbox %s: %v", c.Name(), sb.id, err)
			}
		}
	}

	return nil
}
