package lib

import (
	"fmt"

	"github.com/cri-o/cri-o/v1alpha2/oci"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ContainerPause pauses a running container.
func (c *ContainerServer) ContainerPause(container string) (string, error) {
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find container %s", container)
	}

	cStatus := ctr.State()
	if cStatus.Status != oci.ContainerStatePaused {
		if err := c.runtime.PauseContainer(ctr); err != nil {
			return "", errors.Wrapf(err, "failed to pause container %s", ctr.ID())
		}
		if err := c.ContainerStateToDisk(ctr); err != nil {
			logrus.Warnf("unable to write containers %s state to disk: %v", ctr.ID(), err)
		}
	} else {
		return "", fmt.Errorf("container %s is already paused", ctr.ID())
	}

	return ctr.ID(), nil
}

// ContainerUnpause unpauses a running container with a grace period (i.e., timeout).
func (c *ContainerServer) ContainerUnpause(container string) (string, error) {
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find container %s", container)
	}

	cStatus := ctr.State()
	if cStatus.Status == oci.ContainerStatePaused {
		if err := c.runtime.UnpauseContainer(ctr); err != nil {
			return "", errors.Wrapf(err, "failed to unpause container %s", ctr.ID())
		}
		if err := c.ContainerStateToDisk(ctr); err != nil {
			logrus.Warnf("unable to write containers %s state to disk: %v", ctr.ID(), err)
		}
	} else {
		return "", fmt.Errorf("the container %s is not paused", ctr.ID())
	}

	return ctr.ID(), nil
}
