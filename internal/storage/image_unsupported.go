//go:build !linux
// +build !linux

package storage

import (
	"fmt"
	"runtime"
)

// moveSelfToCgroup moves the current process to a new transient cgroup.
func moveSelfToCgroup(cgroup string) error {
	return fmt.Errorf("moveSelfToCgroup unsupported on %s", runtime.GOOS)
}
