package server

import (
	"fmt"

	"github.com/opencontainers/selinux/go-selinux"
)

// KVMLabel returns labels for running kvm isolated containers.
func KVMLabel(cLabel string) (string, error) {
	if cLabel == "" {
		// selinux is disabled
		return "", nil
	}

	processLabel, err := selinux.KVMContainerLabel()
	if err != nil {
		return "", fmt.Errorf("get KVM container label: %w", err)
	}

	selinux.ReleaseLabel(processLabel)

	return swapSELinuxLabel(cLabel, processLabel)
}

// InitLabel returns labels for running systemd based containers.
func InitLabel(cLabel string) (string, error) {
	if cLabel == "" {
		// selinux is disabled
		return "", nil
	}

	processLabel, err := selinux.InitContainerLabel()
	if err != nil {
		return "", fmt.Errorf("get init container label: %w", err)
	}

	selinux.ReleaseLabel(processLabel)

	return swapSELinuxLabel(cLabel, processLabel)
}

func swapSELinuxLabel(cLabel, processLabel string) (string, error) {
	dcon, err := selinux.NewContext(cLabel)
	if err != nil {
		return "", err
	}

	scon, err := selinux.NewContext(processLabel)
	if err != nil {
		return "", err
	}

	dcon["type"] = scon["type"]

	return dcon.Get(), nil
}
