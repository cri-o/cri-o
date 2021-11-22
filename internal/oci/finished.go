//go:build linux && !arm && !386
// +build linux,!arm,!386

package oci

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func getFinishedTime(fi os.FileInfo) (time.Time, error) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, fmt.Errorf("type assertion failed")
	}
	return time.Unix(st.Ctim.Sec, st.Ctim.Nsec), nil
}
