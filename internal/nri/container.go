package nri

import (
	nri "github.com/containerd/nri/pkg/adaptation"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// ContainerStatus represents the status of a container.
type ContainerStatus struct {
	State      nri.ContainerState
	Reason     string
	Message    string
	Pid        uint32
	CreatedAt  int64
	StartedAt  int64
	FinishedAt int64
	ExitCode   int32
}

// Container interface for interacting with NRI.
type Container interface {
	GetDomain() string

	GetPodSandboxID() string
	GetID() string
	GetName() string
	GetStatus() *ContainerStatus
	GetLabels() map[string]string
	GetAnnotations() map[string]string
	GetArgs() []string
	GetEnv() []string
	GetMounts() []*nri.Mount
	GetHooks() *nri.Hooks
	GetLinuxContainer() LinuxContainer
	GetUser() *nri.User
	GetRlimits() []*nri.POSIXRlimit

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
	status := ctr.GetStatus()

	return &nri.Container{
		Id:           ctr.GetID(),
		PodSandboxId: ctr.GetPodSandboxID(),
		Name:         ctr.GetName(),
		State:        status.State,
		Labels:       ctr.GetLabels(),
		Annotations:  ctr.GetAnnotations(),
		Args:         ctr.GetArgs(),
		Env:          ctr.GetEnv(),
		Mounts:       ctr.GetMounts(),
		Hooks:        ctr.GetHooks(),
		Linux:        linuxContainerToNRI(ctr),
		User:         ctr.GetUser(),
		Rlimits:      ctr.GetRlimits(),
		Pid:          status.Pid,
		CreatedAt:    status.CreatedAt,
		StartedAt:    status.StartedAt,
		FinishedAt:   status.FinishedAt,
		ExitCode:     status.ExitCode,
	}
}

func containersToNRI(ctrList []Container) []*nri.Container {
	ctrs := []*nri.Container{}
	for _, ctr := range ctrList {
		ctrs = append(ctrs, containerToNRI(ctr))
	}

	return ctrs
}
