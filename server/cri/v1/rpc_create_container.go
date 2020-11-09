package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) CreateContainer(
	ctx context.Context, req *pb.CreateContainerRequest,
) (*pb.CreateContainerResponse, error) {
	r := &types.CreateContainerRequest{
		PodSandboxID:  req.PodSandboxId,
		Config:        types.NewContainerConfig(),
		SandboxConfig: types.NewPodSandboxConfig(),
	}
	if req.Config != nil {
		r.Config = &types.ContainerConfig{
			Command:     req.Config.Command,
			Args:        req.Config.Args,
			WorkingDir:  req.Config.WorkingDir,
			Labels:      req.Config.Labels,
			Annotations: req.Config.Annotations,
			LogPath:     req.Config.LogPath,
			Stdin:       req.Config.Stdin,
			StdinOnce:   req.Config.StdinOnce,
			Tty:         req.Config.Tty,
			Linux:       types.NewLinuxContainerConfig(),
		}
		if req.Config.Metadata != nil {
			r.Config.Metadata = &types.ContainerMetadata{
				Name:    req.Config.Metadata.Name,
				Attempt: req.Config.Metadata.Attempt,
			}
		}
		if req.Config.Image != nil {
			r.Config.Image = &types.ImageSpec{
				Image:       req.Config.Image.Image,
				Annotations: req.Config.Image.Annotations,
			}
		}
		if req.Config.Linux != nil {
			r.Config.Linux = types.NewLinuxContainerConfig()
			if req.Config.Linux.Resources != nil {
				r.Config.Linux.Resources = &types.LinuxContainerResources{
					CPUPeriod:          req.Config.Linux.Resources.CpuPeriod,
					CPUQuota:           req.Config.Linux.Resources.CpuQuota,
					CPUShares:          req.Config.Linux.Resources.CpuShares,
					MemoryLimitInBytes: req.Config.Linux.Resources.MemoryLimitInBytes,
					OomScoreAdj:        req.Config.Linux.Resources.OomScoreAdj,
					CPUsetCPUs:         req.Config.Linux.Resources.CpusetCpus,
					CPUsetMems:         req.Config.Linux.Resources.CpusetMems,
				}
				hugepageLimits := []*types.HugepageLimit{}
				for _, x := range req.Config.Linux.Resources.HugepageLimits {
					hugepageLimits = append(hugepageLimits, &types.HugepageLimit{
						PageSize: x.PageSize,
						Limit:    x.Limit,
					})
				}
				r.Config.Linux.Resources.HugepageLimits = hugepageLimits
			}
			if req.Config.Linux.SecurityContext != nil {
				r.Config.Linux.SecurityContext = &types.LinuxContainerSecurityContext{
					RunAsUsername:      req.Config.Linux.SecurityContext.RunAsUsername,
					ApparmorProfile:    req.Config.Linux.SecurityContext.ApparmorProfile,
					SeccompProfilePath: req.Config.Linux.SecurityContext.SeccompProfilePath,
					MaskedPaths:        req.Config.Linux.SecurityContext.MaskedPaths,
					ReadonlyPaths:      req.Config.Linux.SecurityContext.ReadonlyPaths,
					SupplementalGroups: req.Config.Linux.SecurityContext.SupplementalGroups,
					Privileged:         req.Config.Linux.SecurityContext.Privileged,
					ReadonlyRootfs:     req.Config.Linux.SecurityContext.ReadonlyRootfs,
					NoNewPrivs:         req.Config.Linux.SecurityContext.NoNewPrivs,
					Capabilities:       &types.Capability{},
					NamespaceOptions:   &types.NamespaceOption{},
					SelinuxOptions:     &types.SELinuxOption{},
				}
				if req.Config.Linux.SecurityContext.Capabilities != nil {
					r.Config.Linux.SecurityContext.Capabilities = &types.Capability{
						AddCapabilities:  req.Config.Linux.SecurityContext.Capabilities.AddCapabilities,
						DropCapabilities: req.Config.Linux.SecurityContext.Capabilities.DropCapabilities,
					}
				}
				if req.Config.Linux.SecurityContext.Seccomp != nil {
					r.Config.Linux.SecurityContext.Seccomp = &types.SecurityProfile{
						ProfileType:  types.SecurityProfileType(req.Config.Linux.SecurityContext.Seccomp.ProfileType),
						LocalhostRef: req.Config.Linux.SecurityContext.Seccomp.LocalhostRef,
					}
				}
				if req.Config.Linux.SecurityContext.Apparmor != nil {
					r.Config.Linux.SecurityContext.Apparmor = &types.SecurityProfile{
						ProfileType:  types.SecurityProfileType(req.Config.Linux.SecurityContext.Apparmor.ProfileType),
						LocalhostRef: req.Config.Linux.SecurityContext.Apparmor.LocalhostRef,
					}
				}
				if req.Config.Linux.SecurityContext.NamespaceOptions != nil {
					r.Config.Linux.SecurityContext.NamespaceOptions = &types.NamespaceOption{
						Network:  types.NamespaceMode(req.Config.Linux.SecurityContext.NamespaceOptions.Network),
						Pid:      types.NamespaceMode(req.Config.Linux.SecurityContext.NamespaceOptions.Pid),
						Ipc:      types.NamespaceMode(req.Config.Linux.SecurityContext.NamespaceOptions.Ipc),
						TargetID: req.Config.Linux.SecurityContext.NamespaceOptions.TargetId,
					}
				}
				if req.Config.Linux.SecurityContext.SelinuxOptions != nil {
					r.Config.Linux.SecurityContext.SelinuxOptions = &types.SELinuxOption{
						User:  req.Config.Linux.SecurityContext.SelinuxOptions.User,
						Role:  req.Config.Linux.SecurityContext.SelinuxOptions.Role,
						Type:  req.Config.Linux.SecurityContext.SelinuxOptions.Type,
						Level: req.Config.Linux.SecurityContext.SelinuxOptions.Level,
					}
				}
				if req.Config.Linux.SecurityContext.RunAsUser != nil {
					r.Config.Linux.SecurityContext.RunAsUser = &types.Int64Value{
						Value: req.Config.Linux.SecurityContext.RunAsUser.Value,
					}
				}
				if req.Config.Linux.SecurityContext.RunAsGroup != nil {
					r.Config.Linux.SecurityContext.RunAsGroup = &types.Int64Value{
						Value: req.Config.Linux.SecurityContext.RunAsGroup.Value,
					}
				}
			}
		}
		envs := []*types.KeyValue{}
		for _, x := range req.Config.Envs {
			envs = append(envs, &types.KeyValue{
				Key:   x.Key,
				Value: x.Value,
			})
		}
		r.Config.Envs = envs

		mounts := []*types.Mount{}
		for _, x := range req.Config.Mounts {
			mounts = append(mounts, &types.Mount{
				ContainerPath:  x.ContainerPath,
				HostPath:       x.HostPath,
				Readonly:       x.Readonly,
				SelinuxRelabel: x.SelinuxRelabel,
				Propagation:    types.MountPropagation(x.Propagation),
			})
		}
		r.Config.Mounts = mounts

		devices := []*types.Device{}
		for _, x := range req.Config.Devices {
			devices = append(devices, &types.Device{
				ContainerPath: x.ContainerPath,
				HostPath:      x.HostPath,
				Permissions:   x.Permissions,
			})
		}
		r.Config.Devices = devices
	}
	if req.SandboxConfig != nil { // nolint: dupl
		r.SandboxConfig = &types.PodSandboxConfig{
			Hostname:     req.SandboxConfig.Hostname,
			LogDirectory: req.SandboxConfig.LogDirectory,
			Labels:       req.SandboxConfig.Labels,
			Annotations:  req.SandboxConfig.Annotations,
			Linux:        types.NewLinuxPodSandboxConfig(),
		}
		if req.SandboxConfig.DnsConfig != nil {
			r.SandboxConfig.DNSConfig = &types.DNSConfig{
				Servers:  req.SandboxConfig.DnsConfig.Servers,
				Searches: req.SandboxConfig.DnsConfig.Searches,
				Options:  req.SandboxConfig.DnsConfig.Options,
			}
		}
		if req.SandboxConfig.Metadata != nil {
			r.SandboxConfig.Metadata = &types.PodSandboxMetadata{
				Name:      req.SandboxConfig.Metadata.Name,
				UID:       req.SandboxConfig.Metadata.Uid,
				Namespace: req.SandboxConfig.Metadata.Namespace,
				Attempt:   req.SandboxConfig.Metadata.Attempt,
			}
		}
		portMappings := []*types.PortMapping{}
		for _, x := range req.SandboxConfig.PortMappings {
			portMappings = append(portMappings, &types.PortMapping{
				Protocol:      types.Protocol(x.Protocol),
				ContainerPort: x.ContainerPort,
				HostPort:      x.HostPort,
				HostIP:        x.HostIp,
			})
		}
		r.SandboxConfig.PortMappings = portMappings
		if req.SandboxConfig.Linux != nil { // nolint: dupl
			r.SandboxConfig.Linux = &types.LinuxPodSandboxConfig{
				CgroupParent:    req.SandboxConfig.Linux.CgroupParent,
				Sysctls:         req.SandboxConfig.Linux.Sysctls,
				SecurityContext: types.NewLinuxSandboxSecurityContext(),
			}
			if req.SandboxConfig.Linux.SecurityContext != nil {
				r.SandboxConfig.Linux.SecurityContext = &types.LinuxSandboxSecurityContext{
					SeccompProfilePath: req.SandboxConfig.Linux.SecurityContext.SeccompProfilePath,
					SupplementalGroups: req.SandboxConfig.Linux.SecurityContext.SupplementalGroups,
					ReadonlyRootfs:     req.SandboxConfig.Linux.SecurityContext.ReadonlyRootfs,
					Privileged:         req.SandboxConfig.Linux.SecurityContext.Privileged,
					NamespaceOptions:   &types.NamespaceOption{},
					SelinuxOptions:     &types.SELinuxOption{},
				}
				if req.SandboxConfig.Linux.SecurityContext.Seccomp != nil {
					r.SandboxConfig.Linux.SecurityContext.Seccomp = &types.SecurityProfile{
						ProfileType:  types.SecurityProfileType(req.SandboxConfig.Linux.SecurityContext.Seccomp.ProfileType),
						LocalhostRef: req.SandboxConfig.Linux.SecurityContext.Seccomp.LocalhostRef,
					}
				}
				if req.SandboxConfig.Linux.SecurityContext.Apparmor != nil {
					r.SandboxConfig.Linux.SecurityContext.Apparmor = &types.SecurityProfile{
						ProfileType:  types.SecurityProfileType(req.SandboxConfig.Linux.SecurityContext.Apparmor.ProfileType),
						LocalhostRef: req.SandboxConfig.Linux.SecurityContext.Apparmor.LocalhostRef,
					}
				}
				if req.SandboxConfig.Linux.SecurityContext.NamespaceOptions != nil {
					r.SandboxConfig.Linux.SecurityContext.NamespaceOptions = &types.NamespaceOption{
						Network:  types.NamespaceMode(req.SandboxConfig.Linux.SecurityContext.NamespaceOptions.Network),
						Pid:      types.NamespaceMode(req.SandboxConfig.Linux.SecurityContext.NamespaceOptions.Pid),
						Ipc:      types.NamespaceMode(req.SandboxConfig.Linux.SecurityContext.NamespaceOptions.Ipc),
						TargetID: req.SandboxConfig.Linux.SecurityContext.NamespaceOptions.TargetId,
					}
				}
				if req.SandboxConfig.Linux.SecurityContext.SelinuxOptions != nil {
					r.SandboxConfig.Linux.SecurityContext.SelinuxOptions = &types.SELinuxOption{
						User:  req.SandboxConfig.Linux.SecurityContext.SelinuxOptions.User,
						Role:  req.SandboxConfig.Linux.SecurityContext.SelinuxOptions.Role,
						Type:  req.SandboxConfig.Linux.SecurityContext.SelinuxOptions.Type,
						Level: req.SandboxConfig.Linux.SecurityContext.SelinuxOptions.Level,
					}
				}
				if req.SandboxConfig.Linux.SecurityContext.RunAsUser != nil {
					r.SandboxConfig.Linux.SecurityContext.RunAsUser = &types.Int64Value{
						Value: req.SandboxConfig.Linux.SecurityContext.RunAsUser.Value,
					}
				}
				if req.SandboxConfig.Linux.SecurityContext.RunAsGroup != nil {
					r.SandboxConfig.Linux.SecurityContext.RunAsGroup = &types.Int64Value{
						Value: req.SandboxConfig.Linux.SecurityContext.RunAsGroup.Value,
					}
				}
			}
		}
	}
	res, err := s.server.CreateContainer(ctx, r)
	if err != nil {
		return nil, err
	}
	return &pb.CreateContainerResponse{ContainerId: res.ContainerID}, nil
}
