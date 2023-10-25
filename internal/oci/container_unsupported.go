//go:build !linux
// +build !linux

package oci

func getPidStartTime(pid int) (string, error) {
	return "0", nil
}
