package v1alpha2

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

func (s *service) PullImage(
	ctx context.Context, req *pb.PullImageRequest,
) (*pb.PullImageResponse, error) {
	r := &types.PullImageRequest{
		Image: &types.ImageSpec{},
		Auth:  &types.AuthConfig{},
		SandboxConfig: &types.PodSandboxConfig{
			Metadata:     &types.PodSandboxMetadata{},
			DNSConfig:    &types.DNSConfig{},
			PortMappings: []*types.PortMapping{},
			Linux:        types.NewLinuxPodSandboxConfig(),
		},
	}
	if req.Image != nil {
		r.Image = &types.ImageSpec{
			Image:       req.Image.Image,
			Annotations: req.Image.Annotations,
		}
	}
	if req.Auth != nil {
		r.Auth = &types.AuthConfig{
			Username:      req.Auth.Username,
			Password:      req.Auth.Password,
			Auth:          req.Auth.Auth,
			ServerAddress: req.Auth.ServerAddress,
			IdentityToken: req.Auth.IdentityToken,
			RegistryToken: req.Auth.RegistryToken,
		}
	}
	if req.SandboxConfig != nil {
		r.SandboxConfig = &types.PodSandboxConfig{
			Hostname:     req.SandboxConfig.Hostname,
			LogDirectory: req.SandboxConfig.LogDirectory,
			Labels:       req.SandboxConfig.Labels,
			Annotations:  req.SandboxConfig.Annotations,
			Metadata:     &types.PodSandboxMetadata{},
			DNSConfig:    &types.DNSConfig{},
			PortMappings: []*types.PortMapping{},
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
	res, err := s.server.PullImage(ctx, r)
	if err != nil {
		return nil, err
	}
	return &pb.PullImageResponse{ImageRef: res.ImageRef}, nil
}
