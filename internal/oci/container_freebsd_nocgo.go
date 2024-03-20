//go:build freebsd && !cgo
// +build freebsd,!cgo

package oci

import (
	"fmt"
	"os"
	"strings"
)

const (
	procStatusFile = "/proc/%d/status"

	// Fields from the /proc/<PID>/status file. see:
	//   https://man.freebsd.org/cgi/man.cgi?query=procfs&sektion=5
	//
	// Field no. 7, the process start time in seconds and microseconds.
	startTimeFieldIndex = 7
)

// getPidStartTime returns the process start time for a given PID.
func getPidStartTime(pid int) (string, error) {
	return getPidStatDataFromFile(fmt.Sprintf(procStatusFile, pid))
}

// getPidStatData returns the process start time for a given PID.
func getPidStatData(pid int) (string, string, error) { //nolint:gocritic // Ignore unnamedResult.
	startTime, err := getPidStartTime(pid)
	return "", startTime, err
}

// getPidStatData parses the /proc/<PID>/status file, looking for the
// process start time for a given PID. The procfs file system has to be
// mounted, which is not a requirement on FreeBSD.
//
// Note: The process state is not available via the status file on FreeBSD.
func getPidStatDataFromFile(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("unable to read status file: %w", err)
	}

	fields := strings.Fields(string(data))

	// The /proc/<PID>/status file on FreeBSD does not currently
	// include the process state.
	return fields[startTimeFieldIndex], nil
}
