// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

package findprocess

import (
	"os"
	"syscall"
)

func findProcess(pid int) (process *os.Process, err error) {
	process, err = os.FindProcess(pid)
	if err != nil {
		return process, err
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return process, nil
	}
	if err.Error() == "os: process already finished" {
		return process, ErrNotFound
	}
	return process, err
}
