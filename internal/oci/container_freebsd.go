//go:build freebsd && cgo
// +build freebsd,cgo

package oci

// #include <sys/types.h>
// #include <sys/user.h>
import "C"

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Reads the process start time using sysctl. This is marginally more efficient
// than /proc but more importantly, it works when /proc is not mounted which is
// normal.
func getPidStartTime(pid int) (string, error) {
	data, err := unix.SysctlRaw("kern.proc.pid", pid)
	if err != nil {
		return "", err
	}

	if len(data) != C.sizeof_struct_kinfo_proc {
		return "", fmt.Errorf("unexpected size %d", len(data))
	}
	kp := (*C.struct_kinfo_proc)(unsafe.Pointer(&data[0]))
	return fmt.Sprintf("%d,%d", kp.ki_start.tv_sec, kp.ki_start.tv_usec), nil
}
