//go:build freebsd && !cgo
// +build freebsd,!cgo

package oci

import (
	"fmt"
	"os"
	"strings"
)

// Reads the process start time via /proc
func getPidStartTime(pid int) (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return "", fmt.Errorf("%v: %w", err, ErrNotFound)
	}
	fields := strings.Fields(string(data))
	return fields[7], nil
}

// getPidData reads the kernel's /proc entry for various data.
func getPidData(pid int) (*StatData, error) {
	startTime, err := getPidStartTime(pid)
	if err != nil {
		return nil, err
	}
	return &StatData{
		StartTime: startTime,
		State:     "not implemented",
	}, nil
}
