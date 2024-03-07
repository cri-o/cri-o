//go:build !linux
// +build !linux

package oci

// getPidStartTime returns the process start time for a given PID.
func getPidStartTime(pid int) (string, error) {
	return "0", nil
}

// getPidStatData returns the process state and start time for a given PID.
func getPidStatData(pid int) (string, string, error) { //nolint:gocritic // Ignore unnamedResult.
	return "", "0", nil
}
