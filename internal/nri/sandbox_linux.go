//go:build linux
// +build linux

package nri

import (
	nri "github.com/containerd/nri/pkg/adaptation"
)

func podSandboxToNRI(pod PodSandbox) *nri.PodSandbox {
	nriPod := commonPodSandboxToNRI(pod)
	lnxPod := pod.GetLinuxPodSandbox()
	nriPod.Linux = &nri.LinuxPodSandbox{
		Namespaces:   lnxPod.GetLinuxNamespaces(),
		PodOverhead:  lnxPod.GetPodLinuxOverhead(),
		PodResources: lnxPod.GetPodLinuxResources(),
		CgroupParent: lnxPod.GetCgroupParent(),
		CgroupsPath:  lnxPod.GetCgroupsPath(),
		Resources:    lnxPod.GetLinuxResources(),
	}
	return nriPod
}
