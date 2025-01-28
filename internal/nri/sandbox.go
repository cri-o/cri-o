package nri

import (
	nri "github.com/containerd/nri/pkg/adaptation"
)

// PodSandbox interface for interacting with NRI.
type PodSandbox interface {
	GetDomain() string

	GetID() string
	GetName() string
	GetUID() string
	GetNamespace() string
	GetLabels() map[string]string
	GetAnnotations() map[string]string
	GetRuntimeHandler() string
	GetLinuxPodSandbox() LinuxPodSandbox

	GetPid() uint32
	GetIPs() []string
}

type LinuxPodSandbox interface {
	GetLinuxNamespaces() []*nri.LinuxNamespace
	GetPodLinuxOverhead() *nri.LinuxResources
	GetPodLinuxResources() *nri.LinuxResources
	GetCgroupParent() string
	GetCgroupsPath() string
	GetLinuxResources() *nri.LinuxResources
}

func commonPodSandboxToNRI(pod PodSandbox) *nri.PodSandbox {
	return &nri.PodSandbox{
		Id:             pod.GetID(),
		Name:           pod.GetName(),
		Uid:            pod.GetUID(),
		Namespace:      pod.GetNamespace(),
		Labels:         pod.GetLabels(),
		Annotations:    pod.GetAnnotations(),
		RuntimeHandler: pod.GetRuntimeHandler(),
		Pid:            pod.GetPid(),
		Ips:            pod.GetIPs(),
	}
}

func podSandboxesToNRI(podList []PodSandbox) []*nri.PodSandbox {
	pods := []*nri.PodSandbox{}
	for _, pod := range podList {
		pods = append(pods, podSandboxToNRI(pod))
	}

	return pods
}
