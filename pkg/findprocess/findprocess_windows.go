package findprocess

import (
	"os"
)

func findProcess(pid int) (process *os.Process, err error) {
	process, err := os.FindProcess(pid)
	if err != nil {
		// FIXME: is there an analog to POSIX's ESRCH we can check for?
		return process, err
	}
	return process, nil
}
