package lib

import (
	"github.com/cri-o/cri-o/v1/oci"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
)

func isStopped(c *ContainerServer, ctr *oci.Container) bool {
	if err := c.runtime.UpdateContainerStatus(ctr); err != nil {
		logrus.Warnf("unable to update containers %s status: %v", ctr.ID(), err)
	}
	cStatus := ctr.State()
	return cStatus.Status == oci.ContainerStateStopped
}

// ContainerWait stops a running container with a grace period (i.e., timeout).
func (c *ContainerServer) ContainerWait(container string) (int32, error) {
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to find container %s", container)
	}

	err = wait.PollImmediateInfinite(1,
		func() (bool, error) {
			return isStopped(c, ctr), nil
		},
	)

	if err != nil {
		return 0, err
	}
	exitCode := ctr.State().ExitCode
	if err := c.ContainerStateToDisk(ctr); err != nil {
		logrus.Warnf("unable to write containers %s state to disk: %v", ctr.ID(), err)
	}
	if exitCode == nil {
		return 0, errors.New("exit code not set")
	}
	return *exitCode, nil
}
