package lib

import (
	"context"
	"fmt"

	"github.com/cri-o/cri-o/internal/log"

	"github.com/cri-o/cri-o/internal/oci"
)

// ContainerStop stops a running container with a grace period (i.e., timeout).
func (c *ContainerServer) StopContainer(ctx context.Context, ctr *oci.Container, timeout int64) error {
	cStatus := ctr.StateNoLock()
	if cStatus.Status == oci.ContainerStatePaused {
		if err := c.runtime.UnpauseContainer(ctx, ctr); err != nil {
			return fmt.Errorf("failed to stop container %s: %w", ctr.ID(), err)
		}
		if err := c.runtime.UpdateContainerStatus(ctx, ctr); err != nil {
			return fmt.Errorf("failed to update container status %s: %w", ctr.ID(), err)
		}
	}
	if err := c.runtime.StopContainer(ctx, ctr, timeout); err != nil {
		// only fatally error if the error is not that the container was already stopped
		// we still want to write container state to disk if the container has already
		// been stopped
		if err != oci.ErrContainerStopped {
			return fmt.Errorf("failed to stop container %s: %w", ctr.ID(), err)
		}
	} else {
		// we only do these operations if StopContainer didn't fail (even if the failure
		// was the container already being stopped)
		if err := c.runtime.UpdateContainerStatus(ctx, ctr); err != nil {
			return fmt.Errorf("failed to update container status %s: %w", ctr.ID(), err)
		}
		if err := c.storageRuntimeServer.StopContainer(ctr.ID()); err != nil {
			return fmt.Errorf("failed to unmount container %s: %w", ctr.ID(), err)
		}
	}

	if err := c.ContainerStateToDisk(ctx, ctr); err != nil {
		log.Warnf(ctx, "Unable to write containers %s state to disk: %v", ctr.ID(), err)
	}

	return nil
}
