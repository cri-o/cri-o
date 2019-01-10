package lib

import (
	"github.com/kubernetes-sigs/cri-o/oci"
	"github.com/pkg/errors"
	"syscall"
)

// ContainerKill sends the user provided signal to the containers primary process.
func (c *ContainerServer) ContainerKill(container string, killSignal syscall.Signal) (string, error) { // nolint
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find container %s", container)
	}
	c.runtime.UpdateContainerStatus(ctr)
	cStatus := ctr.State()

	// If the container is not running, error and move on.
	if cStatus.Status != oci.ContainerStateRunning {
		return "", errors.Errorf("cannot kill container %s: it is not running", container)
	}

	if err = c.runtime.SignalContainer(ctr, killSignal); err != nil {
		return "", err
	}

	c.ContainerStateToDisk(ctr)
	return ctr.ID(), nil
}
