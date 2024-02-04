package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/mount"
	ctrfactory "github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	bindMountType = "bind"
)

var (
	bindMountOptions = []string{"bind"}

	tmpfsMounts = map[string]string{
		"/run":     "mode=0755",
		"/tmp":     "mode=1777",
		"/var/tmp": "mode=1777",
	}
)

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

func addSysfsMounts(ctr ctrfactory.Container, containerConfig *types.ContainerConfig, hostNet bool) {
	// If the sandbox is configured to run in the host network, do not create a new network namespace
	if hostNet {
		if !isInCRIMounts("/sys", containerConfig.Mounts) {
			ctr.SpecAddMount(rspec.Mount{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "ro"},
			})
			ctr.SpecAddMount(rspec.Mount{
				Destination: cgroupSysFsPath,
				Type:        "cgroup",
				Source:      "cgroup",
				Options:     []string{"nosuid", "noexec", "nodev", "relatime", "ro"},
			})
		}
	}

	if ctr.Privileged() {
		ctr.SpecAddMount(rspec.Mount{
			Destination: "/sys",
			Type:        "sysfs",
			Source:      "sysfs",
			Options:     []string{"nosuid", "noexec", "nodev", "rw", "rslave"},
		})
		ctr.SpecAddMount(rspec.Mount{
			Destination: cgroupSysFsPath,
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"nosuid", "noexec", "nodev", "rw", "relatime", "rslave"},
		})
	}
}

func setOCIBindMountsPrivileged(g *generate.Generator) {
	spec := g.Config
	// clear readonly for /sys and cgroup
	for i := range spec.Mounts {
		clearReadOnly(&spec.Mounts[i])
	}
	spec.Linux.ReadonlyPaths = nil
	spec.Linux.MaskedPaths = nil
}

func clearReadOnly(m *rspec.Mount) {
	var opt []string
	for _, o := range m.Options {
		if o == "rw" {
			return
		} else if o != "ro" {
			opt = append(opt, o)
		}
	}
	m.Options = opt
	m.Options = append(m.Options, "rw")
}

func addOCIBindMounts(ctx context.Context, ctr ctrfactory.Container, mountLabel, bindMountPrefix string, absentMountSourcesToReject []string, maybeRelabel, skipRelabel, cgroup2RW, idMapSupport bool, storageRoot string) ([]oci.ContainerVolume, []rspec.Mount, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	volumes := []oci.ContainerVolume{}
	ociMounts := []rspec.Mount{}
	containerConfig := ctr.Config()
	specgen := ctr.Spec()
	mounts := containerConfig.Mounts

	// Sort mounts in number of parts. This ensures that high level mounts don't
	// shadow other mounts.
	sort.Sort(criOrderedMounts(mounts))

	// Copy all mounts from default mounts, except for
	// - mounts overridden by supplied mount;
	// - all mounts under /dev if a supplied /dev is present.
	mountSet := make(map[string]struct{})
	for _, m := range mounts {
		mountSet[filepath.Clean(m.ContainerPath)] = struct{}{}
	}
	defaultMounts := specgen.Mounts()
	specgen.ClearMounts()
	for _, m := range defaultMounts {
		dst := filepath.Clean(m.Destination)
		if _, ok := mountSet[dst]; ok {
			// filter out mount overridden by a supplied mount
			continue
		}
		if _, mountDev := mountSet["/dev"]; mountDev && strings.HasPrefix(dst, "/dev/") {
			// filter out everything under /dev if /dev is a supplied mount
			continue
		}
		if _, mountSys := mountSet["/sys"]; mountSys && strings.HasPrefix(dst, "/sys/") {
			// filter out everything under /sys if /sys is a supplied mount
			continue
		}
		specgen.AddMount(m)
	}

	mountInfos, err := mount.GetMounts()
	if err != nil {
		return nil, nil, err
	}
	for _, m := range mounts {
		dest := m.ContainerPath
		if dest == "" {
			return nil, nil, fmt.Errorf("mount.ContainerPath is empty")
		}
		if m.HostPath == "" {
			return nil, nil, fmt.Errorf("mount.HostPath is empty")
		}
		if m.HostPath == "/" && dest == "/" {
			log.Warnf(ctx, "Configuration specifies mounting host root to the container root.  This is dangerous (especially with privileged containers) and should be avoided.")
		}

		if isSubDirectoryOf(storageRoot, m.HostPath) && m.Propagation == types.MountPropagation_PROPAGATION_PRIVATE {
			log.Infof(ctx, "Mount propogration for the host path %s will be set to HostToContainer as it includes the container storage root", m.HostPath)
			m.Propagation = types.MountPropagation_PROPAGATION_HOST_TO_CONTAINER
		}

		src := filepath.Join(bindMountPrefix, m.HostPath)

		resolvedSrc, err := resolveSymbolicLink(bindMountPrefix, src)
		if err == nil {
			src = resolvedSrc
		} else {
			if !os.IsNotExist(err) {
				return nil, nil, fmt.Errorf("failed to resolve symlink %q: %w", src, err)
			}
			for _, toReject := range absentMountSourcesToReject {
				if filepath.Clean(src) == toReject {
					// special-case /etc/hostname, as we don't want it to be created as a directory
					// This can cause issues with node reboot.
					return nil, nil, fmt.Errorf("cannot mount %s: path does not exist and will cause issues as a directory", toReject)
				}
			}
			if !ctr.Restore() {
				// Although this would also be really helpful for restoring containers
				// it is problematic as during restore external bind mounts need to be
				// a file if the destination is a file. Unfortunately it is not easy
				// to tell if the destination is a file or a directory. Especially if
				// the destination is a nested bind mount. For now we will just not
				// create the missing bind mount source for restore and return an
				// error to the user.
				if err = os.MkdirAll(src, 0o755); err != nil {
					return nil, nil, fmt.Errorf("failed to mkdir %s: %s", src, err)
				}
			}
		}

		options := []string{"rw"}
		if m.Readonly {
			options = []string{"ro"}
		}
		options = append(options, "rbind")

		// mount propagation
		switch m.Propagation {
		case types.MountPropagation_PROPAGATION_PRIVATE:
			options = append(options, "rprivate")
			// Since default root propagation in runc is rprivate ignore
			// setting the root propagation
		case types.MountPropagation_PROPAGATION_BIDIRECTIONAL:
			if err := ensureShared(src, mountInfos); err != nil {
				return nil, nil, err
			}
			options = append(options, "rshared")
			if err := specgen.SetLinuxRootPropagation("rshared"); err != nil {
				return nil, nil, err
			}
		case types.MountPropagation_PROPAGATION_HOST_TO_CONTAINER:
			if err := ensureSharedOrSlave(src, mountInfos); err != nil {
				return nil, nil, err
			}
			options = append(options, "rslave")
			if specgen.Config.Linux.RootfsPropagation != "rshared" &&
				specgen.Config.Linux.RootfsPropagation != "rslave" {
				if err := specgen.SetLinuxRootPropagation("rslave"); err != nil {
					return nil, nil, err
				}
			}
		default:
			log.Warnf(ctx, "Unknown propagation mode for hostPath %q", m.HostPath)
			options = append(options, "rprivate")
		}

		if m.SelinuxRelabel {
			if skipRelabel {
				log.Debugf(ctx, "Skipping relabel for %s because of super privileged container (type: spc_t)", src)
			} else if err := securityLabel(src, mountLabel, false, maybeRelabel); err != nil {
				return nil, nil, err
			}
		} else {
			log.Debugf(ctx, "Skipping relabel for %s because kubelet did not request it", src)
		}

		volumes = append(volumes, oci.ContainerVolume{
			ContainerPath:  dest,
			HostPath:       src,
			Readonly:       m.Readonly,
			Propagation:    m.Propagation,
			SelinuxRelabel: m.SelinuxRelabel,
		})

		uidMappings := getOCIMappings(m.UidMappings)
		gidMappings := getOCIMappings(m.GidMappings)
		if (uidMappings != nil || gidMappings != nil) && !idMapSupport {
			return nil, nil, fmt.Errorf("idmap mounts specified but OCI runtime does not support them. Perhaps the OCI runtime is too old")
		}
		ociMounts = append(ociMounts, rspec.Mount{
			Source:      src,
			Destination: dest,
			Options:     options,
			UIDMappings: uidMappings,
			GIDMappings: gidMappings,
		})
	}

	if _, mountSys := mountSet["/sys"]; !mountSys {
		m := rspec.Mount{
			Destination: cgroupSysFsPath,
			Type:        "cgroup",
			Source:      "cgroup",
			Options:     []string{"nosuid", "noexec", "nodev", "relatime"},
		}

		if cgroup2RW {
			m.Options = append(m.Options, "rw")
		} else {
			m.Options = append(m.Options, "ro")
		}
		specgen.AddMount(m)
	}

	return volumes, ociMounts, nil
}

func getOCIMappings(m []*types.IDMapping) []rspec.LinuxIDMapping {
	if len(m) == 0 {
		return nil
	}
	ids := make([]rspec.LinuxIDMapping, 0, len(m))
	for _, m := range m {
		ids = append(ids, rspec.LinuxIDMapping{
			ContainerID: m.ContainerId,
			HostID:      m.HostId,
			Size:        m.Length,
		})
	}
	return ids
}

func addShmMount(ctr ctrfactory.Container, sb *sandbox.Sandbox) {
	ctr.SpecAddMount(rspec.Mount{
		Destination: "/dev/shm",
		Type:        "bind",
		Source:      sb.ShmPath(),
		Options:     []string{"rw", "bind"},
	})
}
