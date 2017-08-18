package libkpod

import (
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/pkg/errors"
)

// ContainerRemove removes a r
func (c *ContainerServer) ContainerRemove(container string, force bool) (string, error) {
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return "", err
	}
	ctrID := ctr.ID()

	if err := c.runtime.UpdateStatus(ctr); err != nil {
		return "", errors.Wrapf(err, "failed to update container state")
	}

	err = c.runtime.UpdateStatus(ctr)
	if err != nil {
		return "", errors.Wrapf(err, "could not update status for container %s", ctrID)
	}
	cState := c.runtime.ContainerStatus(ctr)
	if cState.Status == oci.ContainerStateRunning {
		if force {
			_, err = c.ContainerStop(container, -1)
			if err != nil {
				return "", errors.Wrapf(err, "unable to stop container %s", ctrID)
			}
		} else {
			return "", errors.Errorf("cannot remove running container %s", ctrID)
		}
	}

	if err := c.runtime.DeleteContainer(ctr); err != nil {
		return "", errors.Wrapf(err, "failed to delete container %s", ctrID)
	}

	c.RemoveContainer(ctr)

	if err := c.storageRuntimeServer.DeleteContainer(ctrID); err != nil {
		return "", errors.Wrapf(err, "failed to delete storage for container %s", ctrID)
	}

	c.ReleaseContainerName(ctr.Name())

	if err := c.ctrIDIndex.Delete(ctrID); err != nil {
		return "", err
	}
	return ctrID, nil
}
