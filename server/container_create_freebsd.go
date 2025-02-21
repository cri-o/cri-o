package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/containers/storage/pkg/idtools"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"golang.org/x/net/context"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/device"
	ctrfactory "github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	oci "github.com/cri-o/cri-o/internal/oci"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
)

// finalizeUserMapping changes the UID, GID and additional GIDs to reflect the new value in the user namespace.
func (s *Server) finalizeUserMapping(sb *sandbox.Sandbox, specgen *generate.Generator, mappings *idtools.IDMappings) {
}

// this function takes a container config and makes sure its SecurityContext
// is not nil. If it is, it makes sure to set default values for every field.
func setContainerConfigSecurityContext(containerConfig *types.ContainerConfig) *types.LinuxContainerSecurityContext {
	return &types.LinuxContainerSecurityContext{
		NamespaceOptions: &types.NamespaceOption{},
		SelinuxOptions:   &types.SELinuxOption{},
	}
}

func disableFipsForContainer(ctr ctrfactory.Container, containerDir string) error {
	return nil
}

func addSysfsMounts(ctr ctrfactory.Container, containerConfig *types.ContainerConfig, hostNet bool) {
}

func setOCIBindMountsPrivileged(g *generate.Generator) {
}

func (s *Server) addOCIBindMounts(ctx context.Context, ctr ctrfactory.Container, mountLabel, bindMountPrefix string, absentMountSourcesToReject []string, maybeRelabel, skipRelabel, cgroup2RW, idMapSupport bool, rroSupport bool, storageRoot string) ([]oci.ContainerVolume, []rspec.Mount, error) {
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

	for _, m := range mounts {
		dest := m.ContainerPath
		if dest == "" {
			return nil, nil, fmt.Errorf("mount.ContainerPath is empty")
		}
		// TODO: add support for image mounts here
		if m.HostPath == "" {
			return nil, nil, fmt.Errorf("mount.HostPath is empty")
		}
		if m.HostPath == "/" && dest == "/" {
			log.Warnf(ctx, "Configuration specifies mounting host root to the container root.  This is dangerous (especially with privileged containers) and should be avoided.")
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

		volumes = append(volumes, oci.ContainerVolume{
			ContainerPath:  dest,
			HostPath:       src,
			Readonly:       m.Readonly,
			Propagation:    m.Propagation,
			SelinuxRelabel: m.SelinuxRelabel,
		})

		ociMounts = append(ociMounts, rspec.Mount{
			Source:      src,
			Destination: dest,
			Options:     options,
		})
	}

	return volumes, ociMounts, nil
}

func addShmMount(ctr ctrfactory.Container, sb *sandbox.Sandbox) {
}

func setupSystemd(mounts []rspec.Mount, g generate.Generator) {
}

// Returns the spec Generator for the container, with some values set.
func (s *Server) getSpecGen(ctr ctrfactory.Container, containerConfig *types.ContainerConfig) *generate.Generator {
	specgen := ctr.Spec()
	specgen.HostSpecific = true
	specgen.ClearProcessRlimits()

	for _, u := range s.config.Ulimits() {
		specgen.AddProcessRlimits(u.Name, u.Hard, u.Soft)
	}

	specgen.SetRootReadonly(ctr.ReadOnly(s.config.ReadOnly))

	if s.config.ReadOnly {
		// tmpcopyup is a runc extension and is not part of the OCI spec.
		// WORK ON: Use "overlay" mounts as an alternative to tmpfs with tmpcopyup
		// Look at https://github.com/cri-o/cri-o/pull/1434#discussion_r177200245 for more info on this
		options := []string{"rw", "noexec", "nosuid", "nodev", "tmpcopyup"}
		mounts := map[string]string{
			"/tmp":     "mode=1777",
			"/var/run": "mode=0755",
			"/var/tmp": "mode=1777",
		}
		for target, mode := range mounts {
			if !isInCRIMounts(target, containerConfig.Mounts) {
				ctr.SpecAddMount(rspec.Mount{
					Destination: target,
					Type:        "tmpfs",
					Source:      "tmpfs",
					Options:     append(options, mode),
				})
			}
		}
	}

	return specgen
}

func (s *Server) specSetApparmorProfile(ctx context.Context, specgen *generate.Generator, ctr ctrfactory.Container, securityContext *types.LinuxContainerSecurityContext) error {
	return nil
}

func (s *Server) specSetBlockioClass(specgen *generate.Generator, containerName string, containerAnnotations, sandboxAnnotations map[string]string) error {
	return nil
}

func (s *Server) specSetDevices(ctr ctrfactory.Container, sb *sandbox.Sandbox) error {
	configuredDevices := s.config.Devices()

	privilegedWithoutHostDevices, err := s.Runtime().PrivilegedWithoutHostDevices(sb.RuntimeHandler())
	if err != nil {
		return err
	}

	annotationDevices, err := device.DevicesFromAnnotation(sb.Annotations()[crioann.DevicesAnnotation], s.config.AllowedDevices)
	if err != nil {
		return err
	}

	return ctr.SpecAddDevices(configuredDevices, annotationDevices, privilegedWithoutHostDevices, s.config.DeviceOwnershipFromSecurityContext)
}
