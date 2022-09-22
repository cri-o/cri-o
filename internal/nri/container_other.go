//go:build !linux
// +build !linux

package nri

import (
	nri "github.com/containerd/nri/pkg/adaptation"
)

func linuxContainerToNRI(ctr Container) *nri.LinuxContainer {
	return nil
}
