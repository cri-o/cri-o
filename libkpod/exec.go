package libkpod

import (
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/pkg/errors"
)

// ContainerExec runs a command in a running container
func (c *ContainerServer) ContainerExec(container string, command []string, detach bool, environmentVars []string, tty bool, user string) error {
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return errors.Wrapf(err, "failed to find container %s", container)
	}
	ctrID := ctr.ID()

	cStatus := c.runtime.ContainerStatus(ctr)
	switch cStatus.Status {
	case oci.ContainerStatePaused:
		return errors.Errorf("cannot run a command in a paused container %s", ctrID)
	case oci.ContainerStateRunning:
		if err := c.runtime.ExecContainerCommand(ctr, command, detach, environmentVars, tty, user); err != nil {
			return errors.Wrapf(err, "failed to exec command in container %s", ctrID)
		}
	default:
		return errors.Errorf("container %s is not running", ctrID)
	}

	return nil
}
