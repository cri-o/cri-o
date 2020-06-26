// +build !windows

package oci

import (
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/containers/libpod/pkg/cgroups"
	"github.com/containers/storage/pkg/pools"
	"github.com/creack/pty"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"k8s.io/client-go/tools/remotecommand"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

func kill(pid int) error {
	err := unix.Kill(pid, unix.SIGKILL)
	if err != nil && err != unix.ESRCH {
		return fmt.Errorf("failed to kill process: %v", err)
	}
	return nil
}

func calculateCPUPercent(stats *cgroups.Metrics) float64 {
	return genericCalculateCPUPercent(stats.CPU.Usage.Total, stats.CPU.Usage.PerCPU)
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

func setSize(fd uintptr, size remotecommand.TerminalSize) error {
	winsize := &unix.Winsize{Row: size.Height, Col: size.Width}
	return unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, winsize)
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
		if err := setSize(p.Fd(), size); err != nil {
			logrus.Warnf("unable to set terminal size: %v", err)
		}
	})

	var stdinErr, stdoutErr error
	if stdin != nil {
		go func() { _, stdinErr = pools.Copy(p, stdin) }()
	}

	if stdout != nil {
		go func() { _, stdoutErr = pools.Copy(stdout, p) }()
	}

	err = execCmd.Wait()

	if stdinErr != nil {
		logrus.Warnf("stdin copy error: %v", stdinErr)
	}
	if stdoutErr != nil {
		logrus.Warnf("stdout copy error: %v", stdoutErr)
	}

	return err
}
