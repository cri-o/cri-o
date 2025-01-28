package main

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func main() {
	// Check if Seccomp is supported, via CONFIG_SECCOMP.
	if err := unix.Prctl(unix.PR_GET_SECCOMP, 0, 0, 0, 0); !errors.Is(err, unix.EINVAL) {
		// Make sure the kernel has CONFIG_SECCOMP_FILTER.
		if err := unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, 0, 0, 0); !errors.Is(err, unix.EINVAL) {
			os.Exit(0)
		}
	}

	os.Exit(1)
}
