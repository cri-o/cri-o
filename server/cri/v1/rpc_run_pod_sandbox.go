package v1

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *service) RunPodSandbox(
	ctx context.Context, req *pb.RunPodSandboxRequest,
) (*pb.RunPodSandboxResponse, error) {
	r := &types.RunPodSandboxRequest{
		Config:         types.NewPodSandboxConfig(),
		RuntimeHandler: req.RuntimeHandler,
	}
	if req.Config != nil { // nolint: dupl
		r.Config = &types.PodSandboxConfig{
			Hostname:     req.Config.Hostname,
			LogDirectory: req.Config.LogDirectory,
			Labels:       req.Config.Labels,
			Annotations:  req.Config.Annotations,
			Linux:        types.NewLinuxPodSandboxConfig(),
		}
		if req.Config.DnsConfig != nil {
			r.Config.DNSConfig = &types.DNSConfig{
				Servers:  req.Config.DnsConfig.Servers,
				Searches: req.Config.DnsConfig.Searches,
				Options:  req.Config.DnsConfig.Options,
			}
		}
		if req.Config.Metadata != nil {
			r.Config.Metadata = &types.PodSandboxMetadata{
				Name:      req.Config.Metadata.Name,
				UID:       req.Config.Metadata.Uid,
				Namespace: req.Config.Metadata.Namespace,
				Attempt:   req.Config.Metadata.Attempt,
			}
		}
		portMappings := []*types.PortMapping{}
		for _, x := range req.Config.PortMappings {
			portMappings = append(portMappings, &types.PortMapping{
				Protocol:      types.Protocol(x.Protocol),
				ContainerPort: x.ContainerPort,
				HostPort:      x.HostPort,
				HostIP:        x.HostIp,
			})
		}
		r.Config.PortMappings = portMappings
		if req.Config.Linux != nil { // nolint: dupl
			r.Config.Linux = &types.LinuxPodSandboxConfig{
				CgroupParent:    req.Config.Linux.CgroupParent,
				Sysctls:         req.Config.Linux.Sysctls,
				SecurityContext: types.NewLinuxSandboxSecurityContext(),
			}
			if req.Config.Linux.SecurityContext != nil {
				r.Config.Linux.SecurityContext = &types.LinuxSandboxSecurityContext{
					SeccompProfilePath: req.Config.Linux.SecurityContext.SeccompProfilePath,
					SupplementalGroups: req.Config.Linux.SecurityContext.SupplementalGroups,
					ReadonlyRootfs:     req.Config.Linux.SecurityContext.ReadonlyRootfs,
					Privileged:         req.Config.Linux.SecurityContext.Privileged,
					NamespaceOptions:   &types.NamespaceOption{},
					SelinuxOptions:     &types.SELinuxOption{},
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
	}
	res, err := s.server.RunPodSandbox(ctx, r)
	if err != nil {
		return nil, err
	}
	return &pb.RunPodSandboxResponse{PodSandboxId: res.PodSandboxID}, nil
}
