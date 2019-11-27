package lib

import (
	"strconv"
	"sync"
	"time"

	"github.com/containers/psgo"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// the time between checking the registered conmons
var sleepTime = 5 * time.Minute

// conmonmon is a struct responsible for monitoring conmons
// it contains a map of containers -> info on conmons, and sleeps on
// a loop, waiting for a conmon to die. if it has, it kills the associated
// container.
type conmonmon struct {
	conmons   map[*oci.Container]*conmonInfo
	closeChan chan bool
	runtime   *oci.Runtime
	server    *ContainerServer
	lock      sync.Mutex
}

// conmonInfo contains all necessary state to verify
// the conmon the container was originally spawned from is still running
type conmonInfo struct {
	conmonPID int
	startTime string
}

// newConmonmon creates a new conmonmon instance given a runtime.
// It also starts the monitoring routine
func (c *ContainerServer) newConmonmon(r *oci.Runtime) *conmonmon {
	cmm := conmonmon{
		conmons:   make(map[*oci.Container]*conmonInfo),
		runtime:   r,
		server:    c,
		closeChan: make(chan bool, 2),
	}
	go cmm.monitorConmons()
	return &cmm
}

// monitorConmons sits on a loop and sleeps.
// after waking, it signals to the conmons, checking if they're still alive.
func (c *conmonmon) monitorConmons() {
	for {
		select {
		case <-time.After(sleepTime):
			c.signalConmons()
		case <-c.closeChan:
			return
		}
	}
}

// signalConmons loops through all available conmons and they are verified as still alive
// if they're not, the container is killed and we spoof an OOM event for the container
func (c *conmonmon) signalConmons() {
	c.lock.Lock()
	for ctr, info := range c.conmons {
		if ctr.State().Status == oci.ContainerStateRunning {
			if err := c.verifyConmonValid(info.startTime, info.conmonPID); err != nil {
				logrus.Debugf("conmon pid %d invalid: %v. Killing container %s", info.conmonPID, err, ctr.ID())
				delete(c.conmons, ctr)
				// kill container in separate thread to hold the conmonmon lock as little as possible
				go c.oomKillContainer(ctr)
			}
		}
	}
	c.lock.Unlock()
}

// verifyConmonValid checks if the conmon we have saved as being associated with the container
// matches the pid. We first check if the pid is running, then check against the start time
// we originally recorded.
// These two checks should verify we are looking at the same conmon
func (c *conmonmon) verifyConmonValid(savedStart string, pid int) error {
	// check the start time is the same as we recorded (to prevent pid wrap from tricking us)
	startTime, err := getProcessStartTime(pid)
	if err != nil {
		return err
	}
	if startTime != savedStart {
		return errors.Errorf("pids found to differ in stime: recorded %s found %s", savedStart, startTime)
	}

	return nil
}

// oomKillContainer does everything required to pretend as though the container OOM'd
// this includes killing, setting its state, and writing that state to disk
func (c *conmonmon) oomKillContainer(ctr *oci.Container) {
	if err := c.runtime.SignalContainer(ctr, unix.SIGKILL); err != nil {
		logrus.Debugf(err.Error())
	}
	c.runtime.SpoofOOM(ctr)
	if err := c.server.ContainerStateToDisk(ctr); err != nil {
		logrus.Debugf(err.Error())
	}
}

// MonitorConmon adds a container's conmon to map of those watched
func (c *conmonmon) MonitorConmon(ctr *oci.Container) error {
	// silently return if we are asked to monitor a
	// runtime type that doesn't use conmon
	if runtimeType, err := c.runtime.ContainerRuntimeType(ctr); runtimeType == config.RuntimeTypeVM {
		if err != nil {
			logrus.Debugf("error when adding conmon of %s to monitoring loop: %v", ctr.ID(), err)
		}
		return nil
	}

	status := ctr.State().Status
	if status != oci.ContainerStateRunning && status != oci.ContainerStateCreated {
		return nil
	}

	conmonPID, err := oci.ReadConmonPidFile(ctr)
	if err != nil {
		return err
	}

	startTime, err := getProcessStartTime(conmonPID)
	if err != nil {
		return err
	}

	ci := &conmonInfo{
		conmonPID: conmonPID,
		startTime: startTime,
	}

	c.lock.Lock()
	if _, found := c.conmons[ctr]; found {
		c.lock.Unlock()
		return errors.Errorf("container ID: %s already has a registered conmon", ctr.ID())
	}
	c.conmons[ctr] = ci
	c.lock.Unlock()

	return nil
}

// getProcessStartTime takes a pid and runs ps against it, filtering for stime
func getProcessStartTime(pid int) (string, error) {
	psInfo, err := psgo.ProcessInfoByPids([]string{strconv.Itoa(pid)}, []string{"stime"})
	if err != nil {
		return "", err
	}
	if len(psInfo) != 2 || len(psInfo[1]) != 1 {
		return "", errors.Errorf("insufficient ps information; pid likely stopped")
	}

	return psInfo[1][0], nil
}

// StopMonitoringConmon removes a container's conmon to map of those watched
func (c *conmonmon) StopMonitoringConmon(ctr *oci.Container) {
	c.lock.Lock()
	// we can be idempotent here, because there are multiple ways a container can
	// not be tracked anymore
	delete(c.conmons, ctr)
	c.lock.Unlock()
}

// ShutdownConmonmon tells conmonmon to stop sleeping on a loop,
// and to stop monitoring
func (c *conmonmon) ShutdownConmonmon() {
	c.closeChan <- true
}
