//go:build darwin

// Package kernel provides helper function to get, parse and compare kernel
// versions for different platforms.
package kernel

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// GetKernelVersion gets the current kernel version.
func GetKernelVersion() (*VersionInfo, error) {
	release, err := getRelease()
	if err != nil {
		return nil, err
	}

	return ParseRelease(release)
}

// getRelease uses `system_profiler SPSoftwareDataType` to get OSX kernel version
func getRelease() (string, error) {
	cmd := exec.Command("system_profiler", "-json", "SPSoftwareDataType")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// system_profiler returns other fields in addition to kernel_version.
	// Do not decode with RejectUnknownMembers.
	var result struct {
		SPSoftwareDataType []struct {
			KernelVersion string `json:"kernel_version"`
		} `json:"SPSoftwareDataType"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parsing system_profiler JSON: %w", err)
	}
	if len(result.SPSoftwareDataType) == 0 || result.SPSoftwareDataType[0].KernelVersion == "" {
		return "", fmt.Errorf("kernel version is invalid")
	}

	prettyNames := strings.Fields(result.SPSoftwareDataType[0].KernelVersion)
	if len(prettyNames) != 2 {
		return "", fmt.Errorf("kernel version needs to be 'Darwin x.x.x' ")
	}
	return prettyNames[1], nil
}
