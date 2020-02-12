package lib

import (
	"os"
	"strconv"
	"syscall"

	"github.com/cri-o/cri-o/oci"
	"github.com/cri-o/cri-o/utils"
	"github.com/docker/docker/pkg/signal"
	"github.com/pkg/errors"
)

// Check if killSignal exists in the signal map
func inSignalMap(killSignal syscall.Signal) bool {
	for _, v := range signal.SignalMap {
		if v == killSignal {
			return true
		}
	}
	return false

}

// ContainerKill sends the user provided signal to the containers primary process.
func (c *ContainerServer) ContainerKill(container string, killSignal syscall.Signal) (string, error) { // nolint
	ctr, err := c.LookupContainer(container)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find container %s", container)
	}
	c.runtime.UpdateStatus(ctr)
	cStatus := c.runtime.ContainerStatus(ctr)

	// If the container is not running, error and move on.
	if cStatus.Status != oci.ContainerStateRunning {
		return "", errors.Errorf("cannot kill container %s: it is not running", container)
	}
	if !inSignalMap(killSignal) {
		return "", errors.Errorf("unable to find %s in the signal map", killSignal.String())
	}
	rPath, err := c.runtime.Path(ctr)
	if err != nil {
		return "", err
	}
	if err := utils.ExecCmdWithStdStreams(os.Stdin, os.Stdout, os.Stderr, rPath, "kill", ctr.ID(), strconv.Itoa(int(killSignal))); err != nil {
		return "", err
	}
	c.ContainerStateToDisk(ctr)
	return ctr.ID(), nil
}
