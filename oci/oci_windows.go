package oci

import (
	"os"
	"os/exec"

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
