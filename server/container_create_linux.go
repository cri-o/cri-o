package server

import (
	"fmt"

	"github.com/containers/storage/pkg/idtools"
	ctrfactory "github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var tmpfsMounts = map[string]string{
	"/run":     "mode=0755",
	"/tmp":     "mode=1777",
	"/var/tmp": "mode=1777",
}

// createContainerPlatform performs platform dependent intermediate steps before calling the container's oci.Runtime().CreateContainer()
func (s *Server) createContainerPlatform(ctx context.Context, container *oci.Container, cgroupParent string, idMappings *idtools.IDMappings) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if idMappings != nil && !container.Spoofed() {
		rootPair := idMappings.RootPair()
		for _, path := range []string{container.BundlePath(), container.MountPoint()} {
			if err := makeAccessible(path, rootPair.UID, rootPair.GID, false); err != nil {
				return fmt.Errorf("cannot make %s accessible to %d:%d: %w", path, rootPair.UID, rootPair.GID, err)
			}
		}
		if err := makeMountsAccessible(rootPair.UID, rootPair.GID, container.Spec().Mounts); err != nil {
			return err
		}
	}
	return s.Runtime().CreateContainer(ctx, container, cgroupParent, false)
}

func getSecurityContext(containerConfig *types.ContainerConfig) *types.LinuxContainerSecurityContext {
	if containerConfig.Linux == nil {
		containerConfig.Linux = &types.LinuxContainerConfig{}
	}
	if containerConfig.Linux.SecurityContext == nil {
		containerConfig.Linux.SecurityContext = newLinuxContainerSecurityContext()
	}
	return containerConfig.Linux.SecurityContext
}

func newLinuxContainerSecurityContext() *types.LinuxContainerSecurityContext {
	return &types.LinuxContainerSecurityContext{
		Capabilities:     &types.Capability{},
		NamespaceOptions: &types.NamespaceOption{},
		SelinuxOptions:   &types.SELinuxOption{},
		RunAsUser:        &types.Int64Value{},
		RunAsGroup:       &types.Int64Value{},
		Seccomp:          &types.SecurityProfile{},
		Apparmor:         &types.SecurityProfile{},
	}
}

func (s *Server) setAppArmorProfile(ctx context.Context, ctr ctrfactory.Container, securityContext *types.LinuxContainerSecurityContext, specgen *generate.Generator) error {
	if s.Config().AppArmor().IsEnabled() && !ctr.Privileged() {
		profile, err := s.Config().AppArmor().Apply(
			securityContext.ApparmorProfile,
		)
		if err != nil {
			return fmt.Errorf("applying apparmor profile to container %s: %w", ctr.ID(), err)
		}

		log.Debugf(ctx, "Applied AppArmor profile %s to container %s", profile, ctr.ID())
		specgen.SetProcessApparmorProfile(profile)
	}
	return nil
}

func (s *Server) setSecurityContextNamespaceOptions(ctx context.Context, ctr ctrfactory.Container, containerConfig *types.ContainerConfig, sb *sandbox.Sandbox) error {
	var nsTargetCtr *oci.Container
	if target := containerConfig.Linux.SecurityContext.NamespaceOptions.TargetId; target != "" {
		nsTargetCtr = s.GetContainer(ctx, target)
	}

	if err := ctr.SpecAddNamespaces(sb, nsTargetCtr, &s.config); err != nil {
		return err
	}
	return nil
}
