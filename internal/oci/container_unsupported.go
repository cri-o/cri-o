//go:build !linux && !freebsd
// +build !linux,!freebsd

package oci

func getPidStartTime(pid int) (string, error) {
	return "0", nil
}
