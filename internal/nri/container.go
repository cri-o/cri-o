package nri

import (
	nri "github.com/containerd/nri/pkg/adaptation"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// Container interface for interacting with NRI.
type Container interface {
	GetDomain() string

	GetPodSandboxID() string
	GetID() string
	GetName() string
	GetState() nri.ContainerState
	GetLabels() map[string]string
	GetAnnotations() map[string]string
	GetArgs() []string
	GetEnv() []string
	GetMounts() []*nri.Mount
	GetHooks() *nri.Hooks
	GetLinuxContainer() LinuxContainer

	GetSpec() *specs.Spec
}

type LinuxContainer interface {
	GetLinuxNamespaces() []*nri.LinuxNamespace
	GetLinuxDevices() []*nri.LinuxDevice
	GetLinuxResources() *nri.LinuxResources
	GetOOMScoreAdj() *int
	GetCgroupsPath() string
	GetIOPriority() *nri.LinuxIOPriority
	GetScheduler() *nri.LinuxScheduler
	GetNetDevices() map[string]*nri.LinuxNetDevice
	GetRdt() *nri.LinuxRdt
}

func containerToNRI(ctr Container) *nri.Container {
	return &nri.Container{
		Id:           ctr.GetID(),
		PodSandboxId: ctr.GetPodSandboxID(),
		Name:         ctr.GetName(),
		State:        ctr.GetState(),
		Labels:       ctr.GetLabels(),
		Annotations:  ctr.GetAnnotations(),
		Args:         ctr.GetArgs(),
		Env:          ctr.GetEnv(),
		Mounts:       ctr.GetMounts(),
		Hooks:        ctr.GetHooks(),
		Linux:        linuxContainerToNRI(ctr),
	}
}

func containersToNRI(ctrList []Container) []*nri.Container {
	ctrs := []*nri.Container{}
	for _, ctr := range ctrList {
		ctrs = append(ctrs, containerToNRI(ctr))
	}

	return ctrs
}
