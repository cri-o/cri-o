package lib

import (
	"context"

	"github.com/cri-o/cri-o/oci"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ContainerStop stops a running container with a grace period (i.e., timeout).
func (c *ContainerServer) ContainerStop(ctx context.Context, container string, timeout int64) (string, error) {
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find container %s", container)
	}
	ctrID := ctr.ID()

	cStatus := ctr.State()
	switch cStatus.Status {
	case oci.ContainerStateStopped: // no-op
	case oci.ContainerStatePaused:
		return "", errors.Errorf("cannot stop paused container %s", ctrID)
	default:
		if err := c.runtime.StopContainer(ctx, ctr, timeout); err != nil {
			return "", errors.Wrapf(err, "failed to stop container %s", ctrID)
		}
		if err := c.runtime.WaitContainerStateStopped(ctx, ctr); err != nil {
			return "", errors.Wrapf(err, "failed to get container 'stopped' status %s", ctrID)
		}
		if err := c.storageRuntimeServer.StopContainer(ctrID); err != nil {
			return "", errors.Wrapf(err, "failed to unmount container %s", ctrID)
		}
	}

	if err := c.ContainerStateToDisk(ctr); err != nil {
		logrus.Warnf("unable to write containers %s state to disk: %v", ctr.ID(), err)
	}

	return ctrID, nil
}
