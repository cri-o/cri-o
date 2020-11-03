package system

import (
	"os"
	"syscall"
)

func Chmod(name string, mode os.FileMode) error {
	err := os.Chmod(name, mode)

	for isEINTR(err) {
		err = os.Chmod(name, mode)
	}

	return err
}

func isEINTR(err error) bool {
	if err == nil {
		return false
	}
	pathErr, ok := err.(*os.PathError)
	if !ok {
		return false
	}
	syscallErr, ok := pathErr.Err.(*syscall.Errno)
	if !ok {
		return false
	}
	return *syscallErr == syscall.EINTR
}
