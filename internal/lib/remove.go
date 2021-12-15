package lib

import (
	"context"
	"os"
	"path/filepath"

	"github.com/cri-o/cri-o/internal/oci"
	"github.com/pkg/errors"
)

// RemoveAndDeleteContainer performs the steps needed to delete a container's storage and from the runtime,
// and removes it from the container server
func (c *ContainerServer) RemoveAndDeleteContainer(ctx context.Context, ctr *oci.Container) error {
	ctrID := ctr.ID()

	if err := c.runtime.DeleteContainer(ctx, ctr); err != nil {
		return errors.Wrapf(err, "failed to delete container %s", ctrID)
	}
	if err := os.Remove(filepath.Join(c.Config().RuntimeConfig.ContainerExitsDir, ctrID)); err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "failed to remove container exit file %s", ctrID)
	}
	c.RemoveContainer(ctr)

	if err := c.storageRuntimeServer.DeleteContainer(ctrID); err != nil {
		return errors.Wrapf(err, "failed to delete storage for container %s", ctrID)
	}

	ctr.CleanupConmonCgroup()
	c.ReleaseContainerName(ctr.Name())

	if err := c.ctrIDIndex.Delete(ctrID); err != nil {
		return err
	}
	return nil
}
