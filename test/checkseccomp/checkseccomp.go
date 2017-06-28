package main

import (
	"os"

	"golang.org/x/sys/unix"
)

const (
	// SeccompModeFilter refers to the unix argument SECCOMP_MODE_FILTER.
	SeccompModeFilter = uintptr(2)
)

func main() {
	// Check if Seccomp is supported, via CONFIG_SECCOMP.
	if _, _, err := unix.RawSyscall(unix.SYS_PRCTL, unix.PR_GET_SECCOMP, 0, 0); err != unix.EINVAL {
		// Make sure the kernel has CONFIG_SECCOMP_FILTER.
		if _, _, err := unix.RawSyscall(unix.SYS_PRCTL, unix.PR_SET_SECCOMP, SeccompModeFilter, 0); err != unix.EINVAL {
			os.Exit(0)
		}
	}
	os.Exit(1)
}
