//go:build linux

package nri

import (
	nri "github.com/containerd/nri/pkg/adaptation"
)

func linuxContainerToNRI(ctr Container) *nri.LinuxContainer {
	lnx := ctr.GetLinuxContainer()
	return &nri.LinuxContainer{
		Namespaces:  lnx.GetLinuxNamespaces(),
		Devices:     lnx.GetLinuxDevices(),
		Resources:   lnx.GetLinuxResources(),
		OomScoreAdj: nri.Int(lnx.GetOOMScoreAdj()),
		CgroupsPath: lnx.GetCgroupsPath(),
	}
}
