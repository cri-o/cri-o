package libkpod

import (
	"os"
	"path/filepath"

	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/pkg/errors"
)

// Remove removes a container
func (c *ContainerServer) Remove(container string, force bool) (string, error) {
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return "", err
	}
	ctrID := ctr.ID()

	cState := c.runtime.ContainerStatus(ctr)
	if cState.Status == oci.ContainerStateCreated || cState.Status == oci.ContainerStateRunning {
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
	if err := os.Remove(filepath.Join(c.Config().RuntimeConfig.ContainerExitsDir, ctrID)); err != nil && !os.IsNotExist(err) {
		return "", errors.Wrapf(err, "failed to remove container exit file %s", ctrID)
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
