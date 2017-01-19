package main

import (
	"os"
	"syscall"
)

const (
	// SeccompModeFilter refers to the syscall argument SECCOMP_MODE_FILTER.
	SeccompModeFilter = uintptr(2)
)

func main() {
	// Check if Seccomp is supported, via CONFIG_SECCOMP.
	if _, _, err := syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_GET_SECCOMP, 0, 0); err != syscall.EINVAL {
		// Make sure the kernel has CONFIG_SECCOMP_FILTER.
		if _, _, err := syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_SET_SECCOMP, SeccompModeFilter, 0); err != syscall.EINVAL {
			os.Exit(0)
		}
	}
	os.Exit(1)
}
