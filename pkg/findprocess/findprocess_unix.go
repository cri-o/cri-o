// +build !windows

package findprocess

import (
	"os"
	"syscall"
)

func findProcess(pid int) (process *os.Process, err error) {
	// On Unix systems, FindProcess always succeeds and returns a Process
	// for the given pid, regardless of whether the process exists.
	process, _ = os.FindProcess(pid)
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return process, nil
	}
	if err.Error() == "os: process already finished" {
		return process, ErrNotFound
	}
	return process, err
}
