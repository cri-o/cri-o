// +build !windows

package oci

import (
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/docker/docker/pkg/pools"
	"github.com/kr/pty"
	"github.com/opencontainers/runc/libcontainer"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/remotecommand"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/util/term"
)

const (
	// ContainerExitsDir is the location of container exit dirs
	ContainerExitsDir = "/var/run/crio/exits"
	// ContainerAttachSocketDir is the location for container attach sockets
	ContainerAttachSocketDir = "/var/run/crio"
)

func kill(pid int) error {
	err := unix.Kill(pid, unix.SIGKILL)
	if err != nil && err != unix.ESRCH {
		return fmt.Errorf("failed to kill process: %v", err)
	}
	return nil
}

func getExitCode(err error) int32 {
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(unix.WaitStatus); ok {
			return int32(status.ExitStatus())
		}
	}
	return -1
}

func calculateCPUPercent(stats *libcontainer.Stats) float64 {
	return genericCalculateCPUPercent(stats.CgroupStats.CpuStats.CpuUsage.TotalUsage,
		stats.CgroupStats.CpuStats.CpuUsage.PercpuUsage)
}

func genericCalculateCPUPercent(cpuTotal uint64, perCPU []uint64) float64 {
	var (
		cpuPercent = 0.0
		cpuUsage   = float64(cpuTotal)
		systemTime = float64(uint64(time.Now().UnixNano()))
	)
	if systemTime > 0.0 && cpuUsage > 0.0 {
		// gets a ratio of container cpu usage total, multiplies it by the number of cores (4 cores running
		// at 100% utilization should be 400% utilization), and multiplies that by 100 to get a percentage
		cpuPercent = (cpuUsage / systemTime) * float64(len(perCPU)) * 100
	}
	return cpuPercent
}

func ttyCmd(execCmd *exec.Cmd, stdin io.Reader, stdout io.WriteCloser, resize <-chan remotecommand.TerminalSize) error {
	p, err := pty.Start(execCmd)
	if err != nil {
		return err
	}
	defer p.Close()

	// make sure to close the stdout stream
	defer stdout.Close()

	kubecontainer.HandleResizing(resize, func(size remotecommand.TerminalSize) {
		term.SetSize(p.Fd(), size)
	})

	if stdin != nil {
		go pools.Copy(p, stdin)
	}

	if stdout != nil {
		go pools.Copy(stdout, p)
	}

	return execCmd.Wait()
}
