//go:build !linux
// +build !linux

package oci

import (
	"os"
	"time"
)

func getFinishedTime(fi os.FileInfo) (time.Time, error) {
	// Windows would be like
	// st := fi.Sys().(*syscall.Win32FileAttributeDatao)
	// st.CreationTime.Nanoseconds()

	// Darwin and Freebsd would be like
	// st := fi.Sys().(*syscall.Stat_t)
	// st.Ctimespec.Nsec

	// openbsd would be like
	// st := fi.Sys().(*syscall.Stat_t)
	// st.Ctim.Nsec
	return fi.ModTime(), nil
}
