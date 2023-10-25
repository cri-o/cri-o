//go:build !linux
// +build !linux

package iptables

import (
	"fmt"
	"runtime"
)

func grabIptablesLocks(lockfilePath14x, lockfilePath16x string) (iptablesLocker, error) {
	return nil, fmt.Errorf("iptables not supported on %s", runtime.GOOS)
}
