package oci

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"time"
)

const (
	// ContainerStateCreated represents the created state of a container
	ContainerStateCreated = "created"
	// ContainerStatePaused represents the paused state of a container
	ContainerStatePaused = "paused"
	// ContainerStateRunning represents the running state of a container
	ContainerStateRunning = "running"
	// ContainerStateStopped represents the stopped state of a container
	ContainerStateStopped = "stopped"
	// ContainerCreateTimeout represents the value of container creating timeout
	ContainerCreateTimeout = 240 * time.Second

	// CgroupfsCgroupsManager represents cgroupfs native cgroup manager
	CgroupfsCgroupsManager = "cgroupfs"
	// SystemdCgroupsManager represents systemd native cgroup manager
	SystemdCgroupsManager = "systemd"

	// BufSize is the size of buffers passed in to socekts
	BufSize = 8192

	// killContainerTimeout is the timeout that we wait for the container to
	// be SIGKILLed.
	KillContainerTimeout = 2 * time.Minute

	// minCtrStopTimeout is the minimal amout of time in seconds to wait
	// before issuing a timeout regarding the proper termination of the
	// container.
	MinCtrStopTimeout = 10
)

// PrepareProcessExec returns the path of the process.json used in runc exec -p
// caller is responsible to close the returned *os.File if needed.
func PrepareProcessExec(c *Container, cmd []string, tty bool) (*os.File, error) {
	f, err := ioutil.TempFile("", "exec-process-")
	if err != nil {
		return nil, err
	}

	pspec := c.Spec().Process
	pspec.Args = cmd
	// We need to default this to false else it will inherit terminal as true
	// from the container.
	pspec.Terminal = false
	if tty {
		pspec.Terminal = true
	}
	processJSON, err := json.Marshal(pspec)
	if err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(f.Name(), processJSON, 0644); err != nil {
		return nil, err
	}
	return f, nil
}
