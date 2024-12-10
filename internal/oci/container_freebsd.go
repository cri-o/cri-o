//go:build freebsd && cgo

package oci

// #include <sys/types.h>
// #include <sys/user.h>
import "C"

import (
	"fmt"
	"unsafe"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
)

const sysctlName = "kern.proc.pid"

// getPidStartTime returns the process start time for a given PID.
func getPidStartTime(pid int) (string, error) {
	return getPidStatDataFromSysctl(pid)
}

// getPidStatData returns the process state and start time for a given PID.
//
// TODO: Return the process state using struct kinfo_proc.
func getPidStatData(pid int) (string, string, error) { //nolint:gocritic // Ignore unnamedResult.
	startTime, err := getPidStartTime(pid)
	return "", startTime, err
}

// getPidStatDataFromSysctl reads the process start time using sysctl. This
// is marginally more efficient than /proc but more importantly, it works
// when /proc is not mounted, which not a requirement on FreeBSD.
//
// TODO: Add process state collection using struct kinfo_proc.
func getPidStatDataFromSysctl(pid int) (string, error) {
	data, err := unix.SysctlRaw(sysctlName, pid)
	if err != nil {
		return "", err
	}

	if len(data) != C.sizeof_struct_kinfo_proc {
		return "", fmt.Errorf("incorrect read of %d bytes from %s sysctl", len(data), sysctlName)
	}

	kp := (*C.struct_kinfo_proc)(unsafe.Pointer(&data[0]))

	return fmt.Sprintf("%d,%d", kp.ki_start.tv_sec, kp.ki_start.tv_usec), nil
}

// SetRuntimeUser sets the runtime user for the container.
func (c *Container) SetRuntimeUser(spec *specs.Spec) {
	// No-op.
}
