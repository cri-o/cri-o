package lib

import (
	"context"
	"os"
	"path/filepath"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
)

// Remove removes a container
func (c *ContainerServer) Remove(ctx context.Context, container string, force bool) (string, error) {
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return "", err
	}
	ctrID := ctr.ID()

	cStatus := ctr.State()
	switch cStatus.Status {
	case oci.ContainerStatePaused:
		return "", errors.Errorf("cannot remove paused container %s", ctrID)
	case oci.ContainerStateCreated, oci.ContainerStateRunning:
		if force {
			if err = c.StopContainer(ctx, ctr, 10); err != nil {
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

	ctr.CleanupConmonCgroup()
	c.ReleaseContainerName(ctr.Name())

	if err := c.ctrIDIndex.Delete(ctrID); err != nil {
		return "", err
	}
	return ctrID, nil
}
