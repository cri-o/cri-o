package oci

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/containers/common/pkg/resize"
	"golang.org/x/sys/windows"
)

func kill(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func getExitCode(err error) int32 {
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(windows.WaitStatus); ok {
			return int32(status.ExitStatus())
		}
	}
	return -1
}

func ttyCmd(cmd *exec.Cmd, stdin io.Reader, stdout io.WriteCloser, resizeChan <-chan resize.TerminalSize) error {
	return fmt.Errorf("unsupported")
}
