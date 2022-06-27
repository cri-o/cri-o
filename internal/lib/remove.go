package lib

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cri-o/cri-o/internal/oci"
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
	case oci.ContainerStatePaused, oci.ContainerStateCreated, oci.ContainerStateRunning:
		if !force {
			return "", fmt.Errorf("cannot remove %s container %s", cStatus.Status, ctrID)
		}
		if err = c.StopContainer(ctx, ctr, 10); err != nil {
			return "", fmt.Errorf("unable to stop container %s: %w", ctrID, err)
		}
	}

	if err := c.runtime.DeleteContainer(ctx, ctr); err != nil {
		return "", fmt.Errorf("failed to delete container %s: %w", ctrID, err)
	}
	if err := os.Remove(filepath.Join(c.Config().RuntimeConfig.ContainerExitsDir, ctrID)); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to remove container exit file %s: %w", ctrID, err)
	}
	c.RemoveContainer(ctr)

	if err := c.storageRuntimeServer.DeleteContainer(ctrID); err != nil {
		return "", fmt.Errorf("failed to delete storage for container %s: %w", ctrID, err)
	}

	ctr.CleanupConmonCgroup()
	c.ReleaseContainerName(ctr.Name())

	if err := c.ctrIDIndex.Delete(ctrID); err != nil {
		return "", err
	}
	return ctrID, nil
}
