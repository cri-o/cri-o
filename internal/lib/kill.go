package lib

import (
	"syscall"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ContainerKill sends the user provided signal to the containers primary process.
func (c *ContainerServer) ContainerKill(container string, killSignal syscall.Signal) (string, error) {
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find container %s", container)
	}
	if err := c.runtime.UpdateContainerStatus(ctr); err != nil {
		logrus.Warnf("unable to update containers %s status: %v", ctr.ID(), err)
	}
	cStatus := ctr.State()

	// If the container is not running, error and move on.
	if cStatus.Status != oci.ContainerStateRunning {
		return "", errors.Errorf("cannot kill container %s: it is not running", container)
	}

	if err := c.runtime.SignalContainer(ctr, killSignal); err != nil {
		return "", err
	}

	if err := c.ContainerStateToDisk(ctr); err != nil {
		logrus.Warnf("unable to write containers %s state to disk: %v", ctr.ID(), err)
	}
	return ctr.ID(), nil
}
